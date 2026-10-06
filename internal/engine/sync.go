package engine

import (
	"context"
	"fmt"
	"github.com/l1280776919/mangaSync/internal/source"
	"time"
)

// One current run per account; only two accounts perform upstream work concurrently.
type SyncProgress struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"accountId"`
	Status    string `json:"status"`
	Mode      string `json:"mode"`
	ErrorKind string `json:"errorKind"`
	Stage     string `json:"stage"`
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Enqueued  int    `json:"enqueued"`
	Skipped   int    `json:"skipped"`
	Error     string `json:"error"`
}
type syncJob struct {
	progress SyncProgress
	done     chan struct{}
	err      error
}

func activeSync(s string) bool { return s == "queued" || s == "running" }

func (e *Engine) launchSync(parent context.Context, id int64, mode string) (*syncJob, error) {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	if j := e.syncJobs[id]; j != nil && activeSync(j.progress.Status) {
		return j, nil
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if _, err := e.st.GetAccount(id); err != nil {
		return nil, err
	}
	runID, err := e.st.StartSync(id)
	if err != nil {
		return nil, err
	}
	j := &syncJob{progress: SyncProgress{ID: runID, AccountID: id, Status: "queued", Stage: "queued", Mode: mode}, done: make(chan struct{})}
	e.syncJobs[id] = j
	go func() {
		ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
		defer cancel()
		var enq, skip int
		var runErr error
		select {
		case e.syncSlots <- struct{}{}:
			e.updateSync(id, "favorites", 0, 0, 0, 0)
			enq, skip, runErr = e.syncAccount(ctx, id, mode)
			<-e.syncSlots
		case <-ctx.Done():
			runErr = ctx.Err()
		}
		if err := e.st.FinishSync(runID, enq, skip, runErr); err != nil {
			runErr = fmt.Errorf("保存同步记录失败: %w", err)
		}
		e.syncMu.Lock()
		j.progress.Enqueued, j.progress.Skipped = enq, skip
		j.progress.Status = "success"
		if runErr != nil {
			j.progress.Status = "failed"
			if enq+skip > 0 {
				j.progress.Status = "partial"
			}
			j.progress.Error = runErr.Error()
			j.progress.ErrorKind = source.ErrorKind(runErr)
		}
		j.progress.Stage = "finished"
		j.err = runErr
		snapshot := j.progress
		close(j.done)
		e.syncMu.Unlock()
		e.Broadcast("sync", snapshot)
	}()
	return j, nil
}

// StartAccountSync is detached from the initiating HTTP request, but stops on service shutdown.
func (e *Engine) StartAccountSync(id int64) (SyncProgress, error) {
	return e.StartAccountSyncMode(id, "incremental")
}
func (e *Engine) StartAccountSyncMode(id int64, mode string) (SyncProgress, error) {
	if mode != "incremental" && mode != "full" && mode != "resume" && mode != "retry" {
		return SyncProgress{}, fmt.Errorf("同步模式不合法")
	}
	e.syncMu.Lock()
	ctx := e.lifeCtx
	e.syncMu.Unlock()
	j, err := e.launchSync(ctx, id, mode)
	if err != nil {
		return SyncProgress{}, err
	}
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	return j.progress, nil
}

// Scheduled sync shares the same run as a manual sync already in progress.
func (e *Engine) SyncAccount(ctx context.Context, id int64) (int, int, error) {
	j, err := e.launchSync(ctx, id, "incremental")
	if err != nil {
		return 0, 0, err
	}
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case <-j.done:
		e.syncMu.Lock()
		defer e.syncMu.Unlock()
		return j.progress.Enqueued, j.progress.Skipped, j.err
	}
}
func (e *Engine) updateSync(id int64, stage string, processed, total, enq, skip int) {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	if j := e.syncJobs[id]; j != nil {
		j.progress.Status = "running"
		j.progress.Stage = stage
		j.progress.Processed = processed
		j.progress.Total = total
		j.progress.Enqueued = enq
		j.progress.Skipped = skip
	}
}
func (e *Engine) SyncStatus() []SyncProgress {
	e.syncMu.Lock()
	out := make([]SyncProgress, 0, len(e.syncJobs))
	seen := map[int64]bool{}
	for _, j := range e.syncJobs {
		out = append(out, j.progress)
		seen[j.progress.AccountID] = true
	}
	e.syncMu.Unlock()
	if history, err := e.st.SyncHistory(); err == nil {
		for _, r := range history {
			if seen[r.AccountID] {
				continue
			}
			seen[r.AccountID] = true
			out = append(out, SyncProgress{ID: r.ID, AccountID: r.AccountID, Status: r.Status, Stage: "finished", Enqueued: r.Enqueued, Skipped: r.Skipped, Error: r.Error})
		}
	}
	return out
}
func (e *Engine) WaitSync(ctx context.Context) {
	e.syncMu.Lock()
	jobs := make([]*syncJob, 0, len(e.syncJobs))
	for _, j := range e.syncJobs {
		jobs = append(jobs, j)
	}
	e.syncMu.Unlock()
	for _, j := range jobs {
		select {
		case <-j.done:
		case <-ctx.Done():
			return
		}
	}
}
