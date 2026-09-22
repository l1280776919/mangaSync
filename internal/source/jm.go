package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// JM 是禁漫（18comic / JMComic）移动端 API 的原生 Go 实现。
// 协议规则见 jm_crypto.go（md5 token + AES-ECB 响应解密），图片乱序还原见 jm_image.go。
// 不再依赖 Python / jmcomic 库。
type JM struct {
	imageWorkers int

	mu            sync.Mutex
	apiDomain     string
	imageDomain   string
	scrambleCache map[string]int
	chapCache     map[string]jmChapEntry // comicID -> 章节表（阅读器按 order 找 photoID）
	imgCache      map[string]jmImgEntry  // photoID -> 图片文件名表

	api *http.Client // API 请求（带超时）
	img *http.Client // 图片下载（大文件，不设整体超时）
}

// NewJM 创建禁漫源；proxy 为空表示直连
func NewJM(proxy string, imageWorkers int) *JM {
	if imageWorkers <= 0 {
		imageWorkers = 8
	}
	return &JM{
		imageWorkers:  imageWorkers,
		apiDomain:     jmAPIDomains[0],
		imageDomain:   jmImageDomains[0],
		scrambleCache: map[string]int{},
		chapCache:     map[string]jmChapEntry{},
		imgCache:      map[string]jmImgEntry{},
		api:           newHTTPClient(proxy, 45*time.Second),
		img:           newHTTPClient(proxy, 0),
	}
}

func (j *JM) Kind() string { return "jm" }

// ------------------------------ 请求底座 ------------------------------

type jmResult struct {
	data    json.RawMessage
	cookies map[string]string
}

// do 发一次请求：自动带 token 头、cookie、域名失败切换、响应解密
func (j *JM) do(ctx context.Context, cred *Cred, method, path string, params, form url.Values, secret string, client *http.Client) (*jmResult, error) {
	var lastErr error
	for _, domain := range j.domainList(j.apiDomain) {
		ts := time.Now().Unix()
		u := "https://" + domain + path
		if len(params) > 0 {
			u += "?" + params.Encode()
		}
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, u, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("user-agent", jmMobileUA)
		req.Header.Set("token", jmToken(ts, secret))
		req.Header.Set("tokenparam", fmt.Sprintf("%d,%s", ts, jmAppVersion))
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if cred != nil {
			if ck := jmCookies(cred.Token); ck != "" {
				req.Header.Set("Cookie", ck)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue // 换个域名再试
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, ErrAuth
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 120))
			continue
		}

		cookies := map[string]string{}
		for _, c := range resp.Cookies() {
			cookies[c.Name] = c.Value
		}
		if jmIsHTML(raw) {
			// /chapter_view_template 之类返回 HTML，不走 JSON 解密
			return &jmResult{data: raw, cookies: cookies}, nil
		}
		var env struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			lastErr = fmt.Errorf("响应不是 JSON: %s", truncate(string(raw), 120))
			continue
		}
		if env.Code != 200 {
			return nil, fmt.Errorf("禁漫接口错误 code=%d %s", env.Code, truncate(string(raw), 120))
		}

		// data 是字符串 → 需要 AES 解密
		var s string
		if err := json.Unmarshal(env.Data, &s); err == nil {
			plain, derr := jmDecodeData(s, ts, jmDataSecret)
			if derr != nil {
				return nil, derr
			}
			return &jmResult{data: plain, cookies: cookies}, nil
		}
		// 已是明文 JSON（如 /setting 的某些情况）
		j.setAPIDomain(domain)
		return &jmResult{data: env.Data, cookies: cookies}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("禁漫接口无可用域名")
	}
	return nil, lastErr
}

func (j *JM) getJSON(ctx context.Context, cred *Cred, path string, params url.Values, out any) error {
	res, err := j.do(ctx, cred, http.MethodGet, path, params, nil, jmTokenSecret, j.api)
	if err != nil {
		return err
	}
	return json.Unmarshal(res.data, out)
}

