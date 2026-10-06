package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type backupManifest struct {
	Version   int               `json:"version"`
	CreatedAt string            `json:"createdAt"`
	KeySHA256 string            `json:"keySHA256"`
	Files     map[string]string `json:"files"`
}

func fileSHA(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func copyPrivate(src, dst string) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e != nil {
		return e
	}
	return closeErr
}

// Backup uses SQLite's online consistent snapshot, including committed WAL data. The key stays separate.
func Backup(dataDir, destination string) error {
	if _, e := os.Stat(filepath.Join(dataDir, "mangasync.db")); e != nil {
		return e
	}
	if _, e := os.Lstat(destination); !os.IsNotExist(e) {
		return fmt.Errorf("备份目标必须不存在")
	}
	parent := filepath.Dir(destination)
	if e := os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(parent, ".mangasync-backup-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	db, e := sql.Open("sqlite", filepath.Join(dataDir, "mangasync.db")+"?mode=rw&_pragma=busy_timeout(5000)")
	if e != nil {
		return e
	}
	defer db.Close()
	target, _ := filepath.Abs(filepath.Join(stage, "mangasync.db"))
	if _, e = db.Exec("VACUUM INTO '" + strings.ReplaceAll(target, "'", "''") + "'"); e != nil {
		return e
	}
	if e = os.Chmod(target, 0600); e != nil {
		return e
	}
	m := backupManifest{Version: 1, CreatedAt: now(), Files: map[string]string{}}
	if m.KeySHA256, e = fileSHA(filepath.Join(dataDir, "secrets.key")); e != nil && !os.IsNotExist(e) {
		return e
	}
	m.Files["mangasync.db"], e = fileSHA(target)
	if e != nil {
		return e
	}
	if _, err := os.Stat(filepath.Join(dataDir, "config.json")); err == nil {
		if e = copyPrivate(filepath.Join(dataDir, "config.json"), filepath.Join(stage, "config.json")); e != nil {
			return e
		}
		m.Files["config.json"], e = fileSHA(filepath.Join(stage, "config.json"))
		if e != nil {
			return e
		}
	}
	raw, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(stage, "manifest.json"), raw, 0600); e != nil {
		return e
	}
	if e = VerifyBackup(stage, filepath.Join(dataDir, "secrets.key")); e != nil {
		return e
	}
	return os.Rename(stage, destination)
}
func VerifyBackup(backupDir, keyPath string) error {
	raw, e := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if e != nil {
		return e
	}
	var m backupManifest
	if e = json.Unmarshal(raw, &m); e != nil {
		return e
	}
	if m.Version != 1 || m.Files["mangasync.db"] == "" {
		return fmt.Errorf("备份清单不支持或缺少数据库")
	}
	for name, want := range m.Files {
		if name != "mangasync.db" && name != "config.json" {
			return fmt.Errorf("备份包含未知文件")
		}
		path := filepath.Join(backupDir, name)
		fi, e := os.Lstat(path)
		if e != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("备份文件类型不合法")
		}
		got, e := fileSHA(path)
		if e != nil {
			return e
		}
		if got != want {
			return fmt.Errorf("备份校验失败: %s", name)
		}
	}
	var key []byte
	if m.KeySHA256 != "" {
		key, e = os.ReadFile(keyPath)
		if e != nil {
			return e
		}
		if fmt.Sprintf("%x", sha256.Sum256(key)) != m.KeySHA256 {
			return fmt.Errorf("密钥与备份不匹配")
		}
	}
	db, e := sql.Open("sqlite", filepath.Join(backupDir, "mangasync.db")+"?mode=ro")
	if e != nil {
		return e
	}
	defer db.Close()
	var check string
	if e = db.QueryRow(`pragma quick_check`).Scan(&check); e != nil {
		return e
	}
	if check != "ok" {
		return fmt.Errorf("数据库完整性检查失败")
	}
	s := &Store{db: db}
	if len(key) > 0 {
		s.vault, e = cipherForKey(key)
		if e != nil {
			return e
		}
	}
	rows, e := db.Query(`select password,token from accounts`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var pw, tk string
		if e = rows.Scan(&pw, &tk); e != nil {
			return e
		}
		if _, e = s.decrypt(pw); e != nil {
			return e
		}
		if _, e = s.decrypt(tk); e != nil {
			return e
		}
	}
	return rows.Err()
}

// Restore only creates a new directory; switching the service to it is an explicit deployment step.
func RestoreBackup(backupDir, keyPath, destination string) error {
	if e := VerifyBackup(backupDir, keyPath); e != nil {
		return e
	}
	if _, e := os.Lstat(destination); !os.IsNotExist(e) {
		return fmt.Errorf("恢复目标必须不存在，拒绝覆盖运行数据")
	}
	parent := filepath.Dir(destination)
	if e := os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(parent, ".mangasync-restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	for _, name := range []string{"mangasync.db", "config.json"} {
		src := filepath.Join(backupDir, name)
		if _, e = os.Stat(src); os.IsNotExist(e) && name == "config.json" {
			continue
		}
		if e = copyPrivate(src, filepath.Join(stage, name)); e != nil {
			return e
		}
	}
	raw, e := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if e != nil {
		return e
	}
	var manifest backupManifest
	if e = json.Unmarshal(raw, &manifest); e != nil {
		return e
	}
	if manifest.KeySHA256 != "" {
		if e = copyPrivate(keyPath, filepath.Join(stage, "secrets.key")); e != nil {
			return e
		}
	}
	db, e := sql.Open("sqlite", filepath.Join(stage, "mangasync.db"))
	if e != nil {
		return e
	}
	// A restored instance must not revive old browser sessions.
	_, e = db.Exec(`delete from sessions`)
	if e == nil {
		_, e = db.Exec(`pragma wal_checkpoint(TRUNCATE)`)
	}
	db.Close()
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(stage, "restored-at.txt"), []byte(time.Now().Format(time.RFC3339)), 0600); e != nil {
		return e
	}
	return os.Rename(stage, destination)
}
