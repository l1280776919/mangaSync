package source

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Stable categories let callers show a useful next action without logging credentials.
func ErrorKind(err error) string {
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return upstream.Category
	}
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrAuth) {
		return "auth"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return "timeout"
		}
		return "network"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "429") {
		return "rate_limit"
	}
	if strings.Contains(msg, "json") || strings.Contains(msg, "缺少") || strings.Contains(msg, "不完整") || strings.Contains(msg, "格式") {
		return "response"
	}
	return "other"
}

type UpstreamError struct {
	Category   string
	Operation  string
	Status     int
	RetryAfter time.Duration
}

func (e *UpstreamError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s: HTTP %d (%s)", e.Operation, e.Status, e.Category)
	}
	return e.Operation + ": " + e.Category
}
func responseError(op string) error { return &UpstreamError{Category: "response", Operation: op} }
func httpFailure(op string, status int, header http.Header) *UpstreamError {
	kind := "upstream"
	if status == 429 {
		kind = "rate_limit"
	}
	if status == 401 || status == 403 {
		kind = "auth"
	}
	return &UpstreamError{Category: kind, Operation: op, Status: status, RetryAfter: retryAfter(header.Get("Retry-After"))}
}
func retryAfter(value string) time.Duration {
	if n, err := strconv.Atoi(value); err == nil && n > 0 && n < 86400 {
		return time.Duration(n) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(time.Now()) {
		return time.Until(date)
	}
	return 0
}
