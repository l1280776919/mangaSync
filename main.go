package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/l1280776919/mangaSync/internal/api"
	"github.com/l1280776919/mangaSync/internal/auth"
	"github.com/l1280776919/mangaSync/internal/config"
	"github.com/l1280776919/mangaSync/internal/engine"
	"github.com/l1280776919/mangaSync/internal/store"
)

//go:embed all:web/dist
var webFS embed.FS

// 首次初始化用的默认管理员（登录后会强制改密）
const (
	DefaultAdminUser     = "admin"
	DefaultAdminPassword = "admin999"
)

// gzipMiddleware 只处理 /api/：把响应缓冲后一次性写出并带上 Content-Length，
// 避免 chunked 在弱网/穿透链路被截断（浏览器对不定长响应截断是直接报错不重试的）。
// 静态资源由 staticAssets 预压缩处理，SSE 保持流式不压缩。
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/events" ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		buf := &bufferedWriter{header: http.Header{}, status: http.StatusOK}
		next.ServeHTTP(buf, r)

		body := buf.body.Bytes()
		h := w.Header()
		for k, vs := range buf.header {
			for _, v := range vs {
				h.Add(k, v)
			}
		}
		h.Add("Vary", "Accept-Encoding")
		if len(body) >= 512 && compressibleType(buf.header.Get("Content-Type")) {
			var gz bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&gz, gzip.BestSpeed)
			_, _ = zw.Write(body)
			_ = zw.Close()
			h.Set("Content-Encoding", "gzip")
			body = gz.Bytes()
		}
		h.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(buf.status)
		_, _ = w.Write(body)
	})
}

// bufferedWriter 先把响应攒在内存里，便于计算 Content-Length
type bufferedWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *bufferedWriter) Header() http.Header { return b.header }

func (b *bufferedWriter) WriteHeader(code int) { b.status = code }

func (b *bufferedWriter) Write(p []byte) (int, error) { return b.body.Write(p) }

func (b *bufferedWriter) Flush() {}

func compressibleType(ct string) bool {
	ct = strings.ToLower(ct)
	for _, s := range []string{"javascript", "json", "text/", "css", "html", "svg", "xml"} {
		if strings.Contains(ct, s) {
			return true
		}
	}
	return false
}

func main() {
	addr := flag.String("addr", "", "监听地址，例如 :8787（默认取设置里的 serverPort）")
	dataDir := flag.String("data", "", "数据目录（默认取环境变量 MANGASYNC_HOME，再默认 "+config.DefaultDir+"）")
	noScan := flag.Bool("no-scan", false, "启动时不扫描漫画库")
	frontendFor := flag.String("frontend-for", "",
		"前端节点模式：本机只发前端静态资源，并把 /api/* 反代到该地址（例如 http://127.0.0.1:18888）。"+
			"用于 NAS 上行很弱时把静态资源放到 VPS 就近直发，只让 API 走隧道")
	flag.Parse()

	// 前端节点：不碰数据库/引擎，纯粹发静态资源 + 反代 API
	if *frontendFor != "" {
		runFrontendNode(*addr, *frontendFor)
		return
	}

	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[mangasync] ")

	cfg, err := config.Load(*dataDir)
	if err != nil {
		log.Fatalf("读取设置失败: %v", err)
	}
	st, err := store.Open(cfg.Dir())
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	// 首次初始化：没有任何后台账号时创建默认管理员 admin/admin999，并强制首次登录改密
	if n, err := st.CountUsers(); err == nil && n == 0 {
		hash, salt, iter, herr := auth.HashPassword(DefaultAdminPassword)
		if herr != nil {
			log.Fatalf("生成默认密码失败: %v", herr)
		}
		if _, cerr := st.CreateUser(DefaultAdminUser, hash, salt, iter, true, true); cerr != nil {
			log.Fatalf("创建默认管理员失败: %v", cerr)
		}
		log.Printf("首次初始化：已创建管理员 %s / %s（首次登录必须修改密码）", DefaultAdminUser, DefaultAdminPassword)
	}
	_ = st.CleanupSessions()
	eng := engine.New(st, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	eng.Start(ctx)

	// 启动时把磁盘上已有的漫画补进库（不影响正在跑的任务）
	go func() {
		if *noScan {
			return
		}
		found, added, err := eng.ScanLibrary()
		if err != nil {
			log.Printf("扫描漫画库失败: %v", err)
			return
		}
		log.Printf("扫描漫画库: 发现 %d 部，新增 %d 条", found, added)
	}()

	go scheduler(ctx, cfg, eng)

	srv := api.NewServer(st, cfg, eng)
	mux := http.NewServeMux()
	assets := newStaticAssets()
	mux.Handle("/api/", srv.Routes())
	mux.Handle("/", assets)

	listen := *addr
	if listen == "" {
		listen = fmt.Sprintf(":%d", cfg.Get().ServerPort)
	}
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           logRequests(gzipMiddleware(srv.SessionAuth(mux))),
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		log.Printf("mangaSync %s 启动，监听 %s，数据目录 %s", api.Version, listen, cfg.Dir())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务退出: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到退出信号，正在关闭…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

// staticAssets 启动时把前端文本资源预压缩进内存：响应可带 Content-Length（定长），
// 避免 chunked —— 穿透/弱网链路丢包导致 chunked 截断时，浏览器会直接
// ERR_INCOMPLETE_CHUNKED_ENCODING 而拿不到资源（element-plus 那个 1MB 的包就中过招）。
type staticAssets struct {
	sub   fs.FS
	gz    map[string][]byte // "/assets/x.js" -> gzip 字节
	html  []byte            // index.html 兜底（SPA 路由回落）
	found bool
}

func newStaticAssets() *staticAssets {
	sa := &staticAssets{gz: map[string][]byte{}}
	sub, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Printf("前端资源缺失: %v", err)
		return sa
	}
	sa.sub, sa.found = sub, true
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !compressibleAsset(p) {
			return err
		}
		b, rerr := fs.ReadFile(sub, p)
		if rerr != nil || len(b) < 512 {
			return nil
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		_, _ = zw.Write(b)
		_ = zw.Close()
		sa.gz["/"+p] = buf.Bytes()
		return nil
	})
	if err != nil {
		log.Printf("预压缩前端资源失败: %v", err)
	}
	sa.html, _ = fs.ReadFile(sub, "index.html")
	var saved int
	for _, b := range sa.gz {
		saved += len(b)
	}
	log.Printf("前端资源预压缩完成: %d 个文件, gzip 后共 %d KB", len(sa.gz), saved/1024)
	return sa
}

