package engine

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Fingerprint metadata, not image bytes; missing/replaced pages invalidate the short verification cache.
func localFingerprint(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isImageExt(d.Name()) {
			return nil
		}
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		fmt.Fprintf(h, "%s:%d:%d\n", rel, fi.Size(), fi.ModTime().UnixNano())
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil)), err
}
