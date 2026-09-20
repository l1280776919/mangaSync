package source

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	picaHost      = "https://picaapi.picacomic.com/"
	picaAPIKey    = "C69BAF41DA5ABD1FFEDC6D2FEA56B"
	picaNonce     = "ptxdhmjzqtnrtwndhbxcpkjamb33w837"
	picaDigestKey = "~d}$Q7$eIni=V)9\\RK/P.RM4;9[7|@/CA}b~OW!3?EV`:<>M7pddUBL5n|0/*Cn"
)

type ImageResp struct {
	OriginalName string `json:"originalName"`
	Path         string `json:"path"`
	FileServer   string `json:"fileServer"`
}

func (i ImageResp) URL() string {
	fs := strings.TrimRight(i.FileServer, "/")
	if fs == "" || i.Path == "" {
		return ""
	}
	return fs + "/static/" + strings.TrimLeft(i.Path, "/")
}

type Pica struct {
	proxy        string
	quality      string
	imageWorkers int
	api          *http.Client
	img          *http.Client
}

func NewPica(proxy, quality string, imageWorkers int) *Pica {
	return &Pica{
		proxy:        proxy,
		quality:      quality,
		imageWorkers: max(1, imageWorkers),
		api:          newHTTPClient(proxy, 30*time.Second),
		img:          newHTTPClient(proxy, 120*time.Second),
	}
}

func newHTTPClient(proxy string, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     60 * time.Second,
	}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func (p *Pica) Kind() string { return "pica" }

func picaSignature(path, method, ts string) string {
	msg := strings.ToLower(path + ts + picaNonce + method + picaAPIKey)
	mac := hmac.New(sha256.New, []byte(picaDigestKey))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// apiCall 发一次带签名的请求，返回 data 字段
func (p *Pica) apiCall(ctx context.Context, method, path, token string, body any) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		var rdr io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, picaHost+path, rdr)
		if err != nil {
			return nil, err
		}
		h := req.Header
		h.Set("api-key", picaAPIKey)
		h.Set("accept", "application/vnd.picacomic.com.v1+json")
		h.Set("app-channel", "2")
		h.Set("time", ts)
		h.Set("nonce", picaNonce)
		h.Set("app-version", "2.2.1.2.3.3")
		h.Set("app-uuid", "defaultUuid")
		h.Set("app-platform", "android")
		h.Set("app-build-version", "44")
		h.Set("Content-Type", "application/json; charset=UTF-8")
		h.Set("User-Agent", "okhttp/3.8.1")
		h.Set("authorization", token)
		h.Set("image-quality", p.quality)
		h.Set("signature", picaSignature(path, method, ts))

		resp, err := p.api.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 1500 * time.Millisecond)
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == 401 {
			return nil, ErrAuth
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 1500 * time.Millisecond)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
		}
		var env struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Error   string          `json:"error"`
			Detail  string          `json:"detail"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("响应不是 JSON: %s", truncate(string(raw), 150))
		}
		if env.Code == 401 || env.Error == "1005" {
			return nil, ErrAuth
		}
		if env.Code != 200 {
			return nil, fmt.Errorf("code=%d %s %s", env.Code, env.Message, env.Detail)
		}
		return env.Data, nil
	}
	return nil, fmt.Errorf("请求失败: %v", lastErr)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------- 业务接口 ----------

func (p *Pica) Login(ctx context.Context, username, password string) (*AccountInfo, error) {
	data, err := p.apiCall(ctx, "POST", "auth/sign-in", "", map[string]string{"email": username, "password": password})
	if err != nil {
		return nil, err
	}
	var d struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	info, err := p.Profile(ctx, &Cred{Token: d.Token})
	if err != nil {
		return &AccountInfo{Token: d.Token}, nil
	}
	info.Token = d.Token
	return info, nil
}

func (p *Pica) Profile(ctx context.Context, cred *Cred) (*AccountInfo, error) {
	data, err := p.apiCall(ctx, "GET", "users/profile", cred.Token, nil)
	if err != nil {
		return nil, err
	}
	var d struct {
		User struct {
			Name           string `json:"name"`
			Level          int    `json:"level"`
			FavouriteCount int    `json:"favouriteCount"`
			FavouriteMax   int    `json:"favouriteMax"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	info := &AccountInfo{Nickname: d.User.Name, Level: d.User.Level}
	// 哔咔 profile 不带收藏数，用收藏接口第一页的 total 顶上
	if pg, err := p.favouritesPage(ctx, cred, 1); err == nil {
		info.FavoritesCount = pg.Total
	}
	return info, nil
}

