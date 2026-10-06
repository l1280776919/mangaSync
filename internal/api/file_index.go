package api

import (
	"os"
	"sync"
	"time"
)

type fileIndexEntry struct {
	files []string
	mod   time.Time
	at    time.Time
}

var fileIndex = struct {
	sync.Mutex
	entries map[string]fileIndexEntry
}{entries: map[string]fileIndexEntry{}}

// Directory mtime invalidates renames/additions; short TTL covers external filesystem changes.
func cachedImageFiles(dir string) []string {
	fi, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	fileIndex.Lock()
	defer fileIndex.Unlock()
	if old, ok := fileIndex.entries[dir]; ok && old.mod.Equal(fi.ModTime()) && time.Since(old.at) < 5*time.Second {
		return old.files
	}
	files := imageFilesIn(dir)
	if len(fileIndex.entries) >= 128 {
		fileIndex.entries = map[string]fileIndexEntry{}
	}
	fileIndex.entries[dir] = fileIndexEntry{files: files, mod: fi.ModTime(), at: time.Now()}
	return files
}
