package api

import (
	"context"
	"net/http"
	"syscall"
	"time"
)

// ctxTimeout 由「当前请求」派生的内部调用上下文 + 兜底超时。
// 刻意不用 context.Background()：请求上下文在客户端断开（关页面/翻页走人）时会取消，
// 上游请求（登录、Detail、Cover、收藏翻页…）能跟着停掉；用 Background 的话
// 每次调用都会留一个跑到超时为止的 timer，并把并发槽占满。
// 只有下载任务那种要脱离请求生命周期的场景才用 Background（engine 里由 Start 的 ctx 管辖）。
func ctxTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

type unixStatfs = syscall.Statfs_t

func statfs(path string, st *syscall.Statfs_t) error {
	return syscall.Statfs(path, st)
}