type picaPagination struct {
	Total int             `json:"total"`
	Limit int             `json:"limit"`
	Page  int             `json:"page"`
	Pages int             `json:"pages"`
	Docs  json.RawMessage `json:"docs"`
}

func (p *Pica) favouritesPage(ctx context.Context, cred *Cred, page int) (*picaPagination, error) {
	data, err := p.apiCall(ctx, "GET", fmt.Sprintf("users/favourite?s=dd&page=%d", page), cred.Token, nil)
	if err != nil {
		return nil, err
	}
	var d struct {
		Comics picaPagination `json:"comics"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return &d.Comics, nil
}

func (p *Pica) Favorites(ctx context.Context, cred *Cred) ([]*Comic, error) {
	var out []*Comic
	for page := 1; page <= 200; page++ {
		pg, err := p.favouritesPage(ctx, cred, page)
		if err != nil {
			return nil, err
		}
		var docs []struct {
			ID         string    `json:"_id"`
			Title      string    `json:"title"`
			Author     string    `json:"author"`
			PagesCount int       `json:"pagesCount"`
			EpsCount   int       `json:"epsCount"`
			Categories []string  `json:"categories"`
			Tags       []string  `json:"tags"`
			Thumb      ImageResp `json:"thumb"`
		}
		if err := json.Unmarshal(pg.Docs, &docs); err != nil {
			return nil, err
		}
		for _, d := range docs {
			out = append(out, &Comic{
				Kind:          "pica",
				ComicID:       d.ID,
				Title:         d.Title,
				Author:        d.Author,
				Categories:    orEmpty(d.Categories),
				Tags:          orEmpty(d.Tags),
				ChaptersCount: d.EpsCount,
				Cover:         fmt.Sprintf("/api/comics/pica/%s/cover", d.ID),
			})
		}
		if pg.Pages <= page || len(docs) == 0 {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	return out, nil
}

func (p *Pica) Search(ctx context.Context, cred *Cred, keyword string, page, pageSize int, sort string) (*SearchResult, error) {
	if sort == "" {
		sort = "dd"
	}
	token := ""
	if cred != nil {
		token = cred.Token
	}
	path := fmt.Sprintf("comics/advanced-search?page=%d", page)
	data, err := p.apiCall(ctx, "POST", path, token, map[string]any{
		"keyword":    keyword,
		"sort":       sort,
		"categories": []string{},
	})
	if err != nil {
		return nil, err
	}
	var d struct {
		Comics picaPagination `json:"comics"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	var docs []struct {
		ID         string    `json:"_id"`
		Title      string    `json:"title"`
		Author     string    `json:"author"`
		EpsCount   int       `json:"epsCount"`
		Categories []string  `json:"categories"`
		Tags       []string  `json:"tags"`
		Thumb      ImageResp `json:"thumb"`
	}
	if err := json.Unmarshal(d.Comics.Docs, &docs); err != nil {
		return nil, err
	}
	res := &SearchResult{Total: d.Comics.Total, Page: page, PageSize: len(docs)}
	for _, x := range docs {
		res.Items = append(res.Items, &Comic{
			Kind: "pica", ComicID: x.ID, Title: x.Title, Author: x.Author,
			Categories: orEmpty(x.Categories), Tags: orEmpty(x.Tags),
			ChaptersCount: x.EpsCount,
			Cover:         fmt.Sprintf("/api/comics/pica/%s/cover", x.ID),
		})
	}
	return res, nil
}

func (p *Pica) Detail(ctx context.Context, cred *Cred, comicID string) (*Comic, error) {
	token := ""
	if cred != nil {
		token = cred.Token
	}
	data, err := p.apiCall(ctx, "GET", "comics/"+comicID, token, nil)
	if err != nil {
		return nil, err
	}
	var d struct {
		Comic struct {
			ID          string    `json:"_id"`
			Title       string    `json:"title"`
			Author      string    `json:"author"`
			Description string    `json:"description"`
			EpsCount    int       `json:"epsCount"`
			Categories  []string  `json:"categories"`
			Tags        []string  `json:"tags"`
			Thumb       ImageResp `json:"thumb"`
		} `json:"comic"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	c := &Comic{
		Kind: "pica", ComicID: d.Comic.ID, Title: d.Comic.Title, Author: d.Comic.Author,
		Description: d.Comic.Description, Categories: orEmpty(d.Comic.Categories),
		Tags: orEmpty(d.Comic.Tags), ChaptersCount: d.Comic.EpsCount,
		Cover: fmt.Sprintf("/api/comics/pica/%s/cover", d.Comic.ID),
	}
	chs, err := p.chapters(ctx, cred, comicID)
	if err != nil {
		return c, nil // 详情拿到了，章节失败不致命
	}
	c.Chapters = chs
	c.ChaptersCount = len(chs)
	return c, nil
}

func (p *Pica) chapters(ctx context.Context, cred *Cred, comicID string) ([]Chapter, error) {
	token := ""
	if cred != nil {
		token = cred.Token
	}
	type chapterDoc struct {
		ID    string `json:"_id"`
		Title string `json:"title"`
		Order int    `json:"order"`
	}
	var all []chapterDoc
	for page := 1; page <= 100; page++ {
		data, err := p.apiCall(ctx, "GET", fmt.Sprintf("comics/%s/eps?page=%d", comicID, page), token, nil)
		if err != nil {
			return nil, err
		}
		var d struct {
			Eps picaPagination `json:"eps"`
		}
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		var docs []chapterDoc
		if err := json.Unmarshal(d.Eps.Docs, &docs); err != nil {
			return nil, err
		}
		all = append(all, docs...)
		if d.Eps.Pages <= page || len(docs) == 0 {
			break
		}
	}
	out := make([]Chapter, 0, len(all))
	for _, c := range all {
		out = append(out, Chapter{Order: c.Order, ID: c.ID, Title: c.Title})
	}
	sortChapters(out)
	return out, nil
}

func (p *Pica) AddFavorite(ctx context.Context, cred *Cred, comicID string) error {
	action, err := p.toggleFavorite(ctx, cred, comicID)
	if err != nil {
		return err
	}
	if action == "un_favourite" {
		if _, err = p.toggleFavorite(ctx, cred, comicID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pica) DelFavorite(ctx context.Context, cred *Cred, comicID string) error {
	action, err := p.toggleFavorite(ctx, cred, comicID)
	if err != nil {
		return err
	}
	if action == "favourite" {
		if _, err = p.toggleFavorite(ctx, cred, comicID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pica) toggleFavorite(ctx context.Context, cred *Cred, comicID string) (string, error) {
	data, err := p.apiCall(ctx, "POST", fmt.Sprintf("comics/%s/favourite", comicID), cred.Token, map[string]any{})
	if err != nil {
		return "", err
	}
	var d struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(data, &d)
	return d.Action, nil
}

// Cover 取封面原图（thumb 有两种拼法，依次尝试）
func (p *Pica) Cover(ctx context.Context, cred *Cred, comicID string) ([]byte, string, error) {
	token := ""
	if cred != nil {
		token = cred.Token
	}
	data, err := p.apiCall(ctx, "GET", "comics/"+comicID, token, nil)
	if err != nil {
		return nil, "", err
	}
	var d struct {
		Comic struct {
			Thumb ImageResp `json:"thumb"`
		} `json:"comic"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, "", err
	}
	fs := strings.TrimRight(d.Comic.Thumb.FileServer, "/")
	path := d.Comic.Thumb.Path
	if fs == "" || path == "" {
		return nil, "", fmt.Errorf("该漫画没有封面")
	}
	cands := []string{fs + "/static/" + path}
	if parts := strings.Split(path, "/"); len(parts) >= 3 {
		cands = append(cands, fmt.Sprintf("%s/static/%s/%s/%s", fs, parts[0], parts[1], parts[len(parts)-1]))
	}
	var lastErr error
	for _, u := range cands {
		b, ct, err := p.fetchImage(ctx, u, "", "")
		if err == nil {
			return b, ct, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

// ---------- 下载 ----------

type picaPageDoc struct {
	Media ImageResp `json:"media"`
}

// fetchImage 下载单张图片并校验确实是图片
func (p *Pica) fetchImage(ctx context.Context, u string, _ string, _ string) ([]byte, string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "okhttp/3.8.1")
		resp, err := p.img.Do(req)
		if err == nil && resp.StatusCode == 403 {
			resp.Body.Close()
			req2, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
			req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
			req2.Header.Set("Referer", "https://manhuabika.com/")
			resp, err = p.img.Do(req2)
		}
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 800 * time.Millisecond)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		ct := resp.Header.Get("Content-Type")
		resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 800 * time.Millisecond)
			continue
		}
		if !isImage(b) {
			lastErr = fmt.Errorf("返回的不是图片(%d 字节)", len(b))
			continue
		}
		return b, ct, nil
	}
	return nil, "", fmt.Errorf("下载失败 %s: %v", u, lastErr)
}

