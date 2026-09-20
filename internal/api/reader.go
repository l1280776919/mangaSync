package api

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/l1280776919/mangaSync/internal/store"
)

// 在线阅读接口：
//
//	GET /api/reader/{kind}/{comicId}/{order}/meta        章节目录（页数、章节名、是否已下载到本地）
//	GET /api/reader/{kind}/{comicId}/{order}/page/{page} 单页图片（已还原乱序，可直接看）
//
// 取图顺序：本地已下载文件 → 阅读器缓存 → 回源（禁漫的乱序还原在这一步）
// 本地命中时不碰网络，翻页几乎瞬时。

func (s *Server) readerMeta(w http.ResponseWriter, r *http.Request) {
	kind, comicID := r.PathValue("kind"), r.PathValue("comicId")
	order, _ := strconv.Atoi(r.PathValue("order"))
	if order < 1 {
		writeErr(w, 400, "章节序号不合法")
		return
	}

	resp := map[string]any{"kind": kind, "comicId": comicID, "order": order, "pages": 0, "local": false}

	// 本地优先：已下载的章节直接数文件，不走网络
	if dir, ok := s.localChapterDir(kind, comicID, order); ok {
		if files := imageFilesIn(dir); len(files) > 0 {
			resp["pages"] = len(files)
			resp["local"] = true
			resp["dir"] = dir
			resp["sizes"] = sizesOf(files)
			writeJSON(w, 200, resp)
			return
		}
	}

	// 阅读器缓存：本地没有但缓存过，也能给出像素尺寸（前端可精确占位）
	if sizes := s.cachedSizes(kind, comicID, order); len(sizes) > 0 {
		resp["sizes"] = sizes
	}

	src, err := s.eng.SourceFor(kind)
	if err != nil {
		writeJSON(w, 200, resp) // 无账号也能读本地，这里不报错
		return
	}
	pages, title, err := src.Pages(r.Context(), s.bestCred(nil, kind), comicID, order)
	if err != nil {
		resp["error"] = err.Error()
		writeJSON(w, 200, resp)
		return
	}
	resp["pages"] = pages
	resp["chapterTitle"] = title
	writeJSON(w, 200, resp)
}

func (s *Server) readerPage(w http.ResponseWriter, r *http.Request) {
	kind, comicID := r.PathValue("kind"), r.PathValue("comicId")
	order, _ := strconv.Atoi(r.PathValue("order"))
	page, _ := strconv.Atoi(r.PathValue("page"))
	if order < 1 || page < 1 {
		writeErr(w, 400, "参数不合法")
		return
	}
	if page > 5000 {
		writeErr(w, 400, "页码超出上限")
		return
	}

	// 1) 本地已下载的图（零网络、秒开）
	if dir, ok := s.localChapterDir(kind, comicID, order); ok {
		if files := imageFilesIn(dir); page <= len(files) {
			if b, err := os.ReadFile(files[page-1]); err == nil && len(b) > 0 {
				serveReaderImage(w, b, contentTypeByExt(files[page-1]))
				return
			}
		}
	}

	// 2) 阅读器缓存（回源结果落盘，翻页/重看不再重复还原）
	cacheDir := filepath.Join(s.base, "reader", kind, safeName(comicID), fmt.Sprintf("%03d", order))
	for _, ext := range []string{"webp", "jpg", "png", "gif"} {
		p := filepath.Join(cacheDir, fmt.Sprintf("%05d.%s", page, ext))
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			serveReaderImage(w, b, contentTypeByExt(p))
			return
		}
	}

	// 3) 回源
	src, err := s.eng.SourceFor(kind)
	if err != nil {
		writeErr(w, 400, "%v", err)
		return
	}
	var acc *store.Account
	if accID := intQuery(r, "accountId", 0); accID > 0 {
		if a, e := s.st.GetAccount(int64(accID)); e == nil {
			acc = a
		}
	}
	b, ct, err := src.PageImage(r.Context(), s.bestCred(acc, kind), comicID, order, page, true)
	if err != nil {
		writeErr(w, 404, "取图失败: %v", err)
		return
	}
	if len(b) == 0 {
		writeErr(w, 404, "取到空图")
		return
	}
	// 落缓存（失败不影响返回）
	if err := os.MkdirAll(cacheDir, 0o755); err == nil {
		cachePath := filepath.Join(cacheDir, fmt.Sprintf("%05d%s", page, extByContentType(ct)))
		if werr := os.WriteFile(cachePath, b, 0o644); werr != nil {
			// 缓存写不进去就算了，不影响阅读
			_ = werr
		}
	}
	serveReaderImage(w, b, ct)
}

