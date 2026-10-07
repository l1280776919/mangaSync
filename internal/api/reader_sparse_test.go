package api

import (
	"encoding/json"
	"github.com/l1280776919/mangaSync/internal/config"
	"github.com/l1280776919/mangaSync/internal/engine"
	"github.com/l1280776919/mangaSync/internal/store"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReaderPreservesMissingFirstMiddleAndTail(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(st, cfg, engine.New(st, cfg))
	book := filepath.Join(dir, "book")
	os.MkdirAll(book, 0755)
	os.WriteFile(filepath.Join(book, "00003.jpg"), []byte("page-three"), 0644)
	st.UpsertComic(&store.Comic{Kind: "jm", ComicID: "1", Path: book, Chapters: 1})
	st.SaveChapter("jm", "1", store.ChapterCheck{Order: 1, Images: 4})
	r := httptest.NewRequest("GET", "/meta", nil)
	r.SetPathValue("kind", "jm")
	r.SetPathValue("comicId", "1")
	r.SetPathValue("order", "1")
	w := httptest.NewRecorder()
	s.readerMeta(w, r)
	var meta struct {
		Pages        int
		MissingPages []int
		Sizes        [][2]int
	}
	if err = json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Pages != 4 || len(meta.MissingPages) != 3 || len(meta.Sizes) != 4 {
		t.Fatalf("%s", w.Body.String())
	}
	r.SetPathValue("page", "3")
	w = httptest.NewRecorder()
	s.readerPage(w, r)
	if w.Code != 200 || w.Body.String() != "page-three" {
		t.Fatalf("page shifted: %d %s", w.Code, w.Body.String())
	}
	os.Remove(filepath.Join(book, "00003.jpg"))
	w = httptest.NewRecorder()
	s.readerMeta(w, r)
	if err = json.Unmarshal(w.Body.Bytes(), &meta); err != nil || meta.Pages != 4 || len(meta.MissingPages) != 4 {
		t.Fatalf("empty local chapter lost known total: %s", w.Body.String())
	}
}
