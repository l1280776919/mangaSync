package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
)

// Only numbered, non-empty page files count. Covers and temporary downloads do not.
func pageCount(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	seen := map[int]bool{}
	for _, ent := range entries {
		if ent.IsDir() || !isImageExt(ent.Name()) {
			continue
		}
		n, e := strconv.Atoi(strings.TrimSuffix(ent.Name(), filepath.Ext(ent.Name())))
		if e != nil || n < 1 {
			continue
		}
		fi, e := ent.Info()
		if e == nil && fi.Size() > 0 {
			seen[n] = true
		}
	}
	// Gaps must not be mistaken for a complete chapter.
	n := 0
	for seen[n+1] {
		n++
	}
	return n
}
func chapterPath(root string, order, total int) string {
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		parts := strings.Fields(e.Name())
		if len(parts) > 0 {
			n, _ := strconv.Atoi(parts[0])
			if n == order {
				return filepath.Join(root, e.Name())
			}
		}
	}
	if total == 1 && order == 1 {
		return root
	}
	return ""
}
func (e *Engine) verifyComic(ctx context.Context, src source.Source, cred *source.Cred, id string) ([]int, error) {
	detail, err := src.Detail(ctx, cred, id)
	if err != nil {
		return nil, err
	}
	if len(detail.Chapters) == 0 {
		return nil, fmt.Errorf("漫画 %s 没有章节", id)
	}
	rec, _ := e.st.GetComic(src.Kind(), id)
	checks, err := e.st.Chapters(src.Kind(), id)
	if err != nil {
		return nil, err
	}
	missing := []int{}
	done := 0
	for _, ch := range detail.Chapters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := ""
		if rec != nil {
			path = chapterPath(rec.Path, ch.Order, len(detail.Chapters))
		}
		if path == "" || pageCount(path) == 0 {
			missing = append(missing, ch.Order)
			continue
		}
		expected, _, err := src.Pages(ctx, cred, id, ch.Order)
		if err != nil {
			return nil, err
		}
		if expected < 1 {
			return nil, fmt.Errorf("章节 %d 页数未知", ch.Order)
		}

		old, known := checks[ch.Order]
		identityOK := !known || old.RemoteID == "" || old.RemoteID == ch.ID
		if path != "" && identityOK && pageCount(path) == expected {
			if err = e.st.SaveChapter(src.Kind(), id, store.ChapterCheck{Order: ch.Order, RemoteID: ch.ID, Images: expected, Path: path}); err != nil {
				return nil, err
			}
			done++
		} else {
			missing = append(missing, ch.Order)
		}
	}
	return missing, e.st.SetCompleteness(src.Kind(), id, len(detail.Chapters), done)
}

// Library mutations share the same lock as enqueue and dispatch; active downloads cannot be removed.
func (e *Engine) DeleteComic(id int64, files bool) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	c, err := e.st.ComicByID(id)
	if err != nil {
		return err
	}
	active, err := e.st.ActiveJobID(c.Kind, c.ComicID)
	if err != nil {
		return err
	}
	if active > 0 {
		return fmt.Errorf("请先取消下载任务 #%d，等待任务结束后再删除", active)
	}
	if !files {
		_, err = e.st.DeleteComic(id)
		return err
	}
	root, err := filepath.EvalSymlinks(e.SourceDir(c.Kind))
	if err != nil {
		return err
	}
	path, err := filepath.EvalSymlinks(c.Path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("目录不在当前漫画库内，拒绝删除")
	}
	trashRoot := filepath.Join(root, ".mangasync-trash")
	if err = os.MkdirAll(trashRoot, 0700); err != nil {
		return err
	}
	dest := filepath.Join(trashRoot, fmt.Sprintf("%d-%d", id, time.Now().UnixNano()))
	tid, err := e.st.PrepareTrash(c, dest)
	if err != nil {
		return err
	}
	if err = os.Rename(path, dest); err != nil {
		e.st.SetTrashStatus(tid, "aborted")
		return err
	}
	// Keep the recovery record if the DB write fails after the move.
	return e.st.FinishTrash(tid, id)
}
func (e *Engine) RestoreTrash(id int64) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	items, err := e.st.TrashItems()
	if err != nil {
		return err
	}
	for _, t := range items {
		if t.ID != id {
			continue
		}
		active, er := e.st.ActiveJobID(t.Kind, t.ComicID)
		if er != nil {
			return er
		}
		if active > 0 {
			return fmt.Errorf("该漫画存在活动任务")
		}
		var c store.Comic
		if er = json.Unmarshal([]byte(t.ComicJSON), &c); er != nil {
			return er
		}
		if _, er = os.Stat(t.OriginalPath); os.IsNotExist(er) {
			if er = os.Rename(t.Path, t.OriginalPath); er != nil {
				return er
			}
		} else if er != nil {
			return er
		} else {
			if _, te := os.Stat(t.Path); te == nil {
				return fmt.Errorf("原目录已存在，请先处理目录冲突")
			}
		}
		if er = e.st.UpsertComic(&c); er != nil {
			return er
		}
		return e.st.SetTrashStatus(id, "restored")
	}
	return fmt.Errorf("回收记录不存在")
}
func (e *Engine) DownloadStatus() string {
	s := e.cfg.Get()
	if s.MinFreeMB <= 0 {
		return ""
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.DownloadRoot, &stat); err != nil {
		return "无法检查下载磁盘，等待目录恢复"
	}
	if uint64(stat.Bavail)*uint64(stat.Bsize) < uint64(s.MinFreeMB)*1024*1024 {
		return "磁盘空间不足，下载已暂停；释放空间后自动继续"
	}
	return ""
}
func (e *Engine) waitDownload(ctx context.Context) error {
	for e.DownloadStatus() != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return ctx.Err()
}

func (e *Engine) DeleteJob(id int64) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	j, err := e.st.GetJob(id)
	if err != nil {
		return err
	}
	if j.Status == "running" || j.Status == "queued" {
		return fmt.Errorf("请先取消任务，等待结束后再删除记录")
	}
	return e.st.DeleteJob(id)
}
