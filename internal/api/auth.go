package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/l1280776919/mangaSync/internal/auth"
	"github.com/l1280776919/mangaSync/internal/store"
)

const (
	sessionCookie = "ms_session"
	sessionTTL    = 30 * 24 * time.Hour
)

// ctxKey 请求上下文 key 类型
type ctxKey string

const ctxSession ctxKey = "mangasync.session"

// cookieSecure 决定会话 cookie 是否带 Secure。
// 公网入口是 Cloudflare 终结 TLS（客户端到 CF 是 https），此时 Cookie 必须带 Secure，
// 否则浏览器会把它当作明文 cookie：一次 http 请求/一次混合内容就可能把 30 天有效的
// 会话 token 明文发出去，拿到即完全冒充管理员。
// 但内网是 http://<nas>:8787 直连，无条件 Secure 会让内网登录直接丢会话
// （浏览器不会在 http 上保存/回传 Secure cookie），所以只在确实走 https 时启用：
// 判断依据是 TLS 直连（r.TLS != nil）或前置代理声明的 X-Forwarded-Proto: https。
// 取舍：内网明文 http 访问时 cookie 仍非 Secure。要彻底封死这条路径，
// 需把服务本身只跑在 TLS 后面（或在反代上强制 https 跳转）。
func cookieSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// sessionFrom 从请求上下文取会话（由 SessionAuth 注入）
func sessionFrom(r *http.Request) *store.Session {
	sess, _ := r.Context().Value(ctxSession).(*store.Session)
	return sess
}

// 无需登录即可访问的接口：health 供探活，login/logout 自己处理未登录情况
// （非 /api/ 路径一律放行，否则登录页的静态资源都拿不到）
var publicPaths = map[string]bool{
	"/api/health": true, "/api/auth/login": true, "/api/auth/logout": true,
}

// 处于「必须修改初始密码」状态时仍可访问的接口：改密、登出、取当前用户
var duringMustChangePaths = map[string]bool{
	"/api/auth/password": true, "/api/auth/logout": true, "/api/auth/me": true,
}

// SessionAuth 会话鉴权中间件：publicPaths 放行，其余接口必须带有效会话（否则 401）；
// 未完成首次改密时，除 duringMustChangePaths 外一律 403。
func (s *Server) SessionAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") || publicPaths[path] {
			next.ServeHTTP(w, r)
			return
		}
		sess := s.lookupSession(r)
		if sess == nil {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		if sess.MustChangePassword && !duringMustChangePaths[path] {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error": "请先修改初始密码", "mustChangePassword": true,
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxSession, sess)))
	})
}

// lookupSession 读 cookie 查会话
func (s *Server) lookupSession(r *http.Request) *store.Session {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	sess, err := s.st.GetSession(c.Value)
	if err != nil {
		return nil
	}
	return sess
}

func sessionInfo(sess *store.Session) map[string]any {
	return map[string]any{
		"username":           sess.Username,
		"isAdmin":            sess.IsAdmin,
		"mustChangePassword": sess.MustChangePassword,
		"lastLoginAt":        sess.LastLoginAt,
		"sessionExpiresAt":   sess.ExpiresAt,
	}
}

// POST /api/auth/login
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, 400, "请求体不是合法 JSON")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || in.Password == "" {
		writeErr(w, 400, "请输入账号和密码")
		return
	}

	key := strings.ToLower(in.Username)
	if ok, wait := s.limiter.Allow(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		writeErr(w, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试")
		return
	}

	u, err := s.st.GetUserByName(in.Username)
	if err != nil || !auth.VerifyPassword(in.Password, u.PassHash, u.Salt, u.Iterations) {
		s.limiter.Fail(key)
		writeErr(w, http.StatusUnauthorized, "账号或密码错误") // 不区分账号不存在/密码错误
		return
	}
	s.limiter.Reset(key)

	token, err := auth.NewToken()
	if err != nil {
		writeErr(w, 500, "生成会话失败")
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.st.CreateSession(token, u.ID, expires, r.UserAgent()); err != nil {
		writeErr(w, 500, "创建会话失败")
		return
	}
	_ = s.st.TouchLastLogin(u.ID)

	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: cookieSecure(r),
		MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, 200, map[string]any{
		"username":           u.Username,
		"isAdmin":            u.IsAdmin,
		"mustChangePassword": u.MustChangePassword,
		"lastLoginAt":        u.LastLoginAt,
	})
}

// POST /api/auth/logout
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.st.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: cookieSecure(r), MaxAge: -1,
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/auth/me
func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	sess := s.lookupSession(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, 200, sessionInfo(sess))
}

// POST /api/auth/password
func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess == nil {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	var in struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, 400, "请求体不是合法 JSON")
		return
	}
	if err := auth.CheckPassword(in.NewPassword); err != nil {
		writeErr(w, 400, "%s", err.Error())
		return
	}
	if in.NewPassword == in.OldPassword {
		writeErr(w, 400, "新密码不能与原密码相同")
		return
	}

	u, err := s.st.GetUserByName(sess.Username)
	if err != nil {
		writeErr(w, 401, "未登录")
		return
	}
	if !auth.VerifyPassword(in.OldPassword, u.PassHash, u.Salt, u.Iterations) {
		writeErr(w, http.StatusUnauthorized, "原密码不正确")
		return
	}

	hash, salt, iter, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		writeErr(w, 500, "生成密码失败")
		return
	}
	if err := s.st.UpdateUserPassword(u.ID, hash, salt, iter); err != nil {
		writeErr(w, 500, "保存新密码失败")
		return
	}
	// 踢掉该账号的其它会话，保留当前
	_ = s.st.DeleteOtherSessions(u.ID, sess.Token)

	writeJSON(w, 200, map[string]any{"ok": true, "mustChangePassword": false})
}