// chapterImageURLs 取某章的全部图片链接（按页序合并）
func (p *Pica) chapterImageURLs(ctx context.Context, cred *Cred, comicID string, order int) ([]string, error) {
	token := ""
	if cred != nil {
		token = cred.Token
	}
	getPage := func(page int) (*picaPagination, error) {
		data, err := p.apiCall(ctx, "GET",
			fmt.Sprintf("comics/%s/order/%d/pages?page=%d", comicID, order, page), token, nil)
		if err != nil {
			return nil, err
		}
		var d struct {
			Pages picaPagination `json:"pages"`
		}
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return &d.Pages, nil
	}
	first, err := getPage(1)
	if err != nil {
		return nil, err
	}
	type pageResp struct {
		page int
		docs []picaPageDoc
	}
	results := []pageResp{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var firstErr error
	collect := func(pg int, docs []picaPageDoc) {
		mu.Lock()
		results = append(results, pageResp{pg, docs})
		mu.Unlock()
	}
	var docs0 []picaPageDoc
	if err := json.Unmarshal(first.Docs, &docs0); err != nil {
		return nil, err
	}
	collect(1, docs0)
	for pg := 2; pg <= first.Pages; pg++ {
		wg.Add(1)
		go func(pg int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pr, err := getPage(pg)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			var docs []picaPageDoc
			if err := json.Unmarshal(pr.Docs, &docs); err != nil {
				return
			}
			collect(pg, docs)
		}(pg)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	// 按页码排序后展平
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].page < results[i].page {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	var urls []string
	for _, r := range results {
		for _, d := range r.docs {
			if u := d.Media.URL(); u != "" {
				urls = append(urls, u)
			}
		}
	}
	return urls, nil
}

// Download 下载整本（orders 为空）或指定章节
func (p *Pica) Download(ctx context.Context, cred *Cred, comicID string, orders []int, dstRoot string, h Hooks) (*Result, error) {
	detail, err := p.Detail(ctx, cred, comicID)
	if err != nil {
		return nil, err
	}
	if len(detail.Chapters) == 0 {
		return nil, fmt.Errorf("该漫画没有章节")
	}
	selected := detail.Chapters
	if len(orders) > 0 {
		want := map[int]bool{}
		for _, o := range orders {
			want[o] = true
		}
		selected = nil
		for _, c := range detail.Chapters {
			if want[c.Order] {
				selected = append(selected, c)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("指定的章节都不存在")
		}
	}

	cdir := comicDir(dstRoot, detail)
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		return nil, err
	}
	p.saveCoverAndMeta(ctx, cred, detail, cdir)

	total := Progress{ChaptersTotal: len(selected)}
	res := &Result{Path: cdir, Title: detail.Title}
	var failedChapters []string
	h.log("info", fmt.Sprintf("开始下载《%s》共 %d 章 → %s", detail.Title, len(selected), cdir))

	for _, ch := range selected {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		cname := chapterDirName(ch)
		cpath := filepath.Join(cdir, cname)
		h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "start"})

		urls, err := p.chapterImageURLs(ctx, cred, comicID, ch.Order)
		if err != nil {
			failedChapters = append(failedChapters, fmt.Sprintf("%d %s(取图失败)", ch.Order, ch.Title))
			h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "failed", Err: err.Error()})
			h.log("error", fmt.Sprintf("章节 %d 取图失败: %v", ch.Order, err))
			continue
		}
		if len(urls) == 0 {
			failedChapters = append(failedChapters, fmt.Sprintf("%d %s(无图片)", ch.Order, ch.Title))
			h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "failed", Err: "该章没有图片"})
			continue
		}
		// 已完整则跳过
		if n, b := dirImageStats(cpath); n >= len(urls) {
			h.log("info", fmt.Sprintf("  %s 已存在且完整(%d 张)，跳过", cname, n))
			total.ChaptersDone++
			total.ImagesDone += n
			total.Bytes += b
			res.Images += n
			res.Bytes += b
			res.ChaptersDone++
			h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "done", Images: n, Path: cpath})
			h.prog(total)
			continue
		}

		tmp := filepath.Join(cdir, "."+cname+".part")
		os.RemoveAll(tmp)
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			return res, err
		}
		n, b, err := p.downloadImages(ctx, urls, tmp, h, total)
		if err != nil {
			os.RemoveAll(tmp)
			failedChapters = append(failedChapters, fmt.Sprintf("%d %s", ch.Order, ch.Title))
			h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "failed", Err: err.Error()})
			h.log("error", fmt.Sprintf("章节 %d 下载失败: %v", ch.Order, err))
			continue
		}
		os.RemoveAll(cpath)
		if err := os.Rename(tmp, cpath); err != nil {
			os.RemoveAll(tmp)
			h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "failed", Err: err.Error()})
			continue
		}
		total.ChaptersDone++
		total.ImagesDone += n
		total.Bytes += b
		res.Images += n
		res.Bytes += b
		res.ChaptersDone++
		h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "done", Images: n, Path: cpath})
		h.prog(total)
		h.log("info", fmt.Sprintf("  %s · %d 张 %s ✓", cname, n, humanBytes(b)))
	}
	if len(failedChapters) > 0 {
		return res, fmt.Errorf("%d 章失败: %s", len(failedChapters), strings.Join(failedChapters, ", "))
	}
	return res, nil
}

