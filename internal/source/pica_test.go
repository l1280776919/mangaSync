package source

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// H2: ctx 取消后 downloadImages 必须立刻返回（原来生产者裸阻塞发送 → 永久 hang）
func TestH2_DownloadImagesCancelReturns(t *testing.T) {
	p := NewPica("", "original", 4)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	urls := make([]string, 50)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.invalid/%d.jpg", i)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, err := p.downloadImages(ctx, urls, t.TempDir(), Hooks{}, Progress{})
		if err == nil {
			t.Errorf("取消后应返回错误")
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("downloadImages 在 ctx 取消后 5s 仍未返回（H2 未修复）")
	}
}

// M2: 临时目录名随任务号变化（不同任务不会互相 RemoveAll）
func TestM2_TmpDirNameIsolated(t *testing.T) {
	a := tmpDirName("001 - x", 1)
	b := tmpDirName("001 - x", 2)
	if a == b {
		t.Fatalf("不同任务号生成了相同的临时目录名: %s", a)
	}
	c := tmpDirName("001 - x", 0)
	d := tmpDirName("001 - x", 0)
	if c == d {
		t.Fatalf("无任务号时应使用 pid+纳秒兜底: %s", c)
	}
}