// flexInt 禁漫接口有的字段是字符串有的数字，统一容忍
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	if i := strings.IndexAny(s, "."); i >= 0 {
		s = s[:i]
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		*n = 0
		return nil // 解析不了就当 0，不让整条响应失败
	}
	*n = flexInt(v)
	return nil
}

func (n flexInt) Int() int { return int(n) }

func (j *JM) domainList(current string) []string {
	out := []string{current}
	for _, d := range jmAPIDomains {
		if d != current {
			out = append(out, d)
		}
	}
	return out
}

func (j *JM) setAPIDomain(d string) {
	j.mu.Lock()
	j.apiDomain = d
	j.mu.Unlock()
}

func jmIsHTML(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return strings.HasPrefix(s, "<") || strings.Contains(s[:min(len(s), 200)], "<html")
}

// jmCookies 把 cookie map 拼成请求头
func jmCookies(token string) string {
	if token == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(token), &m); err != nil {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func jmCookieMap(token string) map[string]string {
	m := map[string]string{}
	if token != "" {
		_ = json.Unmarshal([]byte(token), &m)
	}
	return m
}

// ------------------------------ 账号 ------------------------------

// Login 走移动端 /login（表单明文），成功后把 cookie + AVS 存进 Token
func (j *JM) Login(ctx context.Context, username, password string) (*AccountInfo, error) {
	form := url.Values{"username": {username}, "password": {password}}
	res, err := j.do(ctx, nil, http.MethodPost, "/login", nil, form, jmTokenSecret, j.api)
	if err != nil {
		return nil, err
	}
	var d struct {
		UID               string `json:"uid"`
		Username          string `json:"username"`
		Nickname          string `json:"nickname"`
		Fname             string `json:"fname"`
		S                 string `json:"s"`
		Level             int    `json:"level"`
		AlbumFavorites    int    `json:"album_favorites"`
		AlbumFavoritesMax int    `json:"album_favorites_max"`
	}
	if err := json.Unmarshal(res.data, &d); err != nil {
		return nil, fmt.Errorf("登录响应解析失败: %w", err)
	}
	if d.Username == "" && d.UID == "" {
		return nil, errors.New("账号或密码错误")
	}
	// 移动端要求必须带 cookie（AVS 是登录态签名）
	cookies := res.cookies
	if d.S != "" {
		cookies["AVS"] = d.S
	}
	token, _ := json.Marshal(cookies)

	nick := d.Nickname
	if nick == "" {
		nick = d.Fname
	}
	if nick == "" {
		nick = d.Username
	}
	return &AccountInfo{
		Nickname:       nick,
		Level:          d.Level,
		FavoritesCount: d.AlbumFavorites,
		FavoritesMax:   d.AlbumFavoritesMax,
		UID:            d.UID,
		Token:          string(token),
	}, nil
}

// Profile 刷新收藏数（昵称等级沿用登录时保存的）
func (j *JM) Profile(ctx context.Context, cred *Cred) (*AccountInfo, error) {
	var out struct {
		Total flexInt `json:"total"`
	}
	if err := j.getJSON(ctx, cred, "/favorite", url.Values{
		"page": {"1"}, "folder_id": {"0"}, "o": {"mr"},
	}, &out); err != nil {
		return nil, err
	}
	return &AccountInfo{Nickname: cred.Username, FavoritesCount: out.Total.Int()}, nil
}

// ------------------------------ 收藏 / 搜索 ------------------------------

type jmListItem struct {
	ID          json.Number `json:"id"`
	Name        string      `json:"name"`
	Author      string      `json:"author"`
	LatestEp    *string     `json:"latest_ep"`
	LatestEpAID json.Number `json:"latest_ep_aid"`
	Category    *jmCategory `json:"category"`
	CategorySub *jmCategory `json:"category_sub"`
	UpdateAt    int64       `json:"update_at"`
	Description string      `json:"description"`
}

type jmCategory struct {
	ID    json.Number `json:"id"`
	Title string      `json:"title"`
}

// jmTag 标签可能是字符串也可能带 id/title 的对象，两种都认
type jmTag struct{ Title string }

func (t *jmTag) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err == nil {
			t.Title = s
		}
		return nil
	}
	var o struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(b, &o); err == nil {
		t.Title = o.Title
	}
	return nil
}

