package engine

import (
	"context"
	"encoding/json"
	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
	"os"
	"path/filepath"
	"testing"
)

type fakeSource struct {
	source.Source
	detail *source.Comic
	pages  map[int]int
}

func (f *fakeSource) Kind() string { return "jm" }
func (f *fakeSource) Detail(context.Context, *source.Cred, string) (*source.Comic, error) {
	return f.detail, nil
}
func (f *fakeSource) Pages(_ context.Context, _ *source.Cred, _ string, n int) (int, string, error) {
	return f.pages[n], "", nil
}
func setupEngine(t *testing.T) *Engine {
	t.Helper()
	cfg := testCfg(t, "")
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return New(s, cfg)
}
func pages(t *testing.T, path string, n int) {
	t.Helper()
	os.MkdirAll(path, 0755)
	for i := 1; i <= n; i++ {
		os.WriteFile(filepath.Join(path, fmtPage(i)), []byte("image"), 0644)
	}
}
func fmtPage(n int) string {
	if n == 1 {
		return "00001.jpg"
	}
	return "00002.jpg"
}
func TestSyncDetectsNewAndMissingChapters(t *testing.T) {
	e := setupEngine(t)
	dir := filepath.Join(e.SourceDir("jm"), "1 Test")
	pages(t, filepath.Join(dir, "001 First"), 2)
	e.st.UpsertComic(&store.Comic{Kind: "jm", ComicID: "1", Path: dir, Chapters: 1, ChaptersDone: 1, Images: 2, Complete: true})
	f := &fakeSource{detail: &source.Comic{Chapters: []source.Chapter{{Order: 1, ID: "a"}, {Order: 2, ID: "b"}}}, pages: map[int]int{1: 2, 2: 2}}
	missing, err := e.verifyComic(context.Background(), f, &source.Cred{}, "1")
	if err != nil || len(missing) != 1 || missing[0] != 2 {
		t.Fatalf("%v %v", missing, err)
	}
	c, _ := e.st.GetComic("jm", "1")
	if c.Complete || c.Chapters != 2 {
		t.Fatalf("%+v", c)
	}
	if _, _, err = e.ScanLibrary(); err != nil {
		t.Fatal(err)
	}
	c, _ = e.st.GetComic("jm", "1")
	if c.Complete || c.Chapters != 2 {
		t.Fatal("scan overwrote remote total")
	}
	os.Remove(filepath.Join(dir, "001 First", "00001.jpg"))
	missing, err = e.verifyComic(context.Background(), f, &source.Cred{}, "1")
	if err != nil || len(missing) != 2 {
		t.Fatalf("missing page ignored: %v %v", missing, err)
	}
}
func TestTrashPreservesAndRestoresFiles(t *testing.T) {
	e := setupEngine(t)
	dir := filepath.Join(e.SourceDir("jm"), "1 Test")
	pages(t, dir, 1)
	e.st.UpsertComic(&store.Comic{Kind: "jm", ComicID: "1", Path: dir})
	c, _ := e.st.GetComic("jm", "1")
	job, _ := e.Enqueue(&store.Job{Kind: "jm", ComicID: "1"})
	if err := e.DeleteComic(c.ID, true); err == nil {
		t.Fatal("deleted active download")
	}
	e.Cancel(job)
	if err := e.DeleteComic(c.ID, true); err != nil {
		t.Fatal(err)
	}
	items, _ := e.st.TrashItems()
	if len(items) != 1 {
		t.Fatal(items)
	}
	if _, err := os.Stat(filepath.Join(items[0].Path, "00001.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Enqueue(&store.Job{Kind: "jm", ComicID: "1"}); err == nil {
		t.Fatal("trash re-enqueued")
	}
	if err := e.RestoreTrash(items[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "00001.jpg")); err != nil {
		t.Fatal(err)
	}
}
func TestLowDiskWaitCanCancel(t *testing.T) {
	e := setupEngine(t)
	e.cfg.Update(map[string]json.RawMessage{"minFreeMB": json.RawMessage(`1048576`)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.waitDownload(ctx); err == nil {
		t.Fatal("canceled wait succeeded")
	}
}

func TestSyncAllPropagatesFailure(t *testing.T) {
	e := setupEngine(t)
	if _, err := e.st.CreateAccount(&store.Account{Kind: "jm", Username: "expired"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := e.SyncAll(context.Background())
	if err == nil {
		t.Fatal("failed account reported as success")
	}
	runs, _ := e.st.SyncHistory()
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("%+v", runs)
	}
}
