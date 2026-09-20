package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/l1280776919/mangaSync/internal/config"
	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
)

type Engine struct {
	st  *store.Store
	cfg *config.Manager

	mu      sync.Mutex
	running map[int64]context.CancelFunc
	subs    map[chan []byte]struct{}

	scanMu sync.Mutex
}

func New(st *store.Store, cfg *config.Manager) *Engine {
	return &Engine{st: st, cfg: cfg,
		running: map[int64]context.CancelFunc{}, subs: map[chan []byte]struct{}{}}
}

func (e *Engine) SourceFor(kind string) (source.Source, error) {
	s := e.cfg.Get()
	switch kind {
	case "pica":
		return source.NewPica(s.PicaProxy, s.Quality, s.ImageWorkers), nil
	case "jm":
		return source.NewJM(s.JmVenv, s.JmBridge, s.JmProxy, s.ImageWorkers, 2), nil
	}
	return nil, fmt.Errorf("未知的源: %s", kind)
}

func (e *Engine) SourceDir(kind string) string { return e.cfg.SourceDir(kind) }

func (e *Engine) Cred(a *store.Account) *source.Cred {
	return &source.Cred{AccountID: a.ID, Kind: a.Kind, Username: a.Username, Token: a.Token}
}

// Login 登录并把结果写回库
func (e *Engine) Login(ctx context.Context, a *store.Account) (*source.AccountInfo, error) {
	src, err := e.SourceFor(a.Kind)
	if err != nil {
		return nil, err
	}
	info, err := src.Login(ctx, a.Username, a.Password)
	if err != nil {
		_ = e.st.SaveLoginErr(a.ID, err.Error())
		return nil, err
	}
	if err := e.st.SaveLoginOK(a.ID, info.Token, info.Nickname, info.Level, info.FavoritesCount, info.FavoritesMax); err != nil {
		return nil, err
	}
	if a.Password == "" && info.Token != "" {
		// 用 token 登录的场景（jm 无密码），把 token 存起来
		_, _ = e.st.ListAccounts()
	}
	return info, nil
}

// EnsureCred 保证账号有可用登录态，失效则用保存的账号密码重登
func (e *Engine) EnsureCred(ctx context.Context, a *store.Account) (*source.Cred, error) {
	if a.Token != "" {
		return e.Cred(a), nil
	}
	if a.Username != "" && a.Password != "" {
		if _, err := e.Login(ctx, a); err != nil {
			return nil, err
		}
		a2, err := e.st.GetAccount(a.ID)
		if err != nil {
			return nil, err
		}
		return e.Cred(a2), nil
	}
	return nil, fmt.Errorf("账号 %s 没有可用登录态，请重新登录", a.Username)
}

// Relogin 强制重新登录
func (e *Engine) Relogin(ctx context.Context, a *store.Account) (*source.Cred, error) {
	if _, err := e.Login(ctx, a); err != nil {
		return nil, err
	}
	a2, err := e.st.GetAccount(a.ID)
	if err != nil {
		return nil, err
	}
	return e.Cred(a2), nil
}

// ---------------- 任务队列 ----------------

func (e *Engine) Enqueue(job *store.Job) (int64, error) {
	id, err := e.st.CreateJob(job)
	if err != nil {
		return 0, err
	}
	e.broadcastEvent("job", map[string]any{"id": id, "status": "queued", "kind": job.Kind,
		"comicId": job.ComicID, "title": job.Title})
	return id, nil
}

func (e *Engine) Cancel(id int64) error {
	e.mu.Lock()
	cancel, ok := e.running[id]
	e.mu.Unlock()
	if ok {
		cancel()
		return nil
	}
	j, err := e.st.GetJob(id)
	if err != nil {
		return err
	}
	if j.Status == "queued" {
		return e.st.SetJobStatus(id, "canceled", "手动取消")
	}
	return fmt.Errorf("任务不在运行中")
}

func (e *Engine) Retry(id int64) error {
	j, err := e.st.GetJob(id)
	if err != nil {
		return err
	}
	if j.Status == "running" {
		return fmt.Errorf("任务正在运行")
	}
	return e.st.RequeueJob(id)
}

// Start 启动调度循环
func (e *Engine) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(800 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e.dispatch(ctx)
			}
		}
	}()
}

