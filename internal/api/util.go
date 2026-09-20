package api

import (
	"context"
	"syscall"
	"time"
)

// rctx 给不持有 *http.Request 的内部调用用；带兜底超时避免卡死
func rctx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	_ = cancel
	return ctx
}

func rctxTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

type unixStatfs = syscall.Statfs_t

func statfs(path string, st *syscall.Statfs_t) error {
	return syscall.Statfs(path, st)
}
