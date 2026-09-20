package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type Account struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	Username       string `json:"username"`
	Password       string `json:"-"`
	Label          string `json:"label"`
	Note           string `json:"note"`
	Token          string `json:"-"`
	Nickname       string `json:"nickname"`
	Level          int    `json:"level"`
	FavoritesCount int    `json:"favoritesCount"`
	FavoritesMax   int    `json:"favoritesMax"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	LastLoginAt    string `json:"lastLoginAt"`
	LastSyncAt     string `json:"lastSyncAt"`
	CreatedAt      string `json:"createdAt"`
}

type Job struct {
	ID             int64  `json:"id"`
	Kind           string `json:"kind"`
	AccountID      int64  `json:"accountId"`
	ComicID        string `json:"comicId"`
	Title          string `json:"title"`
	Chapters       []int  `json:"-"`
	Status         string `json:"status"`
	ChaptersTotal  int    `json:"chaptersTotal"`
	ChaptersDone   int    `json:"chaptersDone"`
	ImagesTotal    int    `json:"imagesTotal"`
	ImagesDone     int    `json:"imagesDone"`
	Bytes          int64  `json:"bytes"`
	SpeedBps       int64  `json:"speedBps"`
	CurrentChapter string `json:"currentChapter"`
	LocalPath      string `json:"localPath"`
	Error          string `json:"error"`
	CreatedAt      string `json:"createdAt"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
}

type Comic struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"`
	ComicID      string `json:"comicId"`
	Title        string `json:"title"`
	Path         string `json:"path"`
	Chapters     int    `json:"chapters"`
	ChaptersDone int    `json:"chaptersDone"`
	Images       int    `json:"images"`
	Bytes        int64  `json:"bytes"`
	Complete     bool   `json:"complete"`
	UpdatedAt    string `json:"updatedAt"`
	Source       string `json:"source"`
}

func now() string { return time.Now().Format(time.RFC3339) }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "mangasync.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`create table if not exists accounts(
			id integer primary key autoincrement,
			kind text not null, username text not null, password text default '',
			label text default '', note text default '', token text default '',
			nickname text default '', level integer default 0,
			favorites_count integer default 0, favorites_max integer default 0,
			status text default 'unknown', error text default '',
			last_login_at text default '', last_sync_at text default '', created_at text)`,
		`create unique index if not exists idx_accounts on accounts(kind, username)`,
		`create table if not exists jobs(
			id integer primary key autoincrement,
			kind text, account_id integer default 0, comic_id text, title text,
			chapters text default '[]', status text default 'queued',
			chapters_total integer default 0, chapters_done integer default 0,
			images_total integer default 0, images_done integer default 0,
			bytes integer default 0, speed_bps integer default 0,
			current_chapter text default '', local_path text default '', error text default '',
			created_at text, started_at text default '', finished_at text default '')`,
		`create table if not exists job_logs(
			id integer primary key autoincrement, job_id integer, ts text, level text, msg text)`,
		`create index if not exists idx_job_logs on job_logs(job_id, id)`,
		`create table if not exists comics(
			id integer primary key autoincrement,
			kind text not null, comic_id text not null, title text default '', path text default '',
			chapters integer default 0, chapters_done integer default 0,
			images integer default 0, bytes integer default 0,
			complete integer default 0, updated_at text default '', source text default 'download')`,
		`create unique index if not exists idx_comics on comics(kind, comic_id)`,
		`create table if not exists users(
			id integer primary key autoincrement,
			username text not null unique,
			pass_hash text not null, salt text not null, iterations integer not null default 200000,
			is_admin integer default 1, must_change_password integer default 0,
			created_at text default '', last_login_at text default '', password_changed_at text default '')`,
		`create table if not exists sessions(
			token text primary key, user_id integer not null,
			created_at text default '', expires_at text default '', user_agent text default '')`,
		`create index if not exists idx_sessions_user on sessions(user_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w (%s)", err, q[:40])
		}
	}
	return nil
}

