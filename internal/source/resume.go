package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Bound memory-intensive decode/encode work across concurrent books and both sources.
var imageProcessingSlots = make(chan struct{}, 2)

func validImageBytes(b []byte) bool {
	imageProcessingSlots <- struct{}{}
	defer func() { <-imageProcessingSlots }()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
		return false
	}
	_, _, err = image.Decode(bytes.NewReader(b))
	return err == nil
}
func validImageFile(path string) (int64, bool) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return 0, false
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(b))
	old, err := os.ReadFile(path + ".sha256")
	if err == nil {
		return int64(len(b)), string(old) == sum
	}
	if !validImageBytes(b) {
		return 0, false
	}
	if err = writeFileAtomic(path+".sha256", []byte(sum)); err != nil {
		return 0, false
	}
	return int64(len(b)), true
}
func saveImageReceipt(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return writeImageReceipt(path, b)
}
func preparePageManifest(dir string, ids []string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ".pages.json")
	old, _ := os.ReadFile(path)
	if len(old) > 0 && !bytes.Equal(old, raw) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := strings.TrimSuffix(ent.Name(), ".sha256")
			ext := filepath.Ext(name)
			if !imgExts[strings.ToLower(ext)] {
				continue
			}
			if _, err := strconv.Atoi(strings.TrimSuffix(name, ext)); err == nil {
				if err = os.Remove(filepath.Join(dir, ent.Name())); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
	}
	return writeFileAtomic(path, raw)
}
func completeImageFiles(dir string, count int) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	seen := map[int]bool{}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		ext := filepath.Ext(ent.Name())
		if !imgExts[strings.ToLower(ext)] {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(ent.Name(), ext))
		if err != nil || n < 1 {
			continue
		}
		if _, ok := validImageFile(filepath.Join(dir, ent.Name())); !ok {
			return false
		}
		if seen[n] {
			return false
		}
		seen[n] = true
	}
	if len(seen) != count {
		return false
	}
	for n := 1; n <= count; n++ {
		if !seen[n] {
			return false
		}
	}
	return count > 0
}
func commitChapter(tmp, dst string) error {
	old := filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".previous")
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if _, e := os.Stat(old); e == nil {
			if e = os.Rename(old, dst); e != nil {
				return e
			}
		}
	}
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	moved := false
	if _, err := os.Stat(dst); err == nil {
		if err = os.Rename(dst, old); err != nil {
			return err
		}
		moved = true
	}
	if err := os.Rename(tmp, dst); err != nil {
		if moved {
			_ = os.Rename(old, dst)
		}
		return err
	}
	return os.RemoveAll(old)
}

func pageManifestMatches(dir string, ids []string) bool {
	old, err := os.ReadFile(filepath.Join(dir, ".pages.json"))
	if os.IsNotExist(err) {
		return true
	}
	raw, _ := json.Marshal(ids)
	return err == nil && bytes.Equal(old, raw)
}

func writeImageReceipt(path string, b []byte) error {
	return writeFileAtomic(path+".sha256", []byte(fmt.Sprintf("%x", sha256.Sum256(b))))
}

// Seed a repair staging directory only from the same page manifest.
func seedChapterRepair(src, dst string, ids []string, urls []string) error {
	if !pageManifestMatches(src, ids) {
		return nil
	}
	for i, u := range urls {
		name := fmt.Sprintf("%03d%s", i+1, imgExtFromURL(u))
		target := filepath.Join(dst, name)
		if _, ok := validImageFile(target); ok {
			continue
		}
		original := filepath.Join(src, name)
		if _, ok := validImageFile(original); !ok {
			continue
		}
		b, err := os.ReadFile(original)
		if err != nil {
			return err
		}
		if err = writeFileAtomic(target, b); err != nil {
			return err
		}
		if err = writeImageReceipt(target, b); err != nil {
			return err
		}
	}
	return nil
}