func (p *Pica) downloadImages(ctx context.Context, urls []string, dir string, h Hooks, base Progress) (int, int64, error) {
	workers := min(p.imageWorkers, len(urls))
	type job struct {
		idx int
		url string
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var imagesDone, imagesTotal = 0, 0
	var bytesDone int64
	var failed []string
	start := time.Now()

	write := func(idx int, url string) error {
		b, _, err := p.fetchImage(ctx, url, "", "")
		if err != nil {
			return err
		}
		ext := imgExtFromURL(url)
		dst := filepath.Join(dir, fmt.Sprintf("%03d%s", idx, ext))
		tmp := dst + ".part"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
		mu.Lock()
		imagesDone++
		bytesDone += int64(len(b))
		mu.Unlock()
		return nil
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := ctx.Err(); err != nil {
					return
				}
				if err := write(j.idx, j.url); err != nil {
					mu.Lock()
					failed = append(failed, err.Error())
					mu.Unlock()
				}
			}
		}()
	}
	// 进度上报协程
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(700 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				mu.Lock()
				pd, bt := imagesDone, bytesDone
				mu.Unlock()
				el := time.Since(start).Seconds()
				cur := base
				cur.ImagesDone = pd
				cur.ImagesTotal = imagesTotal
				cur.Bytes = bt
				if el > 0.5 {
					cur.SpeedBps = int64(float64(bt) / el)
				}
				h.prog(cur)
			}
		}
	}()
	for i, u := range urls {
		jobs <- job{idx: i + 1, url: u}
	}
	close(jobs)
	wg.Wait()
	close(stop)

	mu.Lock()
	defer mu.Unlock()
	if len(failed) > 0 || imagesDone != len(urls) {
		msg := fmt.Sprintf("%d/%d 张下载失败", len(failed), len(urls))
		if len(failed) > 0 {
			msg += ": " + truncate(failed[0], 150)
		}
		return imagesDone, bytesDone, fmt.Errorf("%s", msg)
	}
	return imagesDone, bytesDone, nil
}

