package store

import (
	"encoding/json"
	"github.com/l1280776919/mangaSync/internal/source"
	"time"
)

func (s *Store) PrepareSyncItems(id int64, items []*source.Comic) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`delete from sync_items where account_id=?`, id); err != nil {
		return err
	}
	for _, c := range items {
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`insert into sync_items values(?,?,?,'pending','',?) on conflict(account_id,comic_id) do nothing`, id, c.ComicID, string(raw), now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) PendingSyncItems(id int64, failedOnly bool) ([]*source.Comic, error) {
	status := `status!='done'`
	if failedOnly {
		status = `status='failed'`
	}
	rows, err := s.db.Query(`select data from sync_items where account_id=? and `+status+` order by rowid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*source.Comic{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c source.Comic
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}
func (s *Store) FinishSyncItem(id int64, comic string, result error) error {
	status, msg := "done", ""
	if result != nil {
		status, msg = "failed", result.Error()
	}
	_, err := s.db.Exec(`update sync_items set status=?,error=?,updated_at=? where account_id=? and comic_id=?`, status, msg, now(), id, comic)
	return err
}
func (s *Store) InterruptedSyncAccounts() ([]int64, error) {
	rows, err := s.db.Query(`select distinct s.account_id from sync_items s join accounts a on a.id=s.account_id where s.status='pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (s *Store) ComicCheckFresh(kind, id string, chapters int, fingerprint string) bool {
	var c int
	var f, at string
	if s.db.QueryRow(`select chapters,fingerprint,checked_at from comic_checks where kind=? and comic_id=?`, kind, id).Scan(&c, &f, &at) != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, at)
	return err == nil && chapters > 0 && c == chapters && f == fingerprint && time.Since(t) < 6*time.Hour
}
func (s *Store) SaveComicCheck(kind, id string, chapters int, fingerprint string) error {
	_, err := s.db.Exec(`insert into comic_checks values(?,?,?,?,?) on conflict(kind,comic_id) do update set chapters=excluded.chapters,fingerprint=excluded.fingerprint,checked_at=excluded.checked_at`, kind, id, chapters, fingerprint, now())
	return err
}
