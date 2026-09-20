package main

import (
	"compress/gzip"
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/l1280776919/mangaSync/internal/api"
	"github.com/l1280776919/mangaSync/internal/config"
	"github.com/l1280776919/mangaSync/internal/engine"
	"github.com/l1280776919/mangaSync/internal/store"
)

//go:embed all:web/dist
var webFS embed.FS

// gzipMiddleware 对文本类响应做 gzip 压缩（前端 element-plus 打包后 1MB，压缩后 ~340KB，
// 公网/穿透访问时差别很大；SSE 与二进制流不压缩）
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.URL.Path == "/api/events" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
	compress    bool
}

func compressibleType(ct string) bool {
	ct = strings.ToLower(ct)
	for _, s := range []string{"javascript", "json", "text/", "css", "html", "svg", "xml"} {
		if strings.Contains(ct, s) {
			return true
		}
	}
	return false
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	ct := g.Header().Get("Content-Type")
	if ct != "" && compressibleType(ct) && !strings.Contains(ct, "event-stream") {
		g.compress = true
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Del("Content-Length")
		g.gz = gzip.NewWriter(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		g.WriteHeader(http.StatusOK)
	}
	if g.compress && g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipWriter) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipWriter) close() {
	if g.gz != nil {
		_ = g.gz.Close()
	}
}

func main() {
	addr := flag.String("addr", "", "监听地址，例如 :8787（默认取设置里的 serverPort）")
	dataDir := flag.String("data", "", "数据目录（默认取环境变量 MANGASYNC_HOME，再默认 "+config.DefaultDir+"）")
	noScan := flag.Bool("no-scan", false, "启动时不扫描漫画库")
	flag.Parse()

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
	mux.Handle("/api/", srv.Routes())
	mux.Handle("/", spaHandler())

	listen := *addr
	if listen == "" {
		listen = fmt.Sprintf(":%d", cfg.Get().ServerPort)
	}
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           logRequests(gzipMiddleware(api.BasicAuth(cfg, mux))),
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

// spaHandler 托管前端静态文件，找不到的路径回落到 index.html（hash 路由其实用不到，但留着无妨）
func spaHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Printf("前端资源缺失: %v", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "前端资源未构建", 500)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			// 回落到 index.html
			b, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
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
