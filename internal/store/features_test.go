package store

import (
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}
func TestConcurrentEnqueueAndRetry(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	ids := make(chan int64, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := s.CreateJob(&Job{Kind: "jm", ComicID: "1"})
			if e != nil {
				t.Error(e)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	var first int64
	for id := range ids {
		if first == 0 {
			first = id
		}
		if id != first {
			t.Fatalf("duplicate jobs %d %d", first, id)
		}
	}
	s.SetJobStatus(first, "done", "")
	next, e := s.CreateJob(&Job{Kind: "jm", ComicID: "1"})
	if e != nil {
		t.Fatal(e)
	}
	if next == first {
		t.Fatal("completed job reused")
	}
	if e = s.RequeueJob(first); e == nil {
		t.Fatal("retry bypassed active unique index")
	}
	if e = s.CancelQueued(next); e != nil {
		t.Fatal(e)
	}
	if claimed, e := s.ClaimJob(next); e != nil || claimed {
		t.Fatal("canceled job was claimed")
	}
}
func TestReadingProgressIsolationAndOrdering(t *testing.T) {
	s := testStore(t)
	p := ReadingProgress{Kind: "jm", ComicID: "1", Order: 2, Page: 8, UpdatedAt: 200}
	if e := s.SaveReading(1, p); e != nil {
		t.Fatal(e)
	}
	p.Page = 2
	p.UpdatedAt = 100
	s.SaveReading(1, p)
	a, e := s.Reading(1, "jm", "1")
	if e != nil || len(a) != 1 || a[0].Page != 8 {
		t.Fatalf("stale progress won: %+v %v", a, e)
	}
	b, _ := s.Reading(2, "jm", "1")
	if len(b) != 0 {
		t.Fatal("cross-user leak")
	}
}
func TestPartialSyncHistory(t *testing.T) {
	s := testStore(t)
	id, _ := s.StartSync(1)
	s.FinishSync(id, 2, 1, ErrNotFound)
	runs, e := s.SyncHistory()
	if e != nil || len(runs) != 1 || runs[0].Status != "partial" || runs[0].Enqueued != 2 {
		t.Fatalf("%+v %v", runs, e)
	}
}