func (e *Engine) dispatch(ctx context.Context) {
	s := e.cfg.Get()
	for {
		e.mu.Lock()
		running := len(e.running)
		e.mu.Unlock()
		if running >= s.Concurrency {
			return
		}
		j, err := e.st.NextQueuedJob()
		if err != nil || j == nil {
			return
		}
		jctx, cancel := context.WithCancel(ctx)
		e.mu.Lock()
		e.running[j.ID] = cancel
		e.mu.Unlock()
		// 立刻置为 running，防止下一轮 dispatch 重复取到同一个任务
		if err := e.st.SetJobStatus(j.ID, "running", ""); err != nil {
			e.mu.Lock()
			delete(e.running, j.ID)
			e.mu.Unlock()
			cancel()
			return
		}
		go func(job *store.Job) {
			defer func() {
				e.mu.Lock()
				delete(e.running, job.ID)
				e.mu.Unlock()
			}()
			e.runJob(jctx, job)
		}(j)
	}
}

func (e *Engine) runJob(ctx context.Context, j *store.Job) {
	if j.Title == "" {
		if src, err := e.SourceFor(j.Kind); err == nil {
			if d, err := src.Detail(ctx, e.Cred(&store.Account{Kind: j.Kind}), j.ComicID); err == nil {
				j.Title = d.Title
				_ = e.st.SetJobTitle(j.ID, d.Title)
			}
		}
	}
	e.log(j.ID, "info", fmt.Sprintf("任务 #%d 开始: %s (%s)", j.ID, j.Title, j.ComicID))

	acc, err := e.st.GetAccount(j.AccountID)
	if err != nil {
		e.fail(ctx, j, fmt.Sprintf("账号不存在: %v", err))
		return
	}
	src, err := e.SourceFor(j.Kind)
	if err != nil {
		e.fail(ctx, j, err.Error())
		return
	}
	cred, err := e.EnsureCred(ctx, acc)
	if err != nil {
		e.fail(ctx, j, err.Error())
		return
	}

	lastPush := time.Now()
	prog := source.Progress{}
	h := source.Hooks{
		Log: func(level, msg string) { e.log(j.ID, level, msg) },
		Chapter: func(cs source.ChapterState) {
			if cs.Err != "" {
				e.log(j.ID, "error", fmt.Sprintf("章节 %d %s: %s", cs.Order, cs.Title, cs.Err))
			}
		},
		Progress: func(p source.Progress) {
			prog = p
			if time.Since(lastPush) < 700*time.Millisecond {
				return
			}
			lastPush = time.Now()
			_ = e.st.UpdateJobProgress(j.ID, "running", p.CurrentChapter, p.ChaptersTotal, p.ChaptersDone,
				p.ImagesTotal, p.ImagesDone, p.Bytes, p.SpeedBps)
			e.broadcastEvent("job", map[string]any{
				"id": j.ID, "status": "running", "chaptersTotal": p.ChaptersTotal, "chaptersDone": p.ChaptersDone,
				"imagesTotal": p.ImagesTotal, "imagesDone": p.ImagesDone, "bytes": p.Bytes,
				"speedBps": p.SpeedBps, "currentChapter": p.CurrentChapter,
			})
		},
	}

	dst := e.SourceDir(j.Kind)
	res, dlErr := src.Download(ctx, cred, j.ComicID, j.Chapters, dst, h)

	// 统计落盘结果（无论成功失败都刷新一下本地库）
	if res != nil && res.Path != "" {
		_ = e.st.SetJobPath(j.ID, res.Path)
		imgs, bytes := dirStats(res.Path)
		title := res.Title
		if title == "" {
			title = j.Title
		}
		chapters, chaptersDone := 0, 0
		if d, err := src.Detail(ctx, cred, j.ComicID); err == nil {
			chapters = len(d.Chapters)
			if title == j.Title || title == "" {
				title = d.Title
			}
		}
		if chapters == 0 {
			chapters = countAllChapterDirs(res.Path)
		}
		chaptersDone = countChapterDirs(res.Path)
		_ = e.st.UpsertComic(&store.Comic{
			Kind: j.Kind, ComicID: j.ComicID, Title: title, Path: res.Path,
			Chapters: chapters, ChaptersDone: chaptersDone, Images: imgs, Bytes: bytes,
			Complete: chapters == 0 || chaptersDone >= chapters, Source: "download",
		})
	}

	switch {
	case ctx.Err() != nil:
		_ = e.st.SetJobStatus(j.ID, "canceled", "已取消")
		e.log(j.ID, "warn", "任务已取消")
	case dlErr != nil:
		e.fail(ctx, j, dlErr.Error())
	default:
		_ = e.st.UpdateJobProgress(j.ID, "done", "", prog.ChaptersTotal, prog.ChaptersDone,
			prog.ImagesTotal, prog.ImagesDone, prog.Bytes, 0)
		_ = e.st.SetJobStatus(j.ID, "done", "")
		e.log(j.ID, "info", fmt.Sprintf("任务 #%d 完成: %d 章 / %d 张 / %s",
			j.ID, prog.ChaptersDone, prog.ImagesDone, human(prog.Bytes)))
	}
	e.broadcastEvent("job", map[string]any{"id": j.ID, "status": "refresh"})
}