func (it jmListItem) toComic() *Comic {
	c := &Comic{
		Kind:    "jm",
		ComicID: it.ID.String(),
		Title:   strings.TrimSpace(it.Name),
		Author:  strings.TrimSpace(it.Author),
		Cover:   "/api/comics/jm/" + it.ID.String() + "/cover",
	}
	for _, cat := range []*jmCategory{it.Category, it.CategorySub} {
		if cat != nil && cat.Title != "" && cat.Title != c.Author {
			c.Categories = append(c.Categories, cat.Title)
		}
	}
	c.Categories = dedupStrings(c.Categories)
	if it.LatestEp != nil {
		c.LatestEp = *it.LatestEp
	}
	return c
}

// Favorites 拉全部收藏（服务端分页，逐页取到 total）
func (j *JM) Favorites(ctx context.Context, cred *Cred) ([]*Comic, error) {
	var out []*Comic
	seen := map[string]bool{}
	// maxPages 只是防跑飞的兜底；正常终止条件跟随服务端返回的 total。
	// 原来固定 30 页：收藏超过 30 页的账号会被静默截断（同步显示「成功」却长期缺书）。
	const maxPages = 200
	for page := 1; page <= maxPages; page++ {
		var res struct {
			List  []jmListItem `json:"list"`
			Total flexInt      `json:"total"`
		}
		if err := j.getJSON(ctx, cred, "/favorite", url.Values{
			"page": {strconv.Itoa(page)}, "folder_id": {"0"}, "o": {"mr"},
		}, &res); err != nil {
			if page == 1 {
				return nil, err
			}
			// 中途出错：返回已取到的部分 + err，调用方据此提示「部分同步」，
			// 而不是把部分结果当成功（原来直接 break，错误被吞掉）
			return out, fmt.Errorf("收藏第 %d 页拉取失败（已取到 %d 本）: %w", page, len(out), err)
		}
		for _, it := range res.List {
			id := it.ID.String()
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, it.toComic())
		}
		if len(res.List) == 0 {
			break
		}
		if total := res.Total.Int(); total > 0 && len(out) >= total {
			break
		}
	}
	return out, nil
}

// Search 官方搜索：/search?search_query=&page=&o=
func (j *JM) Search(ctx context.Context, cred *Cred, keyword string, page, pageSize int, sortKey string) (*SearchResult, error) {
	if page < 1 {
		page = 1
	}
	q := url.Values{"search_query": {keyword}, "page": {strconv.Itoa(page)}}
	if s := jmSortParam(sortKey); s != "" {
		q.Set("o", s)
	}
	var res struct {
		Total   flexInt      `json:"total"`
		Content []jmListItem `json:"content"`
	}
	if err := j.getJSON(ctx, cred, "/search", q, &res); err != nil {
		return nil, err
	}
	items := make([]*Comic, 0, len(res.Content))
	for _, it := range res.Content {
		items = append(items, it.toComic())
	}
	return &SearchResult{Total: res.Total.Int(), Page: page, PageSize: pageSize, Items: items}, nil
}

func jmSortParam(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "mr", "latest", "新", "":
		return "mr" // 最新
	case "mv", "views", "浏览":
		return "mv"
	case "mp", "likes", "点赞":
		return "mp"
	case "tf", "today", "今日":
		return "tf"
	}
	return "mr"
}

// ------------------------------ 详情 ------------------------------

type jmAlbum struct {
	ID          json.Number    `json:"id"`
	Name        string         `json:"name"`
	Author      []string       `json:"author"`
	Description string         `json:"description"`
	Image       string         `json:"image"`
	Images      []string       `json:"images"`
	SeriesID    json.Number    `json:"series_id"`
	Tags        []jmTag        `json:"tags"`
	Series      []jmSeriesItem `json:"series"`
	IsFavorite  bool           `json:"is_favorite"`
	// works 字段站点有时返回对象数组、有时返回字符串数组（2026-09-22 修：本子 461647
	// 返回 ["..."] 导致 "cannot unmarshal string into jmAlbum.works.0" 整本详情失败），
	// 且本字段目前未使用 → 原样收下即可。
	Works json.RawMessage `json:"works"`
}