func (p *Pica) saveCoverAndMeta(ctx context.Context, cred *Cred, c *Comic, cdir string) {
	cover := filepath.Join(cdir, "cover.jpg")
	if _, err := os.Stat(cover); err != nil {
		if b, _, err := p.Cover(ctx, cred, c.ComicID); err == nil {
			_ = os.WriteFile(cover, b, 0o644)
		}
	}
	meta := filepath.Join(cdir, "metadata.json")
	if _, err := os.Stat(meta); err == nil {
		return
	}
	type metaCh struct {
		Order int    `json:"order"`
		Title string `json:"title"`
		ID    string `json:"id"`
	}
	mc := []metaCh{}
	for _, ch := range c.Chapters {
		mc = append(mc, metaCh{ch.Order, ch.Title, ch.ID})
	}
	b, _ := json.MarshalIndent(map[string]any{
		"id": c.ComicID, "title": c.Title, "author": c.Author,
		"categories": c.Categories, "tags": c.Tags, "chapters": mc,
		"updatedAt": time.Now().Format(time.RFC3339),
	}, "", "  ")
	_ = os.WriteFile(meta, b, 0o644)
}

// ---------- 工具 ----------

var (
	reBadChars = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)
	reSpaces   = regexp.MustCompile(`\s+`)
)