// ---------------- accounts ----------------

func (s *Store) ListAccounts() ([]*Account, error) {
	rows, err := s.db.Query(`select id,kind,username,label,note,nickname,level,favorites_count,favorites_max,
		status,error,last_login_at,last_sync_at,created_at from accounts order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Account
	for rows.Next() {
		a := &Account{}
		if err := rows.Scan(&a.ID, &a.Kind, &a.Username, &a.Label, &a.Note, &a.Nickname, &a.Level,
			&a.FavoritesCount, &a.FavoritesMax, &a.Status, &a.Error, &a.LastLoginAt, &a.LastSyncAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// FindAccountWithToken 找一个指定源、且已登录（带 token）的账号
func (s *Store) FindAccountWithToken(kind string) (*Account, error) {
	var id int64
	err := s.db.QueryRow(`select id from accounts where kind=? and token!='' order by id limit 1`, kind).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

func (s *Store) GetAccount(id int64) (*Account, error) {
	a := &Account{}
	var pw, tk string
	err := s.db.QueryRow(`select id,kind,username,password,token,label,note,nickname,level,favorites_count,
		favorites_max,status,error,last_login_at,last_sync_at,created_at from accounts where id=?`, id).
		Scan(&a.ID, &a.Kind, &a.Username, &pw, &tk, &a.Label, &a.Note, &a.Nickname, &a.Level, &a.FavoritesCount,
			&a.FavoritesMax, &a.Status, &a.Error, &a.LastLoginAt, &a.LastSyncAt, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	a.Password, a.Token = pw, tk
	return a, nil
}

func (s *Store) CreateAccount(a *Account) (int64, error) {
	a.CreatedAt = now()
	res, err := s.db.Exec(`insert into accounts(kind,username,password,label,note,status,created_at)
		values(?,?,?,?,?,?,?)`, a.Kind, a.Username, a.Password, a.Label, a.Note, "unknown", a.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("该源下已存在同名账号")
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAccount(id int64, label, note, username, password string) error {
	sets := []string{"label=?", "note=?"}
	args := []any{label, note}
	if username != "" {
		sets = append(sets, "username=?")
		args = append(args, username)
	}
	if password != "" {
		sets = append(sets, "password=?")
		args = append(args, password)
	}
	args = append(args, id)
	_, err := s.db.Exec("update accounts set "+strings.Join(sets, ",")+" where id=?", args...)
	return err
}

func (s *Store) DeleteAccount(id int64) error {
	_, err := s.db.Exec(`delete from accounts where id=?`, id)
	return err
}

func (s *Store) SaveLoginOK(id int64, token, nickname string, level, favCount, favMax int) error {
	_, err := s.db.Exec(`update accounts set token=?,nickname=?,level=?,favorites_count=?,favorites_max=?,
		status='ok',error='',last_login_at=? where id=?`, token, nickname, level, favCount, favMax, now(), id)
	return err
}

func (s *Store) SaveLoginErr(id int64, msg string) error {
	_, err := s.db.Exec(`update accounts set status='error',error=? where id=?`, msg, id)
	return err
}

func (s *Store) TouchSync(id int64) error {
	_, err := s.db.Exec(`update accounts set last_sync_at=? where id=?`, now(), id)
	return err
}

// ---------------- jobs ----------------

func (s *Store) CreateJob(j *Job) (int64, error) {
	ch, _ := json.Marshal(j.Chapters)
	j.CreatedAt = now()
	res, err := s.db.Exec(`insert into jobs(kind,account_id,comic_id,title,chapters,status,created_at)
		values(?,?,?,?,?,?,?)`, j.Kind, j.AccountID, j.ComicID, j.Title, string(ch), "queued", j.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) scanJob(scan func(dest ...any) error) (*Job, error) {
	j := &Job{}
	var ch string
	if err := scan(&j.ID, &j.Kind, &j.AccountID, &j.ComicID, &j.Title, &ch, &j.Status, &j.ChaptersTotal,
		&j.ChaptersDone, &j.ImagesTotal, &j.ImagesDone, &j.Bytes, &j.SpeedBps, &j.CurrentChapter,
		&j.LocalPath, &j.Error, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(ch), &j.Chapters)
	return j, nil
}

const jobCols = `id,kind,account_id,comic_id,title,chapters,status,chapters_total,chapters_done,images_total,
	images_done,bytes,speed_bps,current_chapter,local_path,error,created_at,started_at,finished_at`

func (s *Store) GetJob(id int64) (*Job, error) {
	row := s.db.QueryRow(`select `+jobCols+` from jobs where id=?`, id)
	return s.scanJob(row.Scan)
}

func (s *Store) ListJobs(status string, limit, offset int) ([]*Job, int, error) {
	where, args := "", []any{}
	if status != "" {
		where = " where status=?"
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRow("select count(*) from jobs"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query("select "+jobCols+" from jobs"+where+" order by id desc limit ? offset ?", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := s.scanJob(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, nil
}

func (s *Store) NextQueuedJob() (*Job, error) {
	row := s.db.QueryRow(`select ` + jobCols + ` from jobs where status='queued' order by id asc limit 1`)
	j, err := s.scanJob(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

func (s *Store) UpdateJobProgress(id int64, status, currentChapter string, chTotal, chDone, imgTotal, imgDone int, bytes, speed int64) error {
	_, err := s.db.Exec(`update jobs set status=?,current_chapter=?,chapters_total=?,chapters_done=?,
		images_total=?,images_done=?,bytes=?,speed_bps=? where id=?`,
		status, currentChapter, chTotal, chDone, imgTotal, imgDone, bytes, speed, id)
	return err
}

func (s *Store) SetJobStatus(id int64, status, errMsg string) error {
	finished := ""
	if status == "done" || status == "failed" || status == "canceled" {
		finished = now()
	}
	if status == "running" {
		_, err := s.db.Exec(`update jobs set status=?,error='',started_at=? where id=?`, status, now(), id)
		return err
	}
	_, err := s.db.Exec(`update jobs set status=?,error=?,finished_at=?,speed_bps=0 where id=?`, status, errMsg, finished, id)
	return err
}

func (s *Store) SetJobTitle(id int64, title string) error {
	_, err := s.db.Exec(`update jobs set title=? where id=?`, title, id)
	return err
}

func (s *Store) SetJobPath(id int64, path string) error {
	_, err := s.db.Exec(`update jobs set local_path=? where id=?`, path, id)
	return err
}

func (s *Store) RequeueJob(id int64) error {
	_, err := s.db.Exec(`update jobs set status='queued',error='',finished_at='',started_at='',speed_bps=0,
		chapters_done=0,images_done=0 where id=?`, id)
	return err
}

func (s *Store) DeleteJob(id int64) error {
	if _, err := s.db.Exec(`delete from job_logs where job_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`delete from jobs where id=?`, id)
	return err
}