type jmSeriesItem struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
	Sort int         `json:"sort"`
}

// Detail 本子详情：/album 拿元信息 + 章列表，再补每章图片数
func (j *JM) Detail(ctx context.Context, cred *Cred, comicID string) (*Comic, error) {
	var alb jmAlbum
	if err := j.getJSON(ctx, cred, "/album", url.Values{"id": {comicID}}, &alb); err != nil {
		return nil, err
	}
	if alb.ID.String() == "" || alb.ID.String() == "0" {
		return nil, fmt.Errorf("本子不存在: %s", comicID)
	}

	c := &Comic{
		Kind:        "jm",
		ComicID:     alb.ID.String(),
		Title:       strings.TrimSpace(alb.Name),
		Author:      strings.Join(alb.Author, " "),
		Description: strings.TrimSpace(alb.Description),
		Cover:       "/api/comics/jm/" + alb.ID.String() + "/cover",
	}
	for _, t := range alb.Tags {
		if t.Title != "" {
			c.Tags = append(c.Tags, t.Title)
		}
	}
	c.Tags = dedupStrings(c.Tags)
	if len(alb.Tags) > 0 && c.Categories == nil {
		c.Categories = []string{"禁漫"}
	}

	if len(alb.Series) > 0 {
		sort.Slice(alb.Series, func(a, b int) bool { return alb.Series[a].Sort < alb.Series[b].Sort })
		for i, s := range alb.Series {
			title := strings.TrimSpace(s.Name)
			if title == "" {
				title = fmt.Sprintf("第 %d 話", i+1)
			}
			c.Chapters = append(c.Chapters, Chapter{Order: i + 1, ID: s.ID.String(), Title: title})
		}
	} else {
		ch := Chapter{Order: 1, ID: alb.ID.String(), Title: c.Title, Images: len(alb.Images)}
		if ch.Images == 0 {
			// 单章本子的 album 接口可能不带图片列表，补一次章节接口
			if n, err := j.chapterImageCount(ctx, cred, ch.ID); err == nil {
				ch.Images = n
			}
		}
		c.Chapters = []Chapter{ch}
	}
	c.ChaptersCount = len(c.Chapters)
	j.fillChapterImageCounts(ctx, cred, c)
	return c, nil
}

// fillChapterImageCounts 并行补齐每章图片数（拿不到就留 0，不阻塞详情）
func (j *JM) fillChapterImageCounts(ctx context.Context, cred *Cred, c *Comic) {
	const maxChapters = 60
	if len(c.Chapters) <= 1 {
		return
	}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range c.Chapters {
		if i >= maxChapters || c.Chapters[i].Images > 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(ch *Chapter) {
			defer wg.Done()
			defer func() { <-sem }()
			if n, err := j.chapterImageCount(ctx, cred, ch.ID); err == nil {
				ch.Images = n
			}
		}(&c.Chapters[i])
	}
	wg.Wait()
}

// ------------------------------ 章节图片 ------------------------------

type jmChapter struct {
	ID     json.Number `json:"id"`
	Name   string      `json:"name"`
	Images []string    `json:"images"`
}

func (j *JM) chapterImages(ctx context.Context, cred *Cred, photoID string) ([]string, error) {
	var ch jmChapter
	if err := j.getJSON(ctx, cred, "/chapter", url.Values{"id": {photoID}}, &ch); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ch.Images))
	for _, n := range ch.Images {
		if s := strings.TrimSpace(n); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("章节 %s 没有图片", photoID)
	}
	return out, nil
}

func (j *JM) chapterImageCount(ctx context.Context, cred *Cred, photoID string) (int, error) {
	var ch jmChapter
	if err := j.getJSON(ctx, cred, "/chapter", url.Values{"id": {photoID}}, &ch); err != nil {
		return 0, err
	}
	return len(ch.Images), nil
}