func serveReaderImage(w http.ResponseWriter, b []byte, ct string) {
	if ct == "" {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(200)
	_, _ = w.Write(b)
}

// localChapterDir 找本地章节目录：单章本子就是本子目录；多章按目录名的 %03d 前缀匹配 order。
// 不能再用「排序后的第 N 个子目录」：只要中间有一章下载失败/没下载，order=1 就会返回第 3 章的目录，
// 页数与图片全是别的章节的内容。匹配逻辑与 api.go 的 findChapterDir 保持一致。
func (s *Server) localChapterDir(kind, comicID string, order int) (string, bool) {
	rec, err := s.st.GetComic(kind, comicID)
	if err != nil || rec == nil || strings.TrimSpace(rec.Path) == "" {
		return "", false
	}
	dir := rec.Path
	if !filepath.IsAbs(dir) {
		if root := strings.TrimSpace(s.cfg.Get().DownloadRoot); root != "" {
			dir = filepath.Join(root, dir)
		}
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	if found := findChapterDir(dir, order); found != "" {
		return found, true
	}
	// 单章本：图片直接放在本子目录里（没有章节目录）
	if len(subdirsIn(dir)) == 0 && order == 1 {
		return dir, true
	}
	return "", false
}

// readerImgExts 可当作漫画页的扩展名
var readerImgExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".avif": true, ".bmp": true}

// sizesOf 读取每张图的像素尺寸（读不到写 {0,0}），顺序与 imageFilesIn 一致。
// 前端用它给未加载的页预留占位高度，避免滚动时位置乱跳。
func sizesOf(files []string) [][2]int {
	out := make([][2]int, 0, len(files))
	for _, p := range files {
		w, h := imageSize(p)
		out = append(out, [2]int{w, h})
	}
	return out
}

// cachedSizes 读阅读器缓存目录里各页的尺寸（按页号对齐，缺页为 {0,0}）
func (s *Server) cachedSizes(kind, comicID string, order int) [][2]int {
	dir := filepath.Join(s.base, "reader", kind, safeName(comicID), fmt.Sprintf("%03d", order))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	maxPage := 0
	found := map[int][2]int{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !readerImgExts[ext] {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, filepath.Ext(name)))
		if err != nil || n < 1 {
			continue
		}
		if n > maxPage {
			maxPage = n
		}
		if _, ok := found[n]; !ok {
			w, h := imageSize(filepath.Join(dir, name))
			found[n] = [2]int{w, h}
		}
	}
	if maxPage == 0 {
		return nil
	}
	out := make([][2]int, maxPage)
	for i := 1; i <= maxPage; i++ {
		out[i-1] = found[i]
	}
	return out
}

// imageSize 只读图片头拿宽高（jpeg/png/gif/webp），失败返回 0,0
func imageSize(p string) (int, int) {
	f, err := os.Open(p)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// imageFilesIn 返回目录内图片文件（按名字排序，页码即序号）
func imageFilesIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !readerImgExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out
}

// subdirsIn 返回目录内的子目录（排序，通常是章节目录）
func subdirsIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func contentTypeByExt(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	}
	return "image/webp"
}

func extByContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	}
	return ".webp"
}