func (s *Store) AddJobLog(jobID int64, level, msg string) error {
	_, err := s.db.Exec(`insert into job_logs(job_id,ts,level,msg) values(?,?,?,?)`, jobID, now(), level, msg)
	return err
}

type JobLog struct {
	TS    string `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

func (s *Store) JobLogs(jobID int64, limit int) ([]JobLog, error) {
	rows, err := s.db.Query(`select ts,level,msg from job_logs where job_id=? order by id desc limit ?`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobLog
	for rows.Next() {
		var l JobLog
		if err := rows.Scan(&l.TS, &l.Level, &l.Msg); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	// 反过来（旧 -> 新）
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) JobCounts() (map[string]int, error) {
	rows, err := s.db.Query(`select status,count(*) from jobs group by status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{"queued": 0, "running": 0, "done": 0, "failed": 0, "canceled": 0}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, nil
}

// ---------------- comics ----------------

func (s *Store) UpsertComic(c *Comic) error {
	if c.UpdatedAt == "" {
		c.UpdatedAt = now()
	}
	if c.Source == "" {
		c.Source = "download"
	}
	_, err := s.db.Exec(`insert into comics(kind,comic_id,title,path,chapters,chapters_done,images,bytes,complete,updated_at,source)
		values(?,?,?,?,?,?,?,?,?,?,?)
		on conflict(kind,comic_id) do update set title=excluded.title, path=excluded.path,
		chapters=excluded.chapters, chapters_done=excluded.chapters_done, images=excluded.images,
		bytes=excluded.bytes, complete=excluded.complete,
		updated_at=case when excluded.source='download' then excluded.updated_at else comics.updated_at end,
		source=excluded.source`,
		c.Kind, c.ComicID, c.Title, c.Path, c.Chapters, c.ChaptersDone, c.Images, c.Bytes, boolToInt(c.Complete), c.UpdatedAt, c.Source)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) GetComic(kind, comicID string) (*Comic, error) {
	row := s.db.QueryRow(`select id,kind,comic_id,title,path,chapters,chapters_done,images,bytes,complete,updated_at,source
		from comics where kind=? and comic_id=?`, kind, comicID)
	return s.scanComic(row.Scan)
}

