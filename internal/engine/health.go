package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
)

type HealthIssue struct {
	Chapter int    `json:"chapter"`
	Page    int    `json:"page"`
	Reason  string `json:"reason"`
}
type ComicHealth struct {
	ID          int64         `json:"id"`
	Title       string        `json:"title"`
	Status      string        `json:"status"`
	Message     string        `json:"message"`
	Pages       int           `json:"pages"`
	Issues      []HealthIssue `json:"issues"`
	IssueCount  int           `json:"issueCount"`
	Chapters    []int         `json:"chapters"`
	JobID       int64         `json:"jobId,omitempty"`
	fingerprint string
	path        string
	badFiles    []string
}
type LibraryHealth struct {
	ID        int64         `json:"id"`
	Status    string        `json:"status"`
	Deep      bool          `json:"deep"`
	Total     int           `json:"total"`
	Processed int           `json:"processed"`
	Current   string        `json:"current"`
	Results   []ComicHealth `json:"results"`
}
type healthState struct {
	sync.Mutex
	run    LibraryHealth
	cancel context.CancelFunc
}

func (e *Engine) HealthStatus() LibraryHealth {
	e.health.Lock()
	defer e.health.Unlock()
	r := e.health.run
	r.Results = append([]ComicHealth{}, r.Results...)
	return r
}
func (e *Engine) CancelHealth(id int64) error {
	e.health.Lock()
	defer e.health.Unlock()
	if id != e.health.run.ID {
		return fmt.Errorf("体检记录已更新，请刷新")
	}
	if e.health.cancel != nil {
		e.health.cancel()
	}
	return nil
}
func (e *Engine) StartHealth(id int64, deep bool) (LibraryHealth, error) {
	e.health.Lock()
	if e.health.run.Status == "running" {
		e.health.Unlock()
		return LibraryHealth{}, fmt.Errorf("已有体检正在进行，请等待完成或先停止")
	}
	var books []*store.Comic
	var err error
	if id > 0 {
		var c *store.Comic
		c, err = e.st.ComicByID(id)
		if err == nil {
			books = []*store.Comic{c}
		}
	} else {
		books, err = e.st.AllComics()
	}
	if err != nil {
		e.health.Unlock()
		return LibraryHealth{}, err
	}
	e.syncMu.Lock()
	parent := e.lifeCtx
	e.syncMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	e.health.cancel = cancel
	e.health.run = LibraryHealth{ID: max(time.Now().UnixMilli(), e.health.run.ID+1), Status: "running", Deep: deep, Total: len(books), Results: []ComicHealth{}}
	initial := e.health.run
	e.health.Unlock()
	go func() {
		defer cancel()
		for _, c := range books {
			if ctx.Err() != nil {
				break
			}
			e.health.Lock()
			e.health.run.Current = c.Title
			e.health.Unlock()
			result := e.checkComic(ctx, c, deep)
			e.health.Lock()
			e.health.run.Results = append(e.health.run.Results, result)
			e.health.run.Processed++
			e.health.Unlock()
		}
		e.health.Lock()
		defer e.health.Unlock()
		e.health.run.Status = "done"
		if ctx.Err() != nil {
			e.health.run.Status = "canceled"
		}
		e.health.run.Current = ""
		e.health.cancel = nil
	}()
	return initial, nil
}
func (e *Engine) checkComic(ctx context.Context, c *store.Comic, deep bool) ComicHealth {
	r := ComicHealth{ID: c.ID, Title: c.Title, Status: "checked", Issues: []HealthIssue{}, Chapters: []int{}, path: c.Path}
	// Reserve only this comic; downloading/deleting other books remains responsive.
	e.opMu.Lock()
	active, err := e.st.ActiveJobID(c.Kind, c.ComicID)
	if err != nil || active > 0 {
		e.opMu.Unlock()
		r.Status = "skipped"
		r.Message = "存在活动下载或无法读取任务状态，请稍后检查"
		return r
	}
	if e.checking == nil {
		e.checking = map[string]bool{}
	}
	key := c.Kind + "/" + c.ComicID
	e.checking[key] = true
	e.opMu.Unlock()
	defer func() { e.opMu.Lock(); delete(e.checking, key); e.opMu.Unlock() }()
	fingerprint, err := localFingerprint(c.Path)
	if err != nil {
		r.Status = "error"
		r.Message = "无法读取漫画目录，请检查存储挂载和路径"
		return r
	}
	r.fingerprint = fingerprint
	checks, err := e.st.Chapters(c.Kind, c.ComicID)
	if err != nil {
		r.Status = "error"
		r.Message = err.Error()
		return r
	}
	dirs := map[int]string{}
	entries, err := os.ReadDir(c.Path)
	if err != nil {
		r.Status = "error"
		r.Message = err.Error()
		return r
	}
	maxOrder := max(1, c.Chapters)
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		parts := strings.Fields(ent.Name())
		if len(parts) == 0 {
			continue
		}
		n, er := strconv.Atoi(parts[0])
		if er == nil && n > 0 && n <= 10000 {
			dirs[n] = filepath.Join(c.Path, ent.Name())
			maxOrder = max(maxOrder, n)
		}
	}
	if len(dirs) == 0 && maxOrder == 1 {
		dirs[1] = c.Path
	}
	if maxOrder > 10000 {
		r.Status = "error"
		r.Message = "章节数超出检查上限"
		return r
	}
	repair := map[int]bool{}
	issue := func(ch, page int, reason string, fix bool) {
		r.IssueCount++
		if len(r.Issues) < 200 {
			r.Issues = append(r.Issues, HealthIssue{ch, page, reason})
		}
		if fix {
			repair[ch] = true
		}
	}
	unknown := false
	for ch := 1; ch <= maxOrder; ch++ {
		if ctx.Err() != nil {
			r.Status = "canceled"
			r.Message = "检查已停止，结果不完整"
			return r
		}
		dir := dirs[ch]
		if dir == "" {
			issue(ch, 0, "章节目录缺失", true)
			continue
		}
		files, er := os.ReadDir(dir)
		if er != nil {
			r.Status = "error"
			r.Message = er.Error()
			return r
		}
		expected := max(checks[ch].Images, source.StoredPageCount(dir, c.Kind))
		known := expected > 0
		pages := map[int]string{}
		for _, f := range files {
			if f.IsDir() || !isImageExt(f.Name()) {
				continue
			}
			n, er := strconv.Atoi(strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())))
			if er != nil || n < 1 || n > 5000 {
				continue
			}
			p := filepath.Join(dir, f.Name())
			if _, exists := pages[n]; exists {
				issue(ch, n, "同页存在多个图片文件，请核对重复文件", false)
			}
			pages[n] = p
			expected = max(expected, n)
		}
		if expected > 5000 {
			r.Status = "error"
			r.Message = "页数超出检查上限"
			return r
		}
		if !known {
			unknown = true
		}
		if expected == 0 {
			issue(ch, 0, "章节没有图片", true)
		}
		for n := 1; n <= expected; n++ {
			if ctx.Err() != nil {
				r.Status = "canceled"
				r.Message = "检查已停止，结果不完整"
				return r
			}
			p := pages[n]
			if p == "" {
				issue(ch, n, "图片缺失", true)
				continue
			}
			r.Pages++
			fi, er := os.Stat(p)
			if er == nil && fi.Size() == 0 {
				er = fmt.Errorf("图片为空")
			}
			if er == nil && c.Kind == "jm" && source.IsLegacyJMGIF(p) {
				er = fmt.Errorf("旧版 GIF 还原异常")
			}
			if er == nil && deep {
				er = source.InspectImage(ctx, p)
			}
			if er != nil {
				issue(ch, n, er.Error(), true)
				r.badFiles = append(r.badFiles, p)
			}
		}
	}
	if ctx.Err() != nil {
		r.Status = "canceled"
		r.Message = "检查已停止，结果不完整"
		return r
	}
	for ch := range repair {
		r.Chapters = append(r.Chapters, ch)
	}
	sort.Ints(r.Chapters)
	if r.IssueCount > 0 {
		r.Status = "issues"
	}
	if unknown {
		r.Message = "部分章节没有历史页数，无法判断尾页是否缺失；可通过账号完整核验补齐信息。"
	}
	after, err := localFingerprint(c.Path)
	if err != nil || after != fingerprint {
		r.Status = "stale"
		r.Message = "文件在检查期间发生变化，请重新检查"
		r.Chapters = nil
	}
	return r
}

