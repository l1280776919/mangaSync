package source

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// JM 通过 Python 桥接脚本（jmcomic 库）实现，禁漫的接口加解密与图片乱序解码交给它
type JM struct {
	venv         string
	bridge       string
	proxy        string
	imageWorkers int
	photoWorkers int
}

func NewJM(venv, bridge, proxy string, imageWorkers, photoWorkers int) *JM {
	return &JM{venv: venv, bridge: bridge, proxy: proxy,
		imageWorkers: max(1, imageWorkers), photoWorkers: max(1, photoWorkers)}
}

func (j *JM) Kind() string { return "jm" }

func (j *JM) env(cred *Cred) []string {
	e := append([]string{}, os.Environ()...)
	if cred != nil && cred.Token != "" {
		e = append(e, "JM_COOKIES="+cred.Token)
	}
	e = append(e, "JM_PROXY="+j.proxy,
		"JM_IMAGE_WORKERS="+strconv.Itoa(j.imageWorkers),
		"JM_PHOTO_WORKERS="+strconv.Itoa(j.photoWorkers),
		"PYTHONIOENCODING=utf-8")
	return e
}

func (j *JM) run(ctx context.Context, cred *Cred, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, j.venv, append([]string{j.bridge}, args...)...)
	cmd.Env = j.env(cred)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "登录") || strings.Contains(msg, "未登录") || strings.Contains(msg, "unauthorized") {
			return nil, ErrAuth
		}
		return nil, fmt.Errorf("%s", truncate(lastLines(msg, 3), 400))
	}
	return out.Bytes(), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

func (j *JM) runJSON(ctx context.Context, cred *Cred, out any, args ...string) error {
	b, err := j.run(ctx, cred, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes.TrimSpace(b), out); err != nil {
		return fmt.Errorf("桥接输出解析失败: %s", truncate(string(b), 200))
	}
	return nil
}

func (j *JM) Ping(ctx context.Context) error {
	var r struct {
		OK     bool   `json:"ok"`
		Domain string `json:"domain"`
	}
	return j.runJSON(ctx, nil, &r, "ping")
}

func (j *JM) Login(ctx context.Context, username, password string) (*AccountInfo, error) {
	var r struct {
		Cookies        map[string]string `json:"cookies"`
		Username       string            `json:"username"`
		Nickname       string            `json:"nickname"`
		UID            string            `json:"uid"`
		FavoritesCount int               `json:"favoritesCount"`
		FavoritesMax   int               `json:"favoritesMax"`
		Error          string            `json:"error"`
	}
	if err := j.runJSON(ctx, nil, &r, "login", username, password); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("%s", r.Error)
	}
	raw, _ := json.Marshal(r.Cookies)
	nick := r.Nickname
	if nick == "" {
		nick = r.Username
	}
	return &AccountInfo{
		Nickname: nick, FavoritesCount: r.FavoritesCount, FavoritesMax: r.FavoritesMax,
		UID: r.UID, Token: string(raw),
	}, nil
}

func (j *JM) Profile(ctx context.Context, cred *Cred) (*AccountInfo, error) {
	// 禁漫没有独立 profile 接口，用收藏列表是否可读来判断登录态
	if _, err := j.Favorites(ctx, cred); err != nil {
		return nil, err
	}
	return &AccountInfo{Nickname: cred.Username}, nil
}

func (j *JM) Favorites(ctx context.Context, cred *Cred) ([]*Comic, error) {
	var r struct {
		Items []struct {
			ComicID string `json:"comicId"`
			Title   string `json:"title"`
		} `json:"items"`
		Error string `json:"error"`
	}
	if err := j.runJSON(ctx, cred, &r, "favorites"); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("%s", r.Error)
	}
	out := make([]*Comic, 0, len(r.Items))
	for _, it := range r.Items {
		out = append(out, &Comic{
			Kind: "jm", ComicID: it.ComicID, Title: it.Title,
			Categories: []string{}, Tags: []string{},
			Cover: fmt.Sprintf("/api/comics/jm/%s/cover", it.ComicID),
		})
	}
	return out, nil
}

func (j *JM) Search(ctx context.Context, cred *Cred, keyword string, page, pageSize int, sortBy string) (*SearchResult, error) {
	if sortBy == "" {
		sortBy = "mr"
	}
	var r struct {
		Total int `json:"total"`
		Items []struct {
			ComicID  string   `json:"comicId"`
			Title    string   `json:"title"`
			Author   string   `json:"author"`
			Category string   `json:"category"`
			Tags     []string `json:"tags"`
		} `json:"items"`
		Error string `json:"error"`
	}
	if err := j.runJSON(ctx, cred, &r, "search", keyword, strconv.Itoa(page), sortBy); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("%s", r.Error)
	}
	res := &SearchResult{Total: r.Total, Page: page, PageSize: len(r.Items)}
	for _, it := range r.Items {
		cats := []string{}
		if it.Category != "" {
			cats = append(cats, it.Category)
		}
		res.Items = append(res.Items, &Comic{
			Kind: "jm", ComicID: it.ComicID, Title: it.Title, Author: it.Author,
			Categories: cats, Tags: orEmpty(it.Tags),
			Cover: fmt.Sprintf("/api/comics/jm/%s/cover", it.ComicID),
		})
	}
	return res, nil
}