func sanitize(name string, maxLen int) string {
	s := reBadChars.ReplaceAllString(name, "_")
	s = reSpaces.ReplaceAllString(s, " ")
	s = strings.Trim(s, " .")
	if len(s) > maxLen {
		s = strings.TrimSpace(s[:maxLen])
	}
	if s == "" {
		return "untitled"
	}
	return s
}

// comicDir 决定漫画目录：标题重名（且不是同一本）时补 id 后缀
func comicDir(dstRoot string, c *Comic) string {
	base := sanitize(c.Title, 90)
	dir := filepath.Join(dstRoot, base)
	if existing, ok := readMetaID(dir); ok && existing != c.ComicID {
		short := c.ComicID
		if len(short) > 6 {
			short = short[len(short)-6:]
		}
		dir = filepath.Join(dstRoot, base+" ["+short+"]")
	}
	return dir
}

func readMetaID(dir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return "", false
	}
	var m struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(b, &m) != nil || m.ID == "" {
		return "", false
	}
	return m.ID, true
}

func chapterDirName(ch Chapter) string {
	title := ch.Title
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("第%d话", ch.Order)
	}
	return fmt.Sprintf("%03d - %s", ch.Order, sanitize(title, 70))
}

var imgExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}

func dirImageStats(dir string) (int, int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	n, b := 0, int64(0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if imgExts[strings.ToLower(filepath.Ext(e.Name()))] {
			n++
			if fi, err := e.Info(); err == nil {
				b += fi.Size()
			}
		}
	}
	return n, b
}

func isImage(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	switch {
	case b[0] == 0xFF && b[1] == 0xD8:
		return true
	case string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return true
	case string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a":
		return true
	case string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return true
	}
	return false
}

func imgExtFromURL(u string) string {
	low := strings.ToLower(u)
	if i := strings.IndexAny(low, "?#"); i >= 0 {
		low = low[:i]
	}
	for _, e := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(low, e) {
			if e == ".jpeg" {
				return ".jpg"
			}
			return e
		}
	}
	return ".jpg"
}

func humanBytes(n int64) string {
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

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func sortChapters(cs []Chapter) {
	for i := 0; i < len(cs); i++ {
		for j := i + 1; j < len(cs); j++ {
			if cs[j].Order < cs[i].Order {
				cs[i], cs[j] = cs[j], cs[i]
			}
		}
	}
}
