package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func retryImageWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// One bounded budget includes retries and transport-slot waits. Do not evade rate limits by switching hosts.
func fetchImageRetry(parent context.Context, client *http.Client, url string, headers func(*http.Request, bool)) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	var last error
	fallback := false
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
			var upstream *UpstreamError
			if errors.As(last, &upstream) && upstream.RetryAfter > delay {
				delay = upstream.RetryAfter
			}
			if delay > 2*time.Minute {
				return nil, "", last
			}
			if err := retryImageWait(ctx, delay); err != nil {
				return nil, "", err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}
		headers(req, fallback)
		resp, err := client.Do(req)
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			last = httpFailure("图片下载", resp.StatusCode, resp.Header)
			resp.Body.Close()
			if resp.StatusCode == 403 && !fallback {
				fallback = true
				continue
			}
			if resp.StatusCode != 429 && resp.StatusCode < 500 {
				return nil, "", last
			}
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, (64<<20)+1))
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if len(raw) > 64<<20 {
			return nil, "", fmt.Errorf("图片超过大小限制")
		}
		if !isImage(raw) {
			last = responseError("图片内容格式")
			continue
		}
		return raw, resp.Header.Get("Content-Type"), nil
	}
	return nil, "", last
}
