package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/l1280776919/mangaSync/internal/config"
	"github.com/l1280776919/mangaSync/internal/engine"
	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
)

type Server struct {
	st   *store.Store
	cfg  *config.Manager
	eng  *engine.Engine
	base string // 缓存目录
}

func NewServer(st *store.Store, cfg *config.Manager, eng *engine.Engine) *Server {
	base := filepath.Join(config.Dir, "cache")
	_ = os.MkdirAll(filepath.Join(base, "covers"), 0o755)
	return &Server{st: st, cfg: cfg, eng: eng, base: base}
}

func (s *Server) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", s.health)
	m.HandleFunc("GET /api/settings", s.getSettings)
	m.HandleFunc("PUT /api/settings", s.putSettings)

	m.HandleFunc("GET /api/accounts", s.listAccounts)
	m.HandleFunc("POST /api/accounts", s.createAccount)
	m.HandleFunc("PATCH /api/accounts/{id}", s.patchAccount)
	m.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	m.HandleFunc("POST /api/accounts/{id}/login", s.loginAccount)
	m.HandleFunc("POST /api/accounts/{id}/sync", s.syncAccount)
	m.HandleFunc("GET /api/accounts/{id}/favorites", s.favorites)
	m.HandleFunc("POST /api/accounts/{id}/favorites", s.addFavorite)
	m.HandleFunc("DELETE /api/accounts/{id}/favorites/{comicId}", s.delFavorite)

	m.HandleFunc("GET /api/comics/{kind}/{comicId}", s.comicDetail)
	m.HandleFunc("GET /api/comics/{kind}/{comicId}/cover", s.comicCover)
	m.HandleFunc("GET /api/search", s.search)

	m.HandleFunc("GET /api/downloads", s.listJobs)
	m.HandleFunc("POST /api/downloads", s.createJob)
	m.HandleFunc("POST /api/downloads/{id}/cancel", s.cancelJob)
	m.HandleFunc("POST /api/downloads/{id}/retry", s.retryJob)
	m.HandleFunc("DELETE /api/downloads/{id}", s.deleteJob)
	m.HandleFunc("GET /api/downloads/{id}/logs", s.jobLogs)
	m.HandleFunc("POST /api/downloads/sync-all", s.syncAll)

	m.HandleFunc("GET /api/library", s.library)
	m.HandleFunc("DELETE /api/library/{id}", s.deleteLibraryItem)
	m.HandleFunc("POST /api/library/scan", s.scanLibrary)

	m.HandleFunc("GET /api/stats", s.stats)
	m.HandleFunc("GET /api/events", s.events)
	return m
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, format string, args ...any) {
	writeJSON(w, code, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func idOf(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

func intQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (s *Server) account(r *http.Request) (*store.Account, error) {
	id, err := idOf(r, "id")
	if err != nil {
		return nil, fmt.Errorf("账号 ID 不合法")
	}
	return s.st.GetAccount(id)
}

// accountFor 保证登录态可用（失效则自动重登）
func (s *Server) accountFor(a *store.Account) (*store.Account, *source.Cred, source.Source, error) {
	src, err := s.eng.SourceFor(a.Kind)
	if err != nil {
		return nil, nil, nil, err
	}
	cred, err := s.eng.EnsureCred(rctx(), a)
	if err != nil {
		return nil, nil, nil, err
	}
	return a, cred, src, nil
}

// ---------- handlers ----------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "version": Version, "uptimeSec": int(time.Since(startedAt).Seconds())})
}

var (
	Version   = "0.1.0"
	startedAt = time.Now()
)

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.cfg.Get())
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeErr(w, 400, "请求体不是合法 JSON")
		return
	}
	out, err := s.cfg.Update(patch)
	if err != nil {
		writeErr(w, 400, "保存设置失败: %v", err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	accs, err := s.st.ListAccounts()
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, accs)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind     string `json:"kind"`
		Username string `json:"username"`
		Password string `json:"password"`
		Label    string `json:"label"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "请求体不合法")
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.Username = strings.TrimSpace(req.Username)
	if req.Kind != "pica" && req.Kind != "jm" {
		writeErr(w, 400, "kind 只能是 pica 或 jm")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, 400, "账号和密码都要填")
		return
	}
	id, err := s.st.CreateAccount(&store.Account{Kind: req.Kind, Username: req.Username,
		Password: req.Password, Label: req.Label, Note: req.Note})
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	acc, _ := s.st.GetAccount(id)
	ctx, cancel := rctxTimeout(60 * time.Second)
	defer cancel()
	if _, err := s.eng.Login(ctx, acc); err != nil {
		_ = s.st.SaveLoginErr(id, err.Error())
		acc, _ = s.st.GetAccount(id)
		writeJSON(w, 200, acc) // 账号已建但登录失败，前端根据 status 提示
		return
	}
	acc, _ = s.st.GetAccount(id)
	writeJSON(w, 200, acc)
}

func (s *Server) patchAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	var req struct {
		Label    *string `json:"label"`
		Note     *string `json:"note"`
		Username *string `json:"username"`
		Password *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "请求体不合法")
		return
	}
	label, note, user, pass := acc.Label, acc.Note, "", ""
	if req.Label != nil {
		label = *req.Label
	}
	if req.Note != nil {
		note = *req.Note
	}
	if req.Username != nil {
		user = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil {
		pass = *req.Password
	}
	if err := s.st.UpdateAccount(acc.ID, label, note, user, pass); err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if user != "" || pass != "" {
		acc, _ = s.st.GetAccount(acc.ID)
		ctx, cancel := rctxTimeout(60 * time.Second)
		defer cancel()
		_, _ = s.eng.Login(ctx, acc)
	}
	acc, _ = s.st.GetAccount(acc.ID)
	writeJSON(w, 200, acc)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	if err := s.st.DeleteAccount(acc.ID); err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) loginAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	ctx, cancel := rctxTimeout(90 * time.Second)
	defer cancel()
	if _, err := s.eng.Login(ctx, acc); err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	acc, _ = s.st.GetAccount(acc.ID)
	writeJSON(w, 200, acc)
}

func (s *Server) syncAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	ctx, cancel := rctxTimeout(120 * time.Second)
	defer cancel()
	enq, skip, err := s.eng.SyncAccount(ctx, acc.ID)
	if err != nil {
		writeErr(w, 400, "同步失败: %v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"enqueued": enq, "skipped": skip})
}

func (s *Server) syncAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := rctxTimeout(300 * time.Second)
	defer cancel()
	enq, skip, err := s.eng.SyncAll(ctx)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"enqueued": enq, "skipped": skip})
}

// decorate 给漫画补上本地下载状态
func (s *Server) decorate(cs []*source.Comic) {
	for _, c := range cs {
		if rec, err := s.st.GetComic(c.Kind, c.ComicID); err == nil && rec != nil {
			c.Downloaded = rec.Complete && rec.Images > 0
			c.DownloadedChapters = rec.ChaptersDone
			c.Images = rec.Images
			c.Bytes = rec.Bytes
			c.LocalPath = rec.Path
			if rec.Chapters > 0 {
				c.ChaptersCount = rec.Chapters
			}
		}
	}
}

func (s *Server) favorites(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	_, cred, src, err := s.accountFor(acc)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	favs, err := src.Favorites(rctx(), cred)
	if err != nil {
		if err == source.ErrAuth {
			_ = s.st.SaveLoginErr(acc.ID, "登录态失效")
		}
		writeErr(w, 400, "%v", err)
		return
	}
	s.decorate(favs)

	kw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	if kw != "" {
		filtered := make([]*source.Comic, 0, len(favs))
		for _, c := range favs {
			if strings.Contains(strings.ToLower(c.Title), kw) || strings.Contains(strings.ToLower(c.Author), kw) ||
				strings.Contains(c.ComicID, kw) {
				filtered = append(filtered, c)
			}
		}
		favs = filtered
	}
	page := intQuery(r, "page", 1)
	size := intQuery(r, "pageSize", 50)
	total := len(favs)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	writeJSON(w, 200, map[string]any{
		"total": total, "page": page, "pageSize": size, "items": favs[start:end],
	})
}

func (s *Server) addFavorite(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	var req struct {
		ComicID string `json:"comicId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ComicID == "" {
		writeErr(w, 400, "comicId 不能为空")
		return
	}
	_, cred, src, err := s.accountFor(acc)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	if err := src.AddFavorite(rctx(), cred, req.ComicID); err != nil {
		writeErr(w, 400, "加入收藏失败: %v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) delFavorite(w http.ResponseWriter, r *http.Request) {
	acc, err := s.account(r)
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	comicID := r.PathValue("comicId")
	_, cred, src, err := s.accountFor(acc)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	if err := src.DelFavorite(rctx(), cred, comicID); err != nil {
		writeErr(w, 400, "取消收藏失败: %v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) comicDetail(w http.ResponseWriter, r *http.Request) {
	kind, comicID := r.PathValue("kind"), r.PathValue("comicId")
	var acc *store.Account
	if accID := intQuery(r, "accountId", 0); accID > 0 {
		acc, _ = s.st.GetAccount(int64(accID))
	}
	cred := s.bestCred(acc, kind)
	src, err := s.eng.SourceFor(kind)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	c, err := src.Detail(rctx(), cred, comicID)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	s.decorate([]*source.Comic{c})
	// 详情里的章节标出已下载状态
	if rec, _ := s.st.GetComic(kind, comicID); rec != nil && rec.Path != "" {
		for i := range c.Chapters {
			ch := &c.Chapters[i]
			var dir string
			dir = findChapterDir(rec.Path, ch.Order)
			if dir != "" {
				if n, _ := dirImages(dir); n > 0 {
					if ch.Images == 0 {
						ch.Images = n
					}
					if n >= ch.Images {
						ch.Downloaded = true
					}
				}
			}
		}
	}
	writeJSON(w, 200, c)
}

func (s *Server) bestCred(acc *store.Account, kind string) *source.Cred {
	if acc != nil && acc.Kind == kind && acc.Token != "" {
		return &source.Cred{AccountID: acc.ID, Kind: kind, Username: acc.Username, Token: acc.Token}
	}
	// 兜底：找一个同源且已登录的账号
	if a, err := s.st.FindAccountWithToken(kind); err == nil && a != nil {
		return &source.Cred{AccountID: a.ID, Kind: kind, Username: a.Username, Token: a.Token}
	}
	return &source.Cred{Kind: kind}
}

func (s *Server) comicCover(w http.ResponseWriter, r *http.Request) {
	kind, comicID := r.PathValue("kind"), r.PathValue("comicId")
	cachePath := filepath.Join(s.base, "covers", fmt.Sprintf("%s_%s.jpg", kind, safeName(comicID)))
	if b, err := os.ReadFile(cachePath); err == nil && len(b) > 0 {
		serveImage(w, b)
		return
	}
	src, err := s.eng.SourceFor(kind)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	var acc *store.Account
	if accID := intQuery(r, "accountId", 0); accID > 0 {
		acc, _ = s.st.GetAccount(int64(accID))
	}
	b, _, err := src.Cover(rctx(), s.bestCred(acc, kind), comicID)
	if err != nil {
		writeErr(w, 404, "取封面失败: %v", err)
		return
	}
	_ = os.WriteFile(cachePath, b, 0o644)
	serveImage(w, b)
}

func serveImage(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(200)
	_, _ = w.Write(b)
}

func safeName(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(s)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "pica"
	}
	kw := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if kw == "" {
		writeErr(w, 400, "关键词不能为空")
		return
	}
	src, err := s.eng.SourceFor(kind)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	var acc *store.Account
	if accID := intQuery(r, "accountId", 0); accID > 0 {
		acc, _ = s.st.GetAccount(int64(accID))
	}
	page := intQuery(r, "page", 1)
	size := intQuery(r, "pageSize", 20)
	sortBy := r.URL.Query().Get("sort")
	ctx, cancel := rctxTimeout(60 * time.Second)
	defer cancel()
	res, err := src.Search(ctx, s.bestCred(acc, kind), kw, page, size, sortBy)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	s.decorate(res.Items)
	writeJSON(w, 200, res)
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind      string `json:"kind"`
		AccountID int64  `json:"accountId"`
		ComicID   string `json:"comicId"`
		Title     string `json:"title"`
		Chapters  []int  `json:"chapters"`
		All       bool   `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "请求体不合法")
		return
	}
	if req.Kind != "pica" && req.Kind != "jm" {
		writeErr(w, 400, "kind 只能是 pica 或 jm")
		return
	}
	if req.ComicID == "" {
		writeErr(w, 400, "comicId 不能为空")
		return
	}
	if req.AccountID == 0 {
		// 自动挑一个同源账号
		accs, _ := s.st.ListAccounts()
		for _, a := range accs {
			if a.Kind == req.Kind {
				req.AccountID = a.ID
				break
			}
		}
	}
	chapters := req.Chapters
	if req.All {
		chapters = nil
	}
	job := &store.Job{Kind: req.Kind, AccountID: req.AccountID, ComicID: req.ComicID,
		Title: req.Title, Chapters: chapters}
	id, err := s.eng.Enqueue(job)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	page := intQuery(r, "page", 1)
	size := intQuery(r, "pageSize", 20)
	status := r.URL.Query().Get("status")
	jobs, total, err := s.st.ListJobs(status, size, (page-1)*size)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if jobs == nil {
		jobs = []*store.Job{}
	}
	writeJSON(w, 200, map[string]any{"total": total, "page": page, "pageSize": size, "items": jobs})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err != nil {
		writeErr(w, 400, "ID 不合法")
		return
	}
	if err := s.eng.Cancel(id); err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err != nil {
		writeErr(w, 400, "ID 不合法")
		return
	}
	if err := s.eng.Retry(id); err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err != nil {
		writeErr(w, 400, "ID 不合法")
		return
	}
	if err := s.st.DeleteJob(id); err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) jobLogs(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err != nil {
		writeErr(w, 400, "ID 不合法")
		return
	}
	logs, err := s.st.JobLogs(id, 500)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if logs == nil {
		logs = []store.JobLog{}
	}
	writeJSON(w, 200, logs)
}

func (s *Server) library(w http.ResponseWriter, r *http.Request) {
	page := intQuery(r, "page", 1)
	size := intQuery(r, "pageSize", 50)
	kind := r.URL.Query().Get("kind")
	kw := r.URL.Query().Get("keyword")
	sortBy := r.URL.Query().Get("sort")
	items, total, err := s.st.ListComics(kind, kw, sortBy, size, (page-1)*size)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if items == nil {
		items = []*store.Comic{}
	}
	comics, chapters, images, bytes, _ := s.st.ComicStats()
	writeJSON(w, 200, map[string]any{
		"total": total, "page": page, "pageSize": size, "items": items,
		"stats": map[string]any{"comics": comics, "chapters": chapters, "images": images, "bytes": bytes},
	})
}

func (s *Server) deleteLibraryItem(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err != nil {
		writeErr(w, 400, "ID 不合法")
		return
	}
	rec, err := s.st.DeleteComic(id)
	if err != nil {
		writeErr(w, 404, "记录不存在: %v", err)
		return
	}
	if r.URL.Query().Get("files") == "true" && rec.Path != "" {
		if err := os.RemoveAll(rec.Path); err != nil {
			writeErr(w, 500, "删除目录失败: %v", err)
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "deletedPath": rec.Path})
}

func (s *Server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	found, added, err := s.eng.ScanLibrary()
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]any{"found": found, "added": added})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	accs, _ := s.st.ListAccounts()
	counts, _ := s.st.JobCounts()
	comics, chapters, images, bytes, _ := s.st.ComicStats()
	free, total := diskUsage(s.cfg.Get().DownloadRoot)
	favs := 0
	for _, a := range accs {
		favs += a.FavoritesCount
	}
	writeJSON(w, 200, map[string]any{
		"accounts":  len(accs),
		"favorites": favs,
		"downloads": counts,
		"library":   map[string]any{"comics": comics, "chapters": chapters, "images": images, "bytes": bytes},
		"disk":      map[string]any{"freeBytes": free, "totalBytes": total},
	})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "不支持 SSE")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := s.eng.Subscribe()
	defer s.eng.Unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n") // 立刻回一帧心跳，客户端好判断连接已建立
	fl.Flush()
	// 首帧：当前活动任务
	for _, snap := range s.eng.ActiveJobSnapshots() {
		b, _ := json.Marshal(snap)
		fmt.Fprintf(w, "event: job\ndata: %s\n\n", b)
	}
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(msg)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// ---------- 小工具 ----------

func dirImages(dir string) (int, int64) {
	var n int
	var b int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
			n++
			if fi, err := e.Info(); err == nil {
				b += fi.Size()
			}
		}
	}
	return n, b
}

// findChapterDir 在漫画目录下按章节序号找目录（形如 001 - xxx / 1 xxx）
func findChapterDir(root string, order int) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	padded := fmt.Sprintf("%03d", order)
	plain := strconv.Itoa(order)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, padded+" -") || strings.HasPrefix(name, padded+" ") ||
			name == padded || name == plain || strings.HasPrefix(name, plain+" ") {
			return filepath.Join(root, name)
		}
	}
	return ""
}

func diskUsage(path string) (uint64, uint64) {
	var st unixStatfs
	if err := statfs(path, &st); err != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}

var _ = io.Discard
