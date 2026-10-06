package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/l1280776919/mangaSync/internal/source"
	"strings"
)

type FavoriteState struct {
	UpdatedAt   string `json:"updatedAt"`
	AttemptAt   string `json:"attemptAt"`
	Error       string `json:"error"`
	HasSnapshot bool   `json:"hasSnapshot"`
}

func (s *Store) FavoriteState(id int64) (FavoriteState, error) {
	var x FavoriteState
	err := s.db.QueryRow(`select updated_at,attempt_at,error,has_snapshot from favorite_state where account_id=?`, id).Scan(&x.UpdatedAt, &x.AttemptAt, &x.Error, &x.HasSnapshot)
	if err == sql.ErrNoRows {
		return x, nil
	}
	return x, err
}
func (s *Store) BeginFavorites(id int64) error {
	_, e := s.db.Exec(`insert into favorite_state(account_id,attempt_at) values(?,?) on conflict(account_id) do update set attempt_at=excluded.attempt_at`, id, now())
	return e
}
func (s *Store) SaveFavorites(id int64, items []*source.Comic, fetchErr error) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if fetchErr == nil {
		if _, e = tx.Exec(`delete from favorite_items where account_id=?`, id); e != nil {
			return e
		}
	}
	for _, c := range items {
		if c == nil || c.ComicID == "" {
			return fmt.Errorf("收藏条目缺少编号")
		}
		b, e := json.Marshal(c)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`insert into favorite_items(account_id,comic_id,title,author,data) values(?,?,?,?,?) on conflict(account_id,comic_id) do update set title=excluded.title,author=excluded.author,data=excluded.data`, id, c.ComicID, c.Title, c.Author, string(b)); e != nil {
			return e
		}
	}
	msg := ""
	if fetchErr != nil {
		msg = fetchErr.Error()
	}
	if fetchErr == nil {
		_, e = tx.Exec(`update favorite_state set updated_at=?,error='',has_snapshot=1 where account_id=?`, now(), id)
	} else {
		_, e = tx.Exec(`update favorite_state set error=? where account_id=?`, msg, id)
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) FavoritePage(id int64, keyword string, page, size int) ([]*source.Comic, int, error) {
	cond := ` where account_id=?`
	args := []any{id}
	if keyword != "" {
		cond += ` and (instr(lower(title),?)>0 or instr(lower(author),?)>0 or instr(comic_id,?)>0)`
		kw := strings.ToLower(keyword)
		args = append(args, kw, kw, kw)
	}
	var total int
	if e := s.db.QueryRow(`select count(*) from favorite_items`+cond, args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	args = append(args, size, (page-1)*size)
	rows, e := s.db.Query(`select data from favorite_items`+cond+` order by rowid limit ? offset ?`, args...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []*source.Comic{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, 0, e
		}
		var c source.Comic
		if e = json.Unmarshal([]byte(raw), &c); e != nil {
			return nil, 0, e
		}
		out = append(out, &c)
	}
	return out, total, rows.Err()
}
func (s *Store) InvalidateFavorites(id int64) error {
	_, e := s.db.Exec(`update favorite_state set updated_at='',attempt_at='' where account_id=?`, id)
	return e
}
