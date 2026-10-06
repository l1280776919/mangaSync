package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Store) migrateFeatures() error {
	for _, q := range []string{
		`create table if not exists chapter_checks(kind text, comic_id text, chapter_order integer, remote_id text, images integer, path text, primary key(kind,comic_id,chapter_order))`,
		`create table if not exists sync_runs(id integer primary key, account_id integer, started_at text, finished_at text default '', status text, enqueued integer default 0, skipped integer default 0, error text default '')`,
		`create table if not exists reading_progress(user_id integer,kind text,comic_id text,title text,chapter_order integer,page integer,updated_at integer,primary key(user_id,kind,comic_id))`,
		`create table if not exists trash(id integer primary key,kind text,comic_id text,original_path text,path text,comic_json text,status text,created_at text)`,
		`create index if not exists idx_jobs_status on jobs(status,id)`,
		`update jobs set status='canceled',error='合并历史重复任务' where status in ('queued','running') and id not in (select min(id) from jobs where status in ('queued','running') group by kind,comic_id)`,
		`create unique index if not exists idx_jobs_active on jobs(kind,comic_id) where status in ('queued','running')`,
		`update sync_runs set status='failed',error='服务重启中断同步',finished_at=datetime('now') where status='running'`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

type ChapterCheck struct {
	Order    int
	RemoteID string
	Images   int
	Path     string
}

func (s *Store) SaveChapter(kind, id string, c ChapterCheck) error {
	_, err := s.db.Exec(`insert into chapter_checks values(?,?,?,?,?,?) on conflict(kind,comic_id,chapter_order) do update set remote_id=excluded.remote_id,images=excluded.images,path=excluded.path`, kind, id, c.Order, c.RemoteID, c.Images, c.Path)
	return err
}
func (s *Store) Chapters(kind, id string) (map[int]ChapterCheck, error) {
	rows, err := s.db.Query(`select chapter_order,remote_id,images,path from chapter_checks where kind=? and comic_id=?`, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]ChapterCheck{}
	for rows.Next() {
		var c ChapterCheck
		if err = rows.Scan(&c.Order, &c.RemoteID, &c.Images, &c.Path); err != nil {
			return nil, err
		}
		out[c.Order] = c
	}
	return out, rows.Err()
}
func (s *Store) SetCompleteness(kind, id string, total, done int) error {
	_, err := s.db.Exec(`update comics set chapters=?,chapters_done=?,complete=? where kind=? and comic_id=?`, total, done, total > 0 && done >= total, kind, id)
	return err
}

type SyncRun struct {
	ID         int64  `json:"id"`
	AccountID  int64  `json:"accountId"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	Status     string `json:"status"`
	Enqueued   int    `json:"enqueued"`
	Skipped    int    `json:"skipped"`
	Error      string `json:"error"`
}

func (s *Store) StartSync(account int64) (int64, error) {
	r, e := s.db.Exec(`insert into sync_runs(account_id,started_at,status) values(?,?,'running')`, account, now())
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
func (s *Store) FinishSync(id int64, enq, skip int, err error) error {
	status, msg := "success", ""
	if err != nil {
		status = "failed"
		msg = err.Error()
		if enq+skip > 0 {
			status = "partial"
		}
	}
	_, e := s.db.Exec(`update sync_runs set finished_at=?,status=?,enqueued=?,skipped=?,error=? where id=?`, now(), status, enq, skip, msg, id)
	return e
}
func (s *Store) SyncHistory() ([]SyncRun, error) {
	rows, e := s.db.Query(`select id,account_id,started_at,finished_at,status,enqueued,skipped,error from sync_runs order by id desc limit 50`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []SyncRun{}
	for rows.Next() {
		var x SyncRun
		if e = rows.Scan(&x.ID, &x.AccountID, &x.StartedAt, &x.FinishedAt, &x.Status, &x.Enqueued, &x.Skipped, &x.Error); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

type ReadingProgress struct {
	Kind      string `json:"kind"`
	ComicID   string `json:"comicId"`
	Title     string `json:"title"`
	Order     int    `json:"order"`
	Page      int    `json:"page"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (s *Store) SaveReading(user int64, p ReadingProgress) error {
	_, e := s.db.Exec(`insert into reading_progress values(?,?,?,?,?,?,?) on conflict(user_id,kind,comic_id) do update set title=excluded.title,chapter_order=excluded.chapter_order,page=excluded.page,updated_at=excluded.updated_at where excluded.updated_at>reading_progress.updated_at`, user, p.Kind, p.ComicID, p.Title, p.Order, p.Page, p.UpdatedAt)
	return e
}
func (s *Store) Reading(user int64, kind, id string) ([]ReadingProgress, error) {
	q := `select kind,comic_id,title,chapter_order,page,updated_at from reading_progress where user_id=?`
	args := []any{user}
	if id != "" {
		q += ` and kind=? and comic_id=?`
		args = append(args, kind, id)
	}
	q += ` order by updated_at desc limit 20`
	rows, e := s.db.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ReadingProgress{}
	for rows.Next() {
		var p ReadingProgress
		if e = rows.Scan(&p.Kind, &p.ComicID, &p.Title, &p.Order, &p.Page, &p.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ComicByID(id int64) (*Comic, error) {
	return s.scanComic(s.db.QueryRow(`select id,kind,comic_id,title,path,chapters,chapters_done,images,bytes,complete,updated_at,source from comics where id=?`, id).Scan)
}

type Trash struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"`
	ComicID      string `json:"comicId"`
	OriginalPath string `json:"originalPath"`
	Path         string `json:"path"`
	ComicJSON    string `json:"-"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
}

func (s *Store) PrepareTrash(c *Comic, path string) (int64, error) {
	b, _ := json.Marshal(c)
	r, e := s.db.Exec(`insert into trash(kind,comic_id,original_path,path,comic_json,status,created_at) values(?,?,?,?,?,'pending',?)`, c.Kind, c.ComicID, c.Path, path, string(b), now())
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
func (s *Store) FinishTrash(id, comicID int64) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`delete from comics where id=?`, comicID); e != nil {
		return e
	}
	if _, e = tx.Exec(`update trash set status='trashed' where id=?`, id); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) TrashItems() ([]Trash, error) {
	rows, e := s.db.Query(`select id,kind,comic_id,original_path,path,comic_json,status,created_at from trash where status in ('pending','trashed') order by id desc`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Trash{}
	for rows.Next() {
		var t Trash
		if e = rows.Scan(&t.ID, &t.Kind, &t.ComicID, &t.OriginalPath, &t.Path, &t.ComicJSON, &t.Status, &t.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) SetTrashStatus(id int64, status string) error {
	_, e := s.db.Exec(`update trash set status=? where id=?`, status, id)
	return e
}
func (s *Store) InTrash(kind, id string) (bool, error) {
	var n int
	e := s.db.QueryRow(`select count(*) from trash where kind=? and comic_id=? and status in ('pending','trashed')`, kind, id).Scan(&n)
	return n > 0, e
}

// A single transaction plus the partial unique index guards all enqueue/retry callers.
func (s *Store) enqueue(j *Job) (int64, error) {
	tx, e := s.db.Begin()
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	var id int64
	e = tx.QueryRow(`select id from jobs where kind=? and comic_id=? and status in ('queued','running')`, j.Kind, j.ComicID).Scan(&id)
	if e == nil {
		return id, nil
	}
	if e != sql.ErrNoRows {
		return 0, e
	}
	ch, _ := json.Marshal(j.Chapters)
	r, e := tx.Exec(`insert into jobs(kind,account_id,comic_id,title,chapters,status,created_at) values(?,?,?,?,?,'queued',?)`, j.Kind, j.AccountID, j.ComicID, j.Title, string(ch), now())
	if e != nil {
		return 0, e
	}
	id, e = r.LastInsertId()
	if e != nil {
		return 0, e
	}
	return id, tx.Commit()
}
func (s *Store) ClaimJob(id int64) (bool, error) {
	r, e := s.db.Exec(`update jobs set status='running',started_at=?,error='' where id=? and status='queued'`, now(), id)
	if e != nil {
		return false, e
	}
	n, e := r.RowsAffected()
	return n == 1, e
}
func (s *Store) CancelQueued(id int64) error {
	r, e := s.db.Exec(`update jobs set status='canceled',finished_at=? where id=? and status='queued'`, now(), id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return fmt.Errorf("任务状态已变化，请刷新")
	}
	return nil
}
func (s *Store) PruneHistory() {
	cutoff := time.Now().AddDate(0, 0, -30).Format(time.RFC3339)
	s.db.Exec(`delete from job_logs where job_id in (select id from jobs where status in ('done','failed','canceled') and finished_at<?)`, cutoff)
	s.db.Exec(`delete from sync_runs where status!='running' and started_at<?`, cutoff)
}