func (e *Engine) fail(ctx context.Context, j *store.Job, msg string) {
	if ctx.Err() != nil {
		_ = e.st.SetJobStatus(j.ID, "canceled", "已取消")
		return
	}
	_ = e.st.SetJobStatus(j.ID, "failed", msg)
	e.log(j.ID, "error", "任务失败: "+msg)
	e.broadcastEvent("job", map[string]any{"id": j.ID, "status": "failed", "error": msg})
}

func (e *Engine) log(jobID int64, level, msg string) {
	_ = e.st.AddJobLog(jobID, level, msg)
	e.broadcastEvent("log", map[string]any{"id": jobID, "level": level, "msg": msg, "ts": time.Now().Format(time.RFC3339)})
}

// ---------------- SSE ----------------

func (e *Engine) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	e.mu.Lock()
	e.subs[ch] = struct{}{}
	e.mu.Unlock()
	return ch
}

func (e *Engine) Unsubscribe(ch chan []byte) {
	e.mu.Lock()
	delete(e.subs, ch)
	e.mu.Unlock()
	close(ch)
}

func (e *Engine) broadcastEvent(name string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := append([]byte("event: "+name+"\ndata: "), b...)
	msg = append(msg, '\n', '\n')
	e.mu.Lock()
	subs := make([]chan []byte, 0, len(e.subs))
	for ch := range e.subs {
		subs = append(subs, ch)
	}
	e.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- msg:
		default: // 慢消费者丢弃，避免阻塞下载
		}
	}
}

func (e *Engine) Broadcast(name string, payload any) { e.broadcastEvent(name, payload) }

// ---------------- 收藏同步入队 ----------------

// SyncAccount 把某账号的收藏里「没下完/有更新」的入队，返回 (入队数, 跳过数)
func (e *Engine) SyncAccount(ctx context.Context, accID int64) (int, int, error) {
	acc, err := e.st.GetAccount(accID)
	if err != nil {
		return 0, 0, err
	}
	src, err := e.SourceFor(acc.Kind)
	if err != nil {
		return 0, 0, err
	}
	cred, err := e.EnsureCred(ctx, acc)
	if err != nil {
		if acc.Username != "" && acc.Password != "" {
			cred, err = e.Relogin(ctx, acc)
		}
		if err != nil {
			return 0, 0, err
		}
	}
	favs, err := src.Favorites(ctx, cred)
	if err != nil {
		if err == source.ErrAuth && acc.Username != "" && acc.Password != "" {
			if cred2, e2 := e.Relogin(ctx, acc); e2 == nil {
				favs, err = src.Favorites(ctx, cred2)
			}
		}
		if err != nil {
			return 0, 0, err
		}
	}
	enq, skip := 0, 0
	for _, c := range favs {
		rec, _ := e.st.GetComic(acc.Kind, c.ComicID)
		if rec != nil && rec.Complete && rec.Images > 0 {
			if p := rec.Path; p != "" {
				if n, _ := dirStats(p); n >= rec.Images {
					skip++
					continue
				}
			}
		}
		if _, err := e.Enqueue(&store.Job{Kind: acc.Kind, AccountID: acc.ID, ComicID: c.ComicID, Title: c.Title}); err != nil {
			continue
		}
		enq++
	}
	_ = e.st.TouchSync(acc.ID)
	return enq, skip, nil
}

