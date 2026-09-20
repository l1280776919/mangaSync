package source

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrAuth = errors.New("登录态失效")

type Cred struct {
	AccountID int64
	Kind      string
	Username  string
	Token     string // pica: authorization token; jm: cookies(JSON)
}

type Chapter struct {
	Order      int    `json:"order"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Images     int    `json:"images"`
	Downloaded bool   `json:"downloaded"`
}

type Comic struct {
	Kind          string    `json:"kind"`
	ComicID       string    `json:"comicId"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Categories    []string  `json:"categories"`
	Tags          []string  `json:"tags"`
	Description   string    `json:"description,omitempty"`
	ChaptersCount int       `json:"chaptersCount"`
	LatestEp      string    `json:"latestEp"`
	Cover         string    `json:"cover"`
	Chapters      []Chapter `json:"chapters,omitempty"`

	// 本地状态（API 层回填）
	Downloaded         bool   `json:"downloaded"`
	DownloadedChapters int    `json:"downloadedChapters"`
	Images             int    `json:"images"`
	Bytes              int64  `json:"bytes"`
	LocalPath          string `json:"localPath"`
}

// MarshalJSON 让 "chapters" 在列表场景是章节数、在详情场景是章节数组（前端按契约统一用 chapters）
func (c Comic) MarshalJSON() ([]byte, error) {
	type alias Comic
	base, err := json.Marshal(alias(c))
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	if c.Chapters != nil {
		m["chapters"] = c.Chapters
	} else {
		m["chapters"] = c.ChaptersCount
	}
	return json.Marshal(m)
}

type SearchResult struct {
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
	Items    []*Comic `json:"items"`
}

type AccountInfo struct {
	Nickname       string `json:"nickname"`
	Level          int    `json:"level"`
	FavoritesCount int    `json:"favoritesCount"`
	FavoritesMax   int    `json:"favoritesMax"`
	UID            string `json:"uid,omitempty"`
	Token          string `json:"-"`
}

type Progress struct {
	ChaptersTotal  int
	ChaptersDone   int
	ImagesTotal    int
	ImagesDone     int
	Bytes          int64
	SpeedBps       int64
	CurrentChapter string
}

type ChapterState struct {
	Order  int
	Title  string
	State  string // start | done | failed
	Images int
	Path   string
	Err    string
}

type Hooks struct {
	// JobID 当前任务号（可为 0）：源用它给临时目录/文件加后缀，避免同一本漫画
	// 被并发下载时不同任务的中间产物互相覆盖。
	JobID    int64
	Progress func(Progress)
	Chapter  func(ChapterState)
	Log      func(level, msg string)
}

func (h Hooks) log(level, msg string) {
	if h.Log != nil {
		h.Log(level, msg)
	}
}

func (h Hooks) prog(p Progress) {
	if h.Progress != nil {
		h.Progress(p)
	}
}

func (h Hooks) chap(s ChapterState) {
	if h.Chapter != nil {
		h.Chapter(s)
	}
}

type Result struct {
	Path         string
	Images       int
	Bytes        int64
	Title        string
	ChaptersDone int
}

// Source 是一个漫画源（哔咔 / 禁漫）
type Source interface {
	Kind() string
	Login(ctx context.Context, username, password string) (*AccountInfo, error)
	Profile(ctx context.Context, cred *Cred) (*AccountInfo, error)

	// Pages 在线阅读：返回某章的页数与章节名
	Pages(ctx context.Context, cred *Cred, comicID string, order int) (int, string, error)
	// PageImage 在线阅读：返回某章第 page 页（从 1 开始）的图片字节与内容类型
	// preview 为 true 时可用更快更小的编码（手机流量友好）；禁漫的乱序图必须在这里还原
	PageImage(ctx context.Context, cred *Cred, comicID string, order, page int, preview bool) ([]byte, string, error)
	Favorites(ctx context.Context, cred *Cred) ([]*Comic, error)
	Search(ctx context.Context, cred *Cred, keyword string, page, pageSize int, sort string) (*SearchResult, error)
	Detail(ctx context.Context, cred *Cred, comicID string) (*Comic, error)
	AddFavorite(ctx context.Context, cred *Cred, comicID string) error
	DelFavorite(ctx context.Context, cred *Cred, comicID string) error
	Cover(ctx context.Context, cred *Cred, comicID string) ([]byte, string, error)
	Download(ctx context.Context, cred *Cred, comicID string, orders []int, dstRoot string, h Hooks) (*Result, error)
}
