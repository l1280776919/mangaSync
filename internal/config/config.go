package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// DefaultDir 是运行期数据目录的默认值，可用环境变量 MANGASYNC_HOME 或 -data 参数覆盖
const DefaultDir = "/var/lib/mangasync"

// DirFromEnv 解析数据目录：MANGASYNC_HOME > 默认值
func DirFromEnv() string {
	if v := os.Getenv("MANGASYNC_HOME"); v != "" {
		return expand(v)
	}
	return DefaultDir
}

// expand 把相对路径转成绝对路径（相对当前工作目录）
func expand(p string) string {
	if p == "" {
		return p
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

type Schedule struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"` // "04:30"
}

type Settings struct {
	DownloadRoot string `json:"downloadRoot"`
	PicaDir      string `json:"picaDir"`
	JmDir        string `json:"jmDir"`

	PicaProxy string `json:"picaProxy"`
	JmProxy   string `json:"jmProxy"`

	JmVenv   string `json:"jmVenv"`
	JmBridge string `json:"jmBridge"`

	Concurrency  int    `json:"concurrency"`
	ImageWorkers int    `json:"imageWorkers"`
	Quality      string `json:"quality"`

	Schedule   Schedule `json:"schedule"`
	ServerPort int      `json:"serverPort"`

	// 访问认证：authUser 非空时，所有请求（/api/health 除外）都需要 HTTP Basic 认证
	// 适用于把服务通过 frp/反代暴露到公网；authPass 留空表示不修改现有密码
	AuthUser string `json:"authUser"`
	AuthPass string `json:"authPass"`
}

type Manager struct {
	mu   sync.RWMutex
	path string
	dir  string
	s    Settings
}

// Default 返回一份通用默认设置（不含任何个人环境路径）
func Default() Settings {
	return Settings{
		DownloadRoot: "/data/comics",
		PicaDir:      "PicaComic",
		JmDir:        "18Comic",
		PicaProxy:    "",
		JmProxy:      "",
		JmVenv:       "python3",
		JmBridge:     "engines/jm_bridge.py",
		Concurrency:  2,
		ImageWorkers: 8,
		Quality:      "original",
		Schedule:     Schedule{Enabled: true, Time: "04:30"},
		ServerPort:   8787,
	}
}

// Load 从 dir 读取（不存在则用默认值），dir 为空时取 DirFromEnv()
func Load(dir string) (*Manager, error) {
	if dir == "" {
		dir = DirFromEnv()
	}
	dir = expand(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{path: filepath.Join(dir, "config.json"), dir: dir, s: Default()}
	b, err := os.ReadFile(m.path)
	if err == nil {
		var s Settings
		if json.Unmarshal(b, &s) == nil {
			merged := Default()
			mergeSettings(&merged, s)
			m.s = merged
		}
	} else {
		// 首次运行：把默认设置落盘，方便直接改文件
		_ = m.save()
	}
	return m, nil
}

func (m *Manager) Dir() string { return m.dir }

func mergeSettings(dst *Settings, src Settings) {
	if src.DownloadRoot != "" {
		dst.DownloadRoot = src.DownloadRoot
	}
	if src.PicaDir != "" {
		dst.PicaDir = src.PicaDir
	}
	if src.JmDir != "" {
		dst.JmDir = src.JmDir
	}
	dst.PicaProxy = src.PicaProxy
	dst.JmProxy = src.JmProxy
	if src.JmVenv != "" {
		dst.JmVenv = src.JmVenv
	}
	if src.JmBridge != "" {
		dst.JmBridge = src.JmBridge
	}
	if src.Concurrency > 0 {
		dst.Concurrency = src.Concurrency
	}
	if src.ImageWorkers > 0 {
		dst.ImageWorkers = src.ImageWorkers
	}
	if src.Quality != "" {
		dst.Quality = src.Quality
	}
	dst.Schedule = src.Schedule
	if src.ServerPort > 0 {
		dst.ServerPort = src.ServerPort
	}
	dst.AuthUser = src.AuthUser
	if src.AuthPass != "" {
		dst.AuthPass = src.AuthPass
	}
}

func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

// Update 用 patch 里的字段覆盖，返回合并后的设置
func (m *Manager) Update(patch map[string]json.RawMessage) (Settings, error) {
	m.mu.Lock()
	s := m.s
	m.mu.Unlock()

	b, _ := json.Marshal(patch)
	var p Settings
	_ = json.Unmarshal(b, &p)
	// 只覆盖 patch 中出现的键
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	if _, ok := raw["picaProxy"]; !ok {
		p.PicaProxy = s.PicaProxy
	}
	if _, ok := raw["jmProxy"]; !ok {
		p.JmProxy = s.JmProxy
	}
	if _, ok := raw["schedule"]; ok {
		var sch Schedule
		if json.Unmarshal(patch["schedule"], &sch) == nil {
			p.Schedule = sch
		}
	} else {
		p.Schedule = s.Schedule
	}

	merged := s
	mergeSettings(&merged, p)
	if merged.Concurrency < 1 {
		merged.Concurrency = 1
	}
	if merged.Concurrency > 8 {
		merged.Concurrency = 8
	}
	if merged.ImageWorkers < 1 {
		merged.ImageWorkers = 1
	}
	if merged.ImageWorkers > 32 {
		merged.ImageWorkers = 32
	}
	if merged.Quality != "medium" && merged.Quality != "low" {
		merged.Quality = "original"
	}
	if merged.DownloadRoot == "" {
		merged.DownloadRoot = Default().DownloadRoot
	}
	if err := os.MkdirAll(merged.DownloadRoot, 0o755); err != nil {
		return s, err
	}

	m.mu.Lock()
	m.s = merged
	m.mu.Unlock()
	return merged, m.save()
}

func (m *Manager) save() error {
	m.mu.RLock()
	b, err := json.MarshalIndent(m.s, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// SourceDir 返回某个源的下载根目录
func (m *Manager) SourceDir(kind string) string {
	s := m.Get()
	if kind == "jm" {
		return filepath.Join(s.DownloadRoot, s.JmDir)
	}
	return filepath.Join(s.DownloadRoot, s.PicaDir)
}