var jmScramblePattern = regexp.MustCompile(`var\s+scramble_id\s*=\s*(\d+);`)

// scrambleID 取本子的乱序阈值（同本子所有章节相同）；拿不到就用默认值
func (j *JM) scrambleID(ctx context.Context, cred *Cred, photoID string) int {
	j.mu.Lock()
	if v, ok := j.scrambleCache[photoID]; ok {
		j.mu.Unlock()
		return v
	}
	j.mu.Unlock()

	id := jmScramble220980
	params := url.Values{
		"id": {photoID}, "mode": {"vertical"}, "page": {"0"},
		"app_img_shunt": {"1"}, "express": {"off"}, "v": {strconv.FormatInt(time.Now().Unix(), 10)},
	}
	if res, err := j.do(ctx, cred, http.MethodGet, "/chapter_view_template", params, nil, jmScrambleSecret, j.api); err == nil {
		if m := jmScramblePattern.FindSubmatch(res.data); len(m) == 2 {
			if v, cerr := strconv.Atoi(string(m[1])); cerr == nil {
				id = v
			}
		}
	}
	j.mu.Lock()
	j.scrambleCache[photoID] = id
	j.mu.Unlock()
	return id
}

// jmImageURL 拼图片地址；站点要求带上 v 参数（否则可能返回空数据）
func (j *JM) jmImageURL(photoID, filename string) string {
	j.mu.Lock()
	domain := j.imageDomain
	j.mu.Unlock()
	return fmt.Sprintf("https://%s/media/photos/%s/%s?v=%d", domain, photoID, filename, time.Now().Unix())
}

// ------------------------------ 收藏增删 ------------------------------

func (j *JM) AddFavorite(ctx context.Context, cred *Cred, comicID string) error {
	return j.toggleFavorite(ctx, cred, comicID, "add")
}

func (j *JM) DelFavorite(ctx context.Context, cred *Cred, comicID string) error {
	return j.toggleFavorite(ctx, cred, comicID, "remove")
}

// toggleFavorite 移动端 /favorite 是 toggle 语义：按其返回的 type 判断是否需要再来一次
func (j *JM) toggleFavorite(ctx context.Context, cred *Cred, comicID, want string) error {
	for attempt := 0; attempt < 2; attempt++ {
		res, err := j.do(ctx, cred, http.MethodPost, "/favorite", nil, url.Values{"aid": {comicID}}, jmTokenSecret, j.api)
		if err != nil {
			return err
		}
		var d struct {
			Status string `json:"status"`
			Type   string `json:"type"`
			Msg    string `json:"msg"`
		}
		if err := json.Unmarshal(res.data, &d); err != nil {
			return err
		}
		if d.Type == want {
			return nil
		}
		if d.Status != "" && d.Status != "ok" {
			return fmt.Errorf("收藏操作失败: %s", orDefault(d.Msg, d.Status))
		}
	}
	return fmt.Errorf("收藏状态未能切换为 %s", want)
}

// ------------------------------ 封面 ------------------------------

// Cover 取封面图：优先 album.image，否则 /media/albums/<id>.jpg
func (j *JM) Cover(ctx context.Context, cred *Cred, comicID string) ([]byte, string, error) {
	if b, ct, err := j.fetchCoverBytes(ctx, comicID, ""); err == nil {
		return b, ct, nil
	}
	// 兜底：问一次 album 拿 image 字段
	var alb jmAlbum
	if err := j.getJSON(ctx, cred, "/album", url.Values{"id": {comicID}}, &alb); err == nil && alb.Image != "" {
		if b, ct, err := j.fetchCoverBytes(ctx, comicID, alb.Image); err == nil {
			return b, ct, nil
		}
	}
	return nil, "", errors.New("封面获取失败")
}

