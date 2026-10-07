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
	if files := cachedImageFiles(dir); len(files) != 3 || files[1] != "" || files[2] == "" {
		t.Fatalf("sparse page numbers lost: %v", files)
	}
	os.WriteFile(filepath.Join(dir, "002.jpg"), []byte("image"), 0644)
	if n := len(cachedImageFiles(dir)); n != 3 {
		t.Fatalf("index not invalidated: %d", n)
	}
}
