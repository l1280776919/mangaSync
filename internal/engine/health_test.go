package engine

import (
	"context"
	"github.com/l1280776919/mangaSync/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func healthBook(t *testing.T, e *Engine) *store.Comic {
	t.Helper()
	dir := filepath.Join(e.SourceDir("jm"), "1 Test")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := e.st.UpsertComic(&store.Comic{Kind: "jm", ComicID: "1", Title: "Test", Path: dir, Chapters: 1}); err != nil {
		t.Fatal(err)
	}
	c, err := e.st.GetComic("jm", "1")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestHealthDetectsGapsTailAndCorruption(t *testing.T) {
	e := setupEngine(t)
	c := healthBook(t, e)
	os.WriteFile(filepath.Join(c.Path, "00001.jpg"), []byte("broken"), 0644)
	os.WriteFile(filepath.Join(c.Path, "00003.jpg"), []byte("also broken"), 0644)
	e.st.SaveChapter("jm", "1", store.ChapterCheck{Order: 1, Images: 4})
	quick := e.checkComic(context.Background(), c, false)
	if quick.Status != "issues" || quick.IssueCount != 2 || len(quick.Chapters) != 1 {
		t.Fatalf("quick: %+v", quick)
	}
	deep := e.checkComic(context.Background(), c, true)
	if deep.IssueCount != 4 || len(deep.badFiles) != 2 {
		t.Fatalf("deep: %+v", deep)
	}
	if _, err := os.Stat(filepath.Join(c.Path, "00001.jpg.sha256")); !os.IsNotExist(err) {
		t.Fatal("inspection modified files")
	}
	e.st.CreateAccount(&store.Account{Kind: "jm", Username: "test"})
	e.health.run = LibraryHealth{ID: 1, Status: "done", Results: []ComicHealth{deep}}
	job, err := e.RepairHealth(1, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := e.st.GetJob(job)
	if err != nil || len(stored.Chapters) != 1 || stored.Chapters[0] != 1 {
		t.Fatalf("%+v %v", stored, err)
	}
	if _, err = os.Stat(filepath.Join(c.Path, "00001.jpg.repair")); err != nil {
		t.Fatal(err)
	}
	again, err := e.RepairHealth(1, c.ID)
	if err != nil || again != job {
		t.Fatal("repair not idempotent")
	}
}
func TestHealthRejectsStaleRepairAndSkipsDownloads(t *testing.T) {
	e := setupEngine(t)
	c := healthBook(t, e)
	e.st.SaveChapter("jm", "1", store.ChapterCheck{Order: 1, Images: 2})
	r := e.checkComic(context.Background(), c, false)
	e.health.run = LibraryHealth{ID: 1, Status: "done", Results: []ComicHealth{r}}
	os.WriteFile(filepath.Join(c.Path, "00001.jpg"), []byte("new"), 0644)
	if _, err := e.RepairHealth(1, c.ID); err == nil {
		t.Fatal("stale repair accepted")
	}
	e.Enqueue(&store.Job{Kind: "jm", ComicID: "1"})
	if r = e.checkComic(context.Background(), c, true); r.Status != "skipped" {
		t.Fatalf("%+v", r)
	}
}
func TestHealthBackgroundSnapshotAndCancellation(t *testing.T) {
	e := setupEngine(t)
	healthBook(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.lifeCtx = ctx
	initial, err := e.StartHealth(0, true)
	if err != nil {
		t.Fatal(err)
	}
	if initial.ID > 9007199254740991 {
		t.Fatal("run id cannot be represented by JS")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r := e.HealthStatus()
		if r.Status != "running" {
			if r.Status != "canceled" {
				t.Fatal(r)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("check did not stop")
}
