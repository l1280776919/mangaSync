package api

import (
	"context"
	"encoding/json"
	"github.com/l1280776919/mangaSync/internal/config"
	"github.com/l1280776919/mangaSync/internal/engine"
	"github.com/l1280776919/mangaSync/internal/store"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSyncRequestCancellationDoesNotCancelBackgroundRun(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.New(st, cfg)
	s := NewServer(st, cfg, eng)
	id, err := st.CreateAccount(&store.Account{Kind: "pica", Username: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/api/accounts/1/sync", nil).WithContext(ctx)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.syncAccount(w, req)
	if w.Code != 202 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var result engine.SyncProgress
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.ID == 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	wait, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	eng.WaitSync(wait)
	runs, err := st.SyncHistory()
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	// This intentionally unconfigured test account fails on credentials, not on the HTTP context.
	if runs[0].Status != "failed" || strings.Contains(runs[0].Error, "context canceled") {
		t.Fatalf("%+v", runs[0])
	}
}
