package engine

import (
	"context"
	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
	"path/filepath"
	"testing"
)

type fakeDownload struct {
	fakeSource
	path                    string
	detailCalls, pagesCalls int
}

func (f *fakeDownload) Detail(ctx context.Context, c *source.Cred, id string) (*source.Comic, error) {
	f.detailCalls++
	return f.detail, nil
}
func (f *fakeDownload) Pages(ctx context.Context, c *source.Cred, id string, n int) (int, string, error) {
	f.pagesCalls++
	return f.pages[n], "", nil
}
func (f *fakeDownload) Download(ctx context.Context, c *source.Cred, id string, orders []int, dst string, h source.Hooks) (*source.Result, error) {
	h.Progress(source.Progress{ChaptersTotal: 1, ChaptersDone: 1, ImagesTotal: 1, ImagesDone: 1})
	return &source.Result{Path: f.path, Title: "Test", Detail: f.detail, Verified: map[int]int{1: 1}}, nil
}
func TestDownloadChecksRequestedScopeAndReusesMetadata(t *testing.T) {
	for _, partial := range []bool{false, true} {
		e := setupEngine(t)
		dir := filepath.Join(e.SourceDir("jm"), "1 Test")
		pages(t, filepath.Join(dir, "001 First"), 1)
		f := &fakeDownload{fakeSource: fakeSource{detail: &source.Comic{Chapters: []source.Chapter{{Order: 1, ID: "a"}, {Order: 2, ID: "b"}}}, pages: map[int]int{1: 1, 2: 1}}, path: dir}
		e.srcs["jm"] = f
		e.srcKeys["jm"] = srcFingerprint("jm", e.cfg.Get())
		aid, err := e.st.CreateAccount(&store.Account{Kind: "jm", Username: "test"})
		if err != nil {
			t.Fatal(err)
		}
		e.st.SaveLoginOK(aid, "test-token", "", 0, 0, 0)
		j := &store.Job{AccountID: aid, Kind: "jm", ComicID: "1", Title: "Test"}
		if partial {
			j.Chapters = []int{1}
		}
		id, err := e.Enqueue(j)
		if err != nil {
			t.Fatal(err)
		}
		j.ID = id
		e.st.ClaimJob(id)
		e.runJob(context.Background(), j)
		got, _ := e.st.GetJob(id)
		if partial && got.Status != "done" {
			t.Fatalf("valid partial request failed: %+v", got)
		}
		if !partial && got.Status != "failed" {
			t.Fatalf("incomplete book accepted: %+v", got)
		}
		if f.detailCalls != 0 || f.pagesCalls != 0 {
			t.Fatalf("redundant metadata requests: %d %d", f.detailCalls, f.pagesCalls)
		}
	}
}
