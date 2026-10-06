package source

import (
	"context"
	"io"
	"net/http"
	"sync"
)

type guardKey struct{}

func WithDownloadGuard(ctx context.Context, guard func(context.Context) error) context.Context {
	return context.WithValue(ctx, guardKey{}, guard)
}

// Background transfers leave capacity for reader/API traffic. Slots are held until the body closes.
var backgroundSlots = make(chan struct{}, 4)

type priorityTransport struct{ base http.RoundTripper }
type releaseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *releaseBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }
func (t priorityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	guard, _ := r.Context().Value(guardKey{}).(func(context.Context) error)
	if guard == nil {
		return t.base.RoundTrip(r)
	}
	select {
	case backgroundSlots <- struct{}{}:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	release := func() { <-backgroundSlots }
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		release()
		return nil, err
	}
	resp.Body = &releaseBody{ReadCloser: resp.Body, release: release}
	return resp, nil
}

func waitDownload(ctx context.Context) error {
	if guard, ok := ctx.Value(guardKey{}).(func(context.Context) error); ok {
		return guard(ctx)
	}
	return ctx.Err()
}
