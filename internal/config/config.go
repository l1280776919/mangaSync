package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Dir 是运行期数据目录（数据库/缓存/日志）
const Dir = "/var/lib/mangasync"

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
}

type Manager struct {
	mu   sync.RWMutex
	path string
	s    Settings
}

func Default() Settings {
	return Settings{
		DownloadRoot: "/data/comics",
		PicaDir:      "PicaComic",
		JmDir:        "18Comic",
		PicaProxy:    "http://127.0.0.1:7890",
		JmProxy:      "",
		JmVenv:       "python3",
		JmBridge:     "/opt/mangasync/engines/jm_bridge.py",
		Concurrency:  2,
		ImageWorkers: 8,
		Quality:      "original",
		Schedule:     Schedule{Enabled: true, Time: "04:30"},
		ServerPort:   8787,
	}
}

func Load() (*Manager, error) {
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{path: filepath.Join(Dir, "config.json"), s: Default()}
	b, err := os.ReadFile(m.path)
	if err == nil {
		var s Settings
		if json.Unmarshal(b, &s) == nil {
			merged := Default()
			mergeSettings(&merged, s)
			m.s = merged
		}
	}
	return m, nil
}

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
}

func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

// Update 用 patch 里的非零字段覆盖，返回合并后的设置
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
