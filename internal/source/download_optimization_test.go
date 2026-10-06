package source

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestImageRetryClassificationAndCancellation(t *testing.T) {
	for _, status := range []int{404, 429} {
		calls := 0
		client := &http.Client{Transport: picaAuthTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"3600"}}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		})}
		_, _, err := fetchImageRetry(context.Background(), client, "https://test/image", func(*http.Request, bool) {})
		if err == nil || calls != 1 {
			t.Fatalf("status=%d calls=%d err=%v", status, calls, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: picaAuthTransport(func(r *http.Request) (*http.Response, error) { cancel(); return nil, errors.New("offline") })}
	start := time.Now()
	_, _, err := fetchImageRetry(ctx, client, "https://test/image", func(*http.Request, bool) {})
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("cancel delayed: %v", err)
	}
}
func TestImageRetryRecoversServerFailure(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: picaAuthTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		status := 200
		data := tinyImage()
		if calls == 1 {
			status = 503
			data = nil
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
	b, _, err := fetchImageRetry(context.Background(), client, "https://test/image", func(*http.Request, bool) {})
	if err != nil || !validImageBytes(b) || calls != 2 {
		t.Fatalf("retry: %d %v", calls, err)
	}
}
func TestRepairReusesCommittedPages(t *testing.T) {
	old, stage := t.TempDir(), t.TempDir()
	urls := []string{"https://test/1.png", "https://test/2.png"}
	ids := append([]string{"book", "chapter"}, urls...)
	preparePageManifest(old, ids)
	preparePageManifest(stage, ids)
	os.WriteFile(filepath.Join(old, "001.png"), tinyImage(), 0600)
	os.WriteFile(filepath.Join(old, "002.png"), []byte("broken"), 0600)
	if err := seedChapterRepair(old, stage, ids, urls); err != nil {
		t.Fatal(err)
	}
	if _, ok := validImageFile(filepath.Join(stage, "001.png")); !ok {
		t.Fatal("good committed page not reused")
	}
	if _, err := os.Stat(filepath.Join(stage, "002.png")); !os.IsNotExist(err) {
		t.Fatal("corrupt page reused")
	}
}
func TestPoolsIsolateSourcesAndReleaseOnClose(t *testing.T) {
	var calls atomic.Int32
	transport := priorityTransport{base: picaAuthTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})}
	guard := func(context.Context) error { return nil }
	ctx := WithTransferPool(WithDownloadGuard(context.Background(), guard), "test-a", 1)
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://test", nil)
	first, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	next := req.Clone(blocked)
	if _, err = transport.RoundTrip(next); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("pool didn't limit")
	}
	other := WithTransferPool(WithDownloadGuard(context.Background(), guard), "test-b", 1)
	r, err := transport.RoundTrip(req.Clone(other))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	first.Body.Close()
	first.Body.Close()
	r, err = transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if calls.Load() != 3 {
		t.Fatal(calls.Load())
	}
}
func TestJMSavedReceiptMatchesEncodedFile(t *testing.T) {
	path, err := jmSaveImage(tinyImage(), 2, filepath.Join(t.TempDir(), "001.webp"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := validImageFile(path); !ok {
		t.Fatal("receipt mismatch")
	}
}