func (j *JM) Detail(ctx context.Context, cred *Cred, comicID string) (*Comic, error) {
	var r struct {
		ComicID     string   `json:"comicId"`
		Title       string   `json:"title"`
		Author      string   `json:"author"`
		Category    string   `json:"category"`
		Tags        []string `json:"tags"`
		Description string   `json:"description"`
		Chapters    []struct {
			Order  int    `json:"order"`
			ID     string `json:"id"`
			Title  string `json:"title"`
			Images int    `json:"images"`
		} `json:"chapters"`
		Error string `json:"error"`
	}
	if err := j.runJSON(ctx, cred, &r, "detail", comicID); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("%s", r.Error)
	}
	cats := []string{}
	if r.Category != "" {
		cats = append(cats, r.Category)
	}
	c := &Comic{
		Kind: "jm", ComicID: r.ComicID, Title: r.Title, Author: r.Author,
		Categories: cats, Tags: orEmpty(r.Tags), Description: r.Description,
		Cover: fmt.Sprintf("/api/comics/jm/%s/cover", r.ComicID),
	}
	for _, ch := range r.Chapters {
		c.Chapters = append(c.Chapters, Chapter{Order: ch.Order, ID: ch.ID, Title: ch.Title, Images: ch.Images})
	}
	sortChapters(c.Chapters)
	c.ChaptersCount = len(c.Chapters)
	return c, nil
}

func (j *JM) AddFavorite(ctx context.Context, cred *Cred, comicID string) error {
	return j.favOp(ctx, cred, "addfavorite", comicID)
}

func (j *JM) DelFavorite(ctx context.Context, cred *Cred, comicID string) error {
	return j.favOp(ctx, cred, "delfavorite", comicID)
}

func (j *JM) favOp(ctx context.Context, cred *Cred, cmd, comicID string) error {
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := j.runJSON(ctx, cred, &r, cmd, comicID); err != nil {
		return err
	}
	if r.Error != "" {
		return fmt.Errorf("%s", r.Error)
	}
	return nil
}

func (j *JM) Cover(ctx context.Context, cred *Cred, comicID string) ([]byte, string, error) {
	tmp, err := os.CreateTemp("", "jmcover-*")
	if err != nil {
		return nil, "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	var r struct {
		Path        string `json:"path"`
		Bytes       int    `json:"bytes"`
		ContentType string `json:"contentType"`
		Error       string `json:"error"`
	}
	if err := j.runJSON(ctx, cred, &r, "cover", comicID, tmp.Name()); err != nil {
		return nil, "", err
	}
	if r.Error != "" {
		return nil, "", fmt.Errorf("%s", r.Error)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, "", err
	}
	ct := r.ContentType
	if ct == "" {
		ct = "image/jpeg"
	}
	return b, ct, nil
}

// Download 通过桥接脚本流式下载，逐行 JSON 事件映射成 hooks
func (j *JM) Download(ctx context.Context, cred *Cred, comicID string, orders []int, dstRoot string, h Hooks) (*Result, error) {
	args := []string{"download", comicID, dstRoot}
	if len(orders) > 0 {
		ss := make([]string, 0, len(orders))
		for _, o := range orders {
			ss = append(ss, strconv.Itoa(o))
		}
		args = append(args, "--chapters", strings.Join(ss, ","))
	}
	cmd := exec.CommandContext(ctx, j.venv, append([]string{j.bridge}, args...)...)
	cmd.Env = j.env(cred)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	res := &Result{Path: filepath.Join(dstRoot, comicID)}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	prog := Progress{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev["event"] {
		case "start":
			prog.ChaptersTotal = intOf(ev["chaptersTotal"])
			prog.ImagesTotal = intOf(ev["imagesTotal"])
			if t, ok := ev["title"].(string); ok {
				res.Title = t
			}
			h.prog(prog)
		case "chapter":
			st := ChapterState{
				Order: intOf(ev["order"]),
				Title: strOf(ev["title"]),
				State: strOf(ev["state"]),
				Images: intOf(ev["images"]),
				Path:  strOf(ev["path"]),
				Err:   strOf(ev["msg"]),
			}
			if st.State == "done" {
				prog.ChaptersDone++
				res.ChaptersDone++
			}
			if st.State == "done" && st.Path != "" {
				res.Path = filepath.Dir(st.Path)
			}
			h.chap(st)
		case "progress":
			prog.CurrentChapter = strOf(ev["title"])
			prog.ImagesDone = intOf(ev["imagesDone"])
			if v := intOf(ev["imagesTotal"]); v > 0 {
				prog.ImagesTotal = v
			}
			prog.Bytes = int64Of(ev["bytes"])
			prog.SpeedBps = int64Of(ev["speedBps"])
			h.prog(prog)
		case "done":
			if p := strOf(ev["path"]); p != "" {
				res.Path = p
			}
			res.Images = intOf(ev["images"])
			res.Bytes = int64Of(ev["bytes"])
			h.prog(prog)
		case "error":
			h.log("error", "桥接报错: "+strOf(ev["msg"]))
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if waitErr != nil {
		msg := lastLines(errb.String(), 3)
		if msg == "" {
			msg = waitErr.Error()
		}
		return res, fmt.Errorf("%s", truncate(msg, 300))
	}
	if res.Images == 0 {
		// 事件里没带总数时，落盘统计兜底
		n, b := dirStats(res.Path)
		res.Images, res.Bytes = n, b
	}
	return res, nil
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	}
	return 0
}

func int64Of(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int:
		return int64(t)
	}
	return 0
}

func dirStats(dir string) (int, int64) {
	var n int
	var b int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if imgExts[strings.ToLower(filepath.Ext(fi.Name()))] {
			n++
			b += fi.Size()
		}
		return nil
	})
	return n, b
}

var _ = time.Second