func (e *Engine) RepairHealth(runID, id int64) (int64, error) {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.health.Lock()
	defer e.health.Unlock()
	if e.health.run.ID != runID {
		return 0, fmt.Errorf("体检报告已更新，请重新检查")
	}
	if e.health.run.Status == "running" {
		return 0, fmt.Errorf("请等待体检结束")
	}
	for i := range e.health.run.Results {
		r := &e.health.run.Results[i]
		if r.ID != id {
			continue
		}
		if r.JobID > 0 {
			return r.JobID, nil
		}
		if r.Status != "issues" || len(r.Chapters) == 0 {
			return 0, fmt.Errorf("没有可自动修复的章节")
		}
		c, err := e.st.ComicByID(id)
		if err != nil {
			return 0, err
		}
		if active, err := e.st.ActiveJobID(c.Kind, c.ComicID); err != nil {
			return 0, err
		} else if active > 0 {
			return 0, fmt.Errorf("该漫画已有活动任务，请等待完成后重新检查")
		}
		if blocked, err := e.st.InTrash(c.Kind, c.ComicID); err != nil {
			return 0, err
		} else if blocked {
			return 0, fmt.Errorf("请先从回收站恢复漫画")
		}
		fp, err := localFingerprint(c.Path)
		if err != nil || c.Path != r.path || fp != r.fingerprint {
			return 0, fmt.Errorf("文件已变化，请重新检查")
		}
		accounts, err := e.st.ListAccounts()
		if err != nil {
			return 0, err
		}
		var account int64
		for _, a := range accounts {
			if a.Kind == c.Kind {
				account = a.ID
				break
			}
		}
		if account == 0 {
			return 0, fmt.Errorf("请先添加同源账号，再修复")
		}
		root, err := filepath.EvalSymlinks(e.SourceDir(c.Kind))
		if err != nil {
			return 0, err
		}
		for _, p := range r.badFiles {
			parent, err := filepath.EvalSymlinks(filepath.Dir(p))
			if err != nil {
				return 0, err
			}
			rel, err := filepath.Rel(root, parent)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return 0, fmt.Errorf("修复目录不在当前漫画库内")
			}
		}
		// Only mark the exact checked files. Keep original bytes until a replacement succeeds.
		for _, p := range r.badFiles {
			if err := os.WriteFile(p+".repair", []byte("repair requested\n"), 0600); err != nil {
				return 0, err
			}
		}
		job, err := e.st.CreateJob(&store.Job{Kind: c.Kind, AccountID: account, ComicID: c.ComicID, Title: c.Title, Chapters: r.Chapters})
		if err != nil {
			return 0, err
		}
		r.JobID = job
		e.broadcastEvent("job", map[string]any{"id": job, "status": "queued", "kind": c.Kind, "comicId": c.ComicID, "title": c.Title})
		return job, nil
	}
	return 0, fmt.Errorf("体检结果不存在")
}
