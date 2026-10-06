package api

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPageFlightsCoalesceAndCancel(t *testing.T) {
	s := &Server{pages: map[string]*pageFlight{}}
	var calls atomic.Int32
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	load := func(ctx context.Context) ([]byte, string, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-gate:
			return []byte("ok"), "image/jpeg", nil
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, _, e := s.fetchPage(context.Background(), "same", load)
			if e != nil || string(b) != "ok" {
				t.Errorf("%s %v", b, e)
			}
		}()
	}
	<-started
	deadline := time.Now().Add(time.Second)
	for {
		s.pageMu.Lock()
		f := s.pages["same"]
		n := f.waiters
		s.pageMu.Unlock()
		if n == 8 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiters did not join")
		}
		time.Sleep(time.Millisecond)
	}
	close(gate)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan struct{})
	go s.fetchPage(ctx, "cancel", func(up context.Context) ([]byte, string, error) {
		<-up.Done()
		close(canceled)
		return nil, "", up.Err()
	})
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream not canceled")
	}
}
func TestAtomicCacheAndCapacity(t *testing.T) {
	s := &Server{base: t.TempDir()}
	p := filepath.Join(s.base, "reader", "one.jpg")
	if e := atomicCache(p, []byte("123456")); e != nil {
		t.Fatal(e)
	}
	if e := atomicCache(p, []byte("abc")); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "abc" {
		t.Fatal(string(b))
	}
	if n := s.TrimReaderCache(2); n != 3 {
		t.Fatal(n)
	}
}