func (s *Store) scanComic(scan func(dest ...any) error) (*Comic, error) {
	c := &Comic{}
	var complete int
	if err := scan(&c.ID, &c.Kind, &c.ComicID, &c.Title, &c.Path, &c.Chapters, &c.ChaptersDone,
		&c.Images, &c.Bytes, &complete, &c.UpdatedAt, &c.Source); err != nil {
		return nil, err
	}
	c.Complete = complete == 1
	return c, nil
}

func (s *Store) ListComics(kind, keyword, sortBy string, limit, offset int) ([]*Comic, int, error) {
	where := []string{}
	args := []any{}
	if kind != "" {
		where = append(where, "kind=?")
		args = append(args, kind)
	}
	if keyword != "" {
		where = append(where, "(title like ? or comic_id like ? or path like ?)")
		kw := "%" + keyword + "%"
		args = append(args, kw, kw, kw)
	}
	cond := ""
	if len(where) > 0 {
		cond = " where " + strings.Join(where, " and ")
	}
	var total int
	if err := s.db.QueryRow("select count(*) from comics"+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := " order by updated_at desc"
	if sortBy == "size" {
		order = " order by bytes desc"
	} else if sortBy == "title" {
		order = " order by title asc"
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query("select id,kind,comic_id,title,path,chapters,chapters_done,images,bytes,complete,updated_at,source from comics"+cond+order+" limit ? offset ?", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Comic
	for rows.Next() {
		c, err := s.scanComic(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, nil
}

func (s *Store) AllComics() ([]*Comic, error) {
	rows, err := s.db.Query(`select id,kind,comic_id,title,path,chapters,chapters_done,images,bytes,complete,updated_at,source from comics`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Comic
	for rows.Next() {
		c, err := s.scanComic(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) DeleteComic(id int64) (*Comic, error) {
	c, err := func() (*Comic, error) {
		row := s.db.QueryRow(`select id,kind,comic_id,title,path,chapters,chapters_done,images,bytes,complete,updated_at,source from comics where id=?`, id)
		return s.scanComic(row.Scan)
	}()
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`delete from comics where id=?`, id)
	return c, err
}

func (s *Store) ComicStats() (comics, chapters, images int, bytes int64, err error) {
	err = s.db.QueryRow(`select count(*), coalesce(sum(chapters),0), coalesce(sum(images),0), coalesce(sum(bytes),0) from comics`).
		Scan(&comics, &chapters, &images, &bytes)
	return
}