func compressibleAsset(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".js", ".css", ".html", ".svg", ".json", ".map", ".txt":
		return true
	}
	return false
}

func (sa *staticAssets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !sa.found {
		http.Error(w, "前端资源未构建", http.StatusInternalServerError)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}

	// 命中预压缩缓存 → 定长 gzip 响应
	if gz, ok := sa.gz["/"+p]; ok && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h := w.Header()
		h.Set("Content-Type", mime.TypeByExtension(filepath.Ext(p)))
		h.Set("Content-Encoding", "gzip")
		h.Set("Content-Length", strconv.Itoa(len(gz)))
		h.Set("Vary", "Accept-Encoding")
		h.Set("Cache-Control", cacheControlFor(p))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(gz)
		return
	}

	// 其它（未压缩类型 / 不支持 gzip 的客户端）
	if _, err := fs.Stat(sa.sub, p); err != nil {
		if len(sa.html) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(sa.html)))
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(sa.html)
		return
	}
	b, err := fs.ReadFile(sa.sub, p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(p)))
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", cacheControlFor(p))
	_, _ = w.Write(b)
}

// 带哈希的构建产物可以长期强缓存；入口 HTML 不缓存，便于发版后立即生效
func cacheControlFor(p string) string {
	if strings.HasPrefix(p, "assets/") && strings.Contains(filepath.Base(p), "-") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// runFrontendNode 前端节点：静态资源本机直发，/api/* 反代回后端（通常是 NAS 上的隧道端口）。
// 背景：NAS 上行带宽很小（穿透实测单个响应 >55KB 基本拉不动，1MB 的前端包直接报
// ERR_INCOMPLETE_CHUNKED_ENCODING）。把前端放到公网 VPS 就近直发、只有小的 API 请求
// 走隧道，页面才能真正打开。
func runFrontendNode(addr, backend string) {
	if addr == "" {
		addr = ":8787"
	}
	target, err := url.Parse(backend)
	if err != nil {
		log.Fatalf("-frontend-for 不是合法地址: %v", err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.FlushInterval = -1 // SSE 需要立刻透传，不能缓冲
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("反代 %s 失败: %v", r.URL.Path, err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"后端不可达"}`))
	}

	assets := newStaticAssets()
	mux := http.NewServeMux()
	mux.Handle("/api/", rp)
	mux.Handle("/", assets)

	srv := &http.Server{Addr: addr, Handler: logRequests(mux), ReadHeaderTimeout: 15 * time.Second}
	log.Printf("前端节点模式：监听 %s，静态资源本机直发，/api/* → %s", addr, backend)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP 服务退出: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/events" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

// scheduler 每天在设定时间跑一次全量收藏同步
func scheduler(ctx context.Context, cfg *config.Manager, eng *engine.Engine) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	lastRun := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s := cfg.Get()
			if !s.Schedule.Enabled || s.Schedule.Time == "" {
				continue
			}
			now := time.Now()
			hhmm := now.Format("15:04")
			today := now.Format("2006-01-02")
			if hhmm != s.Schedule.Time || lastRun == today {
				continue
			}
			lastRun = today
			log.Printf("定时同步开始（%s）", s.Schedule.Time)
			go func() {
				c := ctx
				enq, skip, err := eng.SyncAll(c)
				if err != nil {
					log.Printf("定时同步失败: %v", err)
					return
				}
				log.Printf("定时同步完成: 入队 %d，跳过 %d", enq, skip)
			}()
		}
	}
}
