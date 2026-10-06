package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPageIndexExcludesCoverAndInvalidatesOnChange(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cover.jpg", "001.jpg", "003.jpg"} {
		os.WriteFile(filepath.Join(dir, name), []byte("image"), 0644)
	}
	if n := len(cachedImageFiles(dir)); n != 1 {
		t.Fatalf("gap or cover counted as page: %d", n)
	}
	os.WriteFile(filepath.Join(dir, "002.jpg"), []byte("image"), 0644)
	if n := len(cachedImageFiles(dir)); n != 3 {
		t.Fatalf("index not invalidated: %d", n)
	}
}