func (j *JM) fetchCoverBytes(ctx context.Context, comicID, image string) ([]byte, string, error) {
	j.mu.Lock()
	domain := j.imageDomain
	j.mu.Unlock()
	name := image
	if name == "" {
		name = comicID + ".jpg"
	}
	u := fmt.Sprintf("https://%s/media/albums/%s", domain, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("user-agent", jmMobileUA)
	req.Header.Set("Referer", "https://"+j.apiDomain)
	req.Header.Set("X-Requested-With", "com.JMComic3.app")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	resp, err := j.img.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("封面 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	if !isImage(b) {
		return nil, "", errors.New("封面不是图片")
	}
	return b, orDefault(resp.Header.Get("Content-Type"), "image/jpeg"), nil
}

// ------------------------------ 在线阅读 ------------------------------

type jmChapEntry struct {
	at       time.Time
	chapters []Chapter
}

type jmImgEntry struct {
	at    time.Time
	names []string
}

// resolveChapter 按章节序号找章节（含 photoID），10 分钟缓存避免反复拉详情
func (j *JM) resolveChapter(ctx context.Context, cred *Cred, comicID string, order int) (Chapter, error) {
	j.mu.Lock()
	if e, ok := j.chapCache[comicID]; ok && time.Since(e.at) < 10*time.Minute {
		for _, ch := range e.chapters {
			if ch.Order == order {
				j.mu.Unlock()
				return ch, nil
			}
		}
	}
	j.mu.Unlock()

	c, err := j.Detail(ctx, cred, comicID)
	if err != nil {
		return Chapter{}, err
	}
	j.mu.Lock()
	j.chapCache[comicID] = jmChapEntry{at: time.Now(), chapters: c.Chapters}
	j.mu.Unlock()
	for _, ch := range c.Chapters {
		if ch.Order == order {
			return ch, nil
		}
	}
	return Chapter{}, fmt.Errorf("章节 %d 不存在", order)
}

func (j *JM) chapterImagesCached(ctx context.Context, cred *Cred, photoID string) ([]string, error) {
	j.mu.Lock()
	if e, ok := j.imgCache[photoID]; ok && time.Since(e.at) < 10*time.Minute {
		j.mu.Unlock()
		return e.names, nil
	}
	j.mu.Unlock()
	names, err := j.chapterImages(ctx, cred, photoID)
	if err != nil {
		return nil, err
	}
	j.mu.Lock()
	j.imgCache[photoID] = jmImgEntry{at: time.Now(), names: names}
	j.mu.Unlock()
	return names, nil
}

// Pages 在线阅读：某章页数与章节名
func (j *JM) Pages(ctx context.Context, cred *Cred, comicID string, order int) (int, string, error) {
	ch, err := j.resolveChapter(ctx, cred, comicID, order)
	if err != nil {
		return 0, "", err
	}
	names, err := j.chapterImagesCached(ctx, cred, ch.ID)
	if err != nil {
		return 0, "", err
	}
	return len(names), ch.Title, nil
}

// PageImage 在线阅读：某章第 page 页图片（乱序在这里还原，直接可看）
func (j *JM) PageImage(ctx context.Context, cred *Cred, comicID string, order, page int, preview bool) ([]byte, string, error) {
	ch, err := j.resolveChapter(ctx, cred, comicID, order)
	if err != nil {
		return nil, "", err
	}
	names, err := j.chapterImagesCached(ctx, cred, ch.ID)
	if err != nil {
		return nil, "", err
	}
	if page < 1 || page > len(names) {
		return nil, "", fmt.Errorf("页码 %d 超出范围（本章共 %d 页）", page, len(names))
	}
	name := names[page-1]
	raw, err := j.fetchImage(ctx, ch.ID, name)
	if err != nil {
		return nil, "", err
	}
	num := jmSegNum(j.scrambleID(ctx, cred, ch.ID), jmAid(ch.ID), name)
	if num <= 0 {
		return raw, jmImageContentType(name), nil
	}
	mode := "lossless"
	if preview {
		mode = "jpeg" // 在线阅读：小图快传
	}
	data, ext, err := jmEncodePage(raw, num, mode)
	if err != nil {
		return nil, "", err
	}
	return data, jmImageContentType("x." + ext), nil
}

func jmImageContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	}
	return "image/webp"
}

