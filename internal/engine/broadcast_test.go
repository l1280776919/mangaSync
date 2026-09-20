package engine

import (
	"sync"
	"testing"
)

// H1: 广播与退订并发时不得 panic（原来 Unsubscribe 在锁外 close(ch) → send on closed channel）
func TestH1_NoSendOnClosedChannel(t *testing.T) {
	e := New(nil, nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5000; i++ {
			e.broadcastEvent("job", map[string]any{"i": i})
		}
	}()
	for i := 0; i < 500; i++ {
		ch := e.Subscribe()
		e.Unsubscribe(ch)
	}
	wg.Wait()
}

// H1: 退订后 channel 不被 close（handler 靠 request ctx 退出）
func TestH1_UnsubscribeDoesNotClose(t *testing.T) {
	e := New(nil, nil)
	ch := e.Subscribe()
	e.Unsubscribe(ch)
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("Unsubscribe 关闭了 channel：仍可能 panic")
		}
	default:
	}
}

// H3: 源实例按配置缓存，重复调用返回同一实例；配置指纹变化后重建
func TestH3_SourceCached(t *testing.T) {
	cfg := testCfg(t, "http://127.0.0.1:1")
	e := New(nil, cfg)
	a, err := e.SourceFor("pica")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := e.SourceFor("pica")
	if a != b {
		t.Fatal("SourceFor 未复用实例（连接池与章节缓存会失效）")
	}
	e.InvalidateSources()
	c, _ := e.SourceFor("pica")
	if a == c {
		t.Fatal("InvalidateSources 后应重建实例")
	}
}
