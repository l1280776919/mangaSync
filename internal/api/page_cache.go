package api

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

type pageFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	data    []byte
	ct      string
	err     error
}

var readerSlots = make(chan struct{}, 4)

// Coalesce concurrent readers, but cancel upstream when the last reader leaves.
func (s *Server) fetchPage(ctx context.Context, key string, load func(context.Context) ([]byte, string, error)) ([]byte, string, error) {
	s.pageMu.Lock()
	f := s.pages[key]
	if f == nil {
		upstream, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		f = &pageFlight{done: make(chan struct{}), cancel: cancel}
		s.pages[key] = f
		go func() {
			defer cancel()
			select {
			case readerSlots <- struct{}{}:
				f.data, f.ct, f.err = load(upstream)
				<-readerSlots
			case <-upstream.Done():
				f.err = upstream.Err()
			}
			s.pageMu.Lock()
			if s.pages[key] == f {
				delete(s.pages, key)
			}
			close(f.done)
			s.pageMu.Unlock()
		}()
	}
	f.waiters++
	s.pageMu.Unlock()
	defer func() {
		s.pageMu.Lock()
		f.waiters--
		if f.waiters == 0 {
			f.cancel()
			if s.pages[key] == f {
				delete(s.pages, key)
			}
		}
		s.pageMu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-f.done:
		return f.data, f.ct, f.err
	}
}
func atomicCache(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".page-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