// ------------------------------ 下载 ------------------------------

// Download 下载整本或指定章节（orders 为空表示全部）。
// 已完整的章节直接跳过；图片并行抓取，乱序图重排后无损保存。
func (j *JM) Download(ctx context.Context, cred *Cred, comicID string, orders []int, dstRoot string, h Hooks) (*Result, error) {
	c, err := j.Detail(ctx, cred, comicID)
	if err != nil {
		return nil, err
	}
	selected := pickChapters(c.Chapters, orders)
	if len(selected) == 0 {
		return nil, fmt.Errorf("没有可下载的章节")
	}

	albumDir := filepath.Join(dstRoot, jmAlbumDirName(c))
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		return nil, err
	}
	multi := len(c.Chapters) > 1

	res := &Result{Path: albumDir, Title: c.Title}
	var (
		failedChapters []string
		mu             sync.Mutex
		imagesDone     int
		imagesTotal    int
		bytesTotal     int64
	)
	prog := func(cur string) {
		mu.Lock()
		p := Progress{
			ChaptersTotal: len(selected), ChaptersDone: res.ChaptersDone,
			ImagesTotal: imagesTotal, ImagesDone: imagesDone,
			Bytes: bytesTotal, CurrentChapter: cur,
		}
		mu.Unlock()
		h.prog(p)
	}

	for _, ch := range selected {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		// 章节图片列表
		imgs, ierr := j.chapterImages(ctx, cred, ch.ID)
		if ierr != nil {
			h.log("error", fmt.Sprintf("章节《%s》取图片列表失败: %v", ch.Title, ierr))
			failedChapters = append(failedChapters, ch.Title)
			continue
		}

		dir := albumDir
		if multi {
			dir = filepath.Join(albumDir, jmChapterDirName(ch))
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return res, err
		}

		mu.Lock()
		imagesTotal += len(imgs)
		mu.Unlock()

		if n, _ := dirImageStats(dir); n >= len(imgs) {
			h.log("info", fmt.Sprintf("章节《%s》已存在且完整（%d 张），跳过", ch.Title, n))
			res.ChaptersDone++
			res.Images += n
			prog(ch.Title)
			continue
		}

		h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "start", Images: len(imgs)})
		h.log("info", fmt.Sprintf("章节《%s》开始下载 %d 张", ch.Title, len(imgs)))
		prog(ch.Title)

		sid := j.scrambleID(ctx, cred, ch.ID)
		n, b, skipped, derr := j.downloadChapter(ctx, cred, ch.ID, imgs, dir, sid, func() {
			mu.Lock()
			imagesDone++
			p := Progress{
				ChaptersTotal: len(selected), ChaptersDone: res.ChaptersDone,
				ImagesTotal: imagesTotal, ImagesDone: imagesDone,
				Bytes: bytesTotal, CurrentChapter: ch.Title,
			}
			mu.Unlock()
			h.prog(p)
		}, func(delta int64) {
			mu.Lock()
			bytesTotal += delta
			mu.Unlock()
		})
		mu.Lock()
		imagesDone += skipped
		mu.Unlock()

		if derr != nil {
			h.log("error", fmt.Sprintf("章节《%s》下载出错: %v", ch.Title, derr))
			failedChapters = append(failedChapters, ch.Title)
			h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "failed", Images: len(imgs), Err: derr.Error()})
			continue
		}
		res.ChaptersDone++
		res.Images += n + skipped
		res.Bytes += b
		h.chap(ChapterState{Order: ch.Order, Title: ch.Title, State: "done", Images: n + skipped, Path: dir})
		h.log("info", fmt.Sprintf("章节《%s》完成: %d 张 / %s", ch.Title, n+skipped, humanBytes(b)))
		prog(ch.Title)
	}

	if len(failedChapters) > 0 {
		return res, fmt.Errorf("%d 章下载失败: %s", len(failedChapters), strings.Join(failedChapters, "、"))
	}
	return res, nil
}