// SyncAll 供定时任务调用
func (e *Engine) SyncAll(ctx context.Context) (int, int, error) {
	accounts, err := e.st.ListAccounts()
	if err != nil {
		return 0, 0, err
	}
	enq, skip := 0, 0
	for _, a := range accounts {
		en, sk, err := e.SyncAccount(ctx, a.ID)
		if err != nil {
			e.Broadcast("notice", map[string]any{"level": "error", "msg": fmt.Sprintf("账号 %s 同步失败: %v", a.Username, err)})
			continue
		}
		enq += en
		skip += sk
	}
	e.Broadcast("notice", map[string]any{"level": "info",
		"msg": fmt.Sprintf("定时同步完成: 入队 %d，跳过 %d", enq, skip)})
	return enq, skip, nil
}

// ---------------- 目录扫描 ----------------

// ScanLibrary 扫描两个源的下载目录，把磁盘上的漫画补进库
func (e *Engine) ScanLibrary() (found, added int, err error) {
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	for _, kind := range []string{"pica", "jm"} {
		root := e.SourceDir(kind)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
				continue
			}
			found++
			dir := filepath.Join(root, ent.Name())
			comicID := ""
			title := ent.Name()
			if kind == "pica" {
				comicID = readMetadataID(dir)
				if t := readMetadataTitle(dir); t != "" {
					title = t
				}
				if comicID == "" {
					comicID = "dir:" + ent.Name()
				}
			} else {
				// 禁漫目录名形如 "1473052 标题..."
				parts := strings.SplitN(ent.Name(), " ", 2)
				comicID = parts[0]
				if len(parts) > 1 {
					title = parts[1]
				}
				if _, perr := strconv.Atoi(comicID); perr != nil {
					comicID = "dir:" + ent.Name()
				}
			}
			if rec, _ := e.st.GetComic(kind, comicID); rec == nil {
				added++
			}
			imgs, bytes := dirStats(dir)
			chapters, chaptersDone := countAllChapterDirs(dir), countChapterDirs(dir)
			if chapters == 0 && imgs > 0 {
				// 单章本：图片直接放在本子目录里
				chapters, chaptersDone = 1, 1
			}
			complete := chapters == 0 || chaptersDone >= chapters
			_ = e.st.UpsertComic(&store.Comic{
				Kind: kind, ComicID: comicID, Title: title, Path: dir,
				Chapters: chapters, ChaptersDone: chaptersDone, Images: imgs, Bytes: bytes,
				Complete: complete, Source: "scan",
			})
		}
	}
	return found, added, nil
}

func readMetadataID(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return ""
	}
	var m struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(b, &m)
	return m.ID
}

func readMetadataTitle(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(b, &m)
	return m.Title
}

// countChapterDirs 统计「章节目录」（目录名形如 001 - xxx 或 1 xxx），单章本以图片文件数计
func countChapterDirs(dir string) int {
	n := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		imgs, _ := dirStats(filepath.Join(dir, ent.Name()))
		if imgs > 0 {
			n++
		}
	}
	return n
}

func countAllChapterDirs(dir string) int {
	return countChapterDirs(dir)
}

func dirStats(dir string) (int, int64) {
	var n int
	var b int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if isImageExt(fi.Name()) {
			n++
			b += fi.Size()
		}
		return nil
	})
	return n, b
}

func isImageExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}

func human(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB", "GB"} {
		if f < 1024 {
			if u == "B" {
				return fmt.Sprintf("%.0f%s", f, u)
			}
			return fmt.Sprintf("%.1f%s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1fTB", f)
}

// ActiveJobSnapshots 给 SSE 首次推送用
func (e *Engine) ActiveJobSnapshots() []map[string]any {
	jobs, _, err := e.st.ListJobs("running", 50, 0)
	if err != nil {
		return nil
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	out := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, map[string]any{
			"id": j.ID, "status": j.Status, "kind": j.Kind, "title": j.Title,
			"chaptersTotal": j.ChaptersTotal, "chaptersDone": j.ChaptersDone,
			"imagesTotal": j.ImagesTotal, "imagesDone": j.ImagesDone,
			"bytes": j.Bytes, "speedBps": j.SpeedBps, "currentChapter": j.CurrentChapter,
		})
	}
	return out
}
