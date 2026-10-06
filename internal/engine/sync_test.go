package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/l1280776919/mangaSync/internal/source"
	"github.com/l1280776919/mangaSync/internal/store"
	"sync/atomic"
	"testing"
	"time"
)

type syncTestSource struct {
	source.Source
	favorites   func(context.Context) ([]*source.Comic, error)
	detailCalls atomic.Int32
}

func (f *syncTestSource) Kind() string { return "pica" }
func (f *syncTestSource) Favorites(ctx context.Context, _ *source.Cred) ([]*source.Comic, error) {
	return f.favorites(ctx)
}
func (f *syncTestSource) Detail(context.Context, *source.Cred, string) (*source.Comic, error) {
	f.detailCalls.Add(1)
	return nil, errors.New("unexpected detail request")
}
func installSyncSource(t *testing.T, e *Engine, f *syncTestSource) int64 {
	t.Helper()
	id, err := e.st.CreateAccount(&store.Account{Kind: "pica", Username: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.st.SaveLoginOK(id, "test-token", "", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	e.srcs["pica"] = f
	e.srcKeys["pica"] = srcFingerprint("pica", e.cfg.Get())
	return id
}
func awaitSync(t *testing.T, e *Engine) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	e.WaitSync(ctx)
	if ctx.Err() != nil {
		t.Fatal("sync failed to finish")
	}
}
func TestBackgroundSyncDeduplicatesAndSkipsUnneededRequests(t *testing.T) {
	e := setupEngine(t)
	gate := make(chan struct{})
	started := make(chan struct{})
	var calls atomic.Int32
	f := &syncTestSource{favorites: func(ctx context.Context) ([]*source.Comic, error) {
		calls.Add(1)
		close(started)
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		out := []*source.Comic{}
		for i := 0; i < 100; i++ {
			out = append(out, &source.Comic{ComicID: fmt.Sprint(i), Title: "test"})
		}
		return out, nil
	}}
	id := installSyncSource(t, e, f)
	first, err := e.StartAccountSync(id)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("not started")
	}
	for i := 0; i < 10; i++ {
		next, err := e.StartAccountSync(id)
		if err != nil || next.ID != first.ID {
			t.Fatalf("duplicate run: %+v %v", next, err)
		}
	}
	close(gate)
	awaitSync(t, e)
	status := e.SyncStatus()[0]
	if status.Status != "success" || status.Enqueued != 100 || status.Processed != 100 || calls.Load() != 1 || f.detailCalls.Load() != 0 {
		t.Fatalf("status=%+v calls=%d details=%d", status, calls.Load(), f.detailCalls.Load())
	}
	// Re-sync skips active jobs before any chapter/detail lookup.
	f.favorites = func(context.Context) ([]*source.Comic, error) { return []*source.Comic{{ComicID: "1"}}, nil }
	enq, skip, err := e.SyncAccount(context.Background(), id)
	if err != nil || enq != 0 || skip != 1 || f.detailCalls.Load() != 0 {
		t.Fatalf("enq=%d skip=%d err=%v", enq, skip, err)
	}
}
func TestPartialFavoritesAreEnqueuedAndRecorded(t *testing.T) {
	e := setupEngine(t)
	f := &syncTestSource{favorites: func(context.Context) ([]*source.Comic, error) {
		return []*source.Comic{{ComicID: "1"}}, errors.New("page 2 unavailable")
	}}
	id := installSyncSource(t, e, f)
	enq, _, err := e.SyncAccount(context.Background(), id)
	if enq != 1 || err == nil {
		t.Fatalf("enq=%d err=%v", enq, err)
	}
	runs, _ := e.st.SyncHistory()
	if len(runs) != 1 || runs[0].Status != "partial" || runs[0].Enqueued != 1 {
		t.Fatalf("%+v", runs)
	}
}
func TestBackgroundSyncStopsOnShutdown(t *testing.T) {
	e := setupEngine(t)
	root, cancel := context.WithCancel(context.Background())
	e.lifeCtx = root
	f := &syncTestSource{favorites: func(ctx context.Context) ([]*source.Comic, error) { <-ctx.Done(); return nil, ctx.Err() }}
	id := installSyncSource(t, e, f)
	if _, err := e.StartAccountSync(id); err != nil {
		t.Fatal(err)
	}
	cancel()
	awaitSync(t, e)
	if status := e.SyncStatus()[0]; status.Status != "failed" {
		t.Fatalf("%+v", status)
	}
}

func TestCancelingWaiterDoesNotCancelSharedManualSync(t *testing.T) {
	e := setupEngine(t)
	gate := make(chan struct{})
	started := make(chan struct{})
	f := &syncTestSource{favorites: func(ctx context.Context) ([]*source.Comic, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 9*time.Minute {
			t.Error("background task inherited a short HTTP deadline")
		}
		close(started)
		select {
		case <-gate:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	id := installSyncSource(t, e, f)
	e.StartAccountSync(id)
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := e.SyncAccount(ctx, id); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	close(gate)
	awaitSync(t, e)
	if p := e.SyncStatus()[0]; p.Status != "success" {
		t.Fatalf("%+v", p)
	}
}

func TestResumeOnlyProcessesUnfinishedCheckpoints(t *testing.T) {
	e := setupEngine(t)
	f := &syncTestSource{favorites: func(context.Context) ([]*source.Comic, error) {
		t.Error("resume should use its persisted item list")
		return nil, nil
	}}
	id := installSyncSource(t, e, f)
	e.st.PrepareSyncItems(id, []*source.Comic{{ComicID: "done"}, {ComicID: "pending"}, {ComicID: "failed"}})
	e.st.FinishSyncItem(id, "done", nil)
	e.st.FinishSyncItem(id, "failed", errors.New("temporary"))
	if _, err := e.StartAccountSyncMode(id, "resume"); err != nil {
		t.Fatal(err)
	}
	awaitSync(t, e)
	if p := e.SyncStatus()[0]; p.Status != "success" || p.Enqueued != 2 {
		t.Fatalf("%+v", p)
	}
}
func TestFreshFavoriteSnapshotAvoidsRepeatedUpstreamReads(t *testing.T) {
	e := setupEngine(t)
	var calls atomic.Int32
	f := &syncTestSource{favorites: func(context.Context) ([]*source.Comic, error) {
		calls.Add(1)
		return []*source.Comic{{ComicID: "a", Title: "Alpha"}}, nil
	}}
	id := installSyncSource(t, e, f)
	if _, err := e.RefreshFavorites(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, total, _, refreshing, err := e.CachedFavorites(id, "Alpha", 1, 20, false)
		if err != nil || total != 1 || refreshing {
			t.Fatalf("cache: %d %v", total, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cache refetched upstream")
	}
}