// downloadChapter 并行下载一章的图片，返回(新下载张数, 字节, 已存在张数, 错误)
func (j *JM) downloadChapter(ctx context.Context, cred *Cred, photoID string, names []string, dir string,
	scrambleID int, onImage func(), onBytes func(int64)) (int, int64, int, error) {

	type job struct {
		idx  int
		name string
	}
	jobs := make(chan job)
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		done       int
		skipped    int
		bytesTotal int64
		firstErr   error
	)
	workers := j.imageWorkers
	if workers > len(names) {
		workers = len(names)
	}
	if workers < 1 {
		workers = 1
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range jobs {
				dst := filepath.Join(dir, fmt.Sprintf("%05d%s", it.idx+1, orDefault(filepath.Ext(it.name), ".webp")))
				if fi, err := os.Stat(dst); err == nil && fi.Size() > 0 {
					mu.Lock()
					skipped++
					mu.Unlock()
					onImage()
					continue
				}
				if alt := strings.TrimSuffix(dst, filepath.Ext(dst)) + ".png"; fileExists(alt) {
					mu.Lock()
					skipped++
					mu.Unlock()
					onImage()
					continue
				}

				raw, err := j.fetchImage(ctx, photoID, it.name)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				num := jmSegNum(scrambleID, jmAid(photoID), it.name)
				if _, err := jmSaveImage(raw, num, dst); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				done++
				bytesTotal += int64(len(raw))
				mu.Unlock()
				onBytes(int64(len(raw)))
				onImage()
			}
		}()
	}
	for i, n := range names {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return done, bytesTotal, skipped, ctx.Err()
		case jobs <- job{idx: i, name: n}:
		}
	}
	close(jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	return done, bytesTotal, skipped, firstErr
}

// fetchImage 下载图片原始字节（带 1 次重试与域名切换）
func (j *JM) fetchImage(ctx context.Context, photoID, filename string) ([]byte, error) {
	var lastErr error
	domains := func() []string {
		j.mu.Lock()
		cur := j.imageDomain
		j.mu.Unlock()
		out := []string{cur}
		for _, d := range jmImageDomains {
			if d != cur {
				out = append(out, d)
			}
		}
		return out
	}
	for _, domain := range domains() {
		for attempt := 0; attempt < 2; attempt++ {
			u := fmt.Sprintf("https://%s/media/photos/%s/%s?v=%d", domain, photoID, filename, time.Now().Unix())
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("user-agent", jmMobileUA)
			req.Header.Set("Referer", "https://"+jmAPIDomains[0])
			req.Header.Set("X-Requested-With", "com.JMComic3.app")
			req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
			resp, err := j.img.Do(req)
			if err != nil {
				lastErr = err
				continue
			}
			raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
			_ = resp.Body.Close()
			if rerr != nil {
				lastErr = rerr
				continue
			}
			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
				continue
			}
			if !isImage(raw) {
				// 空数据/防盗链页
				lastErr = errors.New("返回内容不是图片")
				continue
			}
			j.mu.Lock()
			j.imageDomain = domain
			j.mu.Unlock()
			return raw, nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("图片下载失败")
	}
	return nil, lastErr
}

// ------------------------------ 小工具 ------------------------------

func pickChapters(all []Chapter, orders []int) []Chapter {
	if len(orders) == 0 {
		out := make([]Chapter, len(all))
		copy(out, all)
		sortChapters(out)
		return out
	}
	want := map[int]bool{}
	for _, o := range orders {
		want[o] = true
	}
	var out []Chapter
	for _, c := range all {
		if want[c.Order] {
			out = append(out, c)
		}
	}
	sortChapters(out)
	return out
}

// jmAlbumDirName 与既有目录约定一致：<本子id> <标题>
func jmAlbumDirName(c *Comic) string {
	return sanitize(c.ComicID+" "+c.Title, 120)
}

func jmChapterDirName(ch Chapter) string {
	return fmt.Sprintf("%03d - %s", ch.Order, sanitize(ch.Title, 70))
}

func jmAid(photoID string) int {
	n, _ := strconv.Atoi(photoID)
	return n
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Size() > 0
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
