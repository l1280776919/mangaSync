package store

import (
	"errors"
	"github.com/l1280776919/mangaSync/internal/source"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultAndBackupRestore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateAccount(&Account{Kind: "pica", Username: "test", Password: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveLoginOK(id, "test-token", "", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	var stored string
	s.db.QueryRow(`select password from accounts where id=?`, id).Scan(&stored)
	if !strings.HasPrefix(stored, secretPrefix) || strings.Contains(stored, "test-secret") {
		t.Fatal("credential not encrypted")
	}
	got, err := s.GetAccount(id)
	if err != nil || got.Password != "test-secret" || got.Token != "test-token" {
		t.Fatalf("roundtrip: %v", err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if err = Backup(dir, backup); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(backup, "secrets.key")); !os.IsNotExist(err) {
		t.Fatal("key included in backup")
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if err = RestoreBackup(backup, filepath.Join(dir, "secrets.key"), dest); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	a, err := restored.GetAccount(id)
	if err != nil || a.Password != "test-secret" {
		t.Fatal("restore lost encrypted credentials")
	}
	if err = RestoreBackup(backup, filepath.Join(dir, "secrets.key"), dest); err == nil {
		t.Fatal("overwrote existing destination")
	}
	wrong := filepath.Join(t.TempDir(), "wrong-key")
	os.WriteFile(wrong, make([]byte, 32), 0600)
	if VerifyBackup(backup, wrong) == nil {
		t.Fatal("accepted wrong key")
	}
	os.WriteFile(filepath.Join(backup, "mangasync.db"), []byte("corrupt"), 0600)
	if VerifyBackup(backup, filepath.Join(dir, "secrets.key")) == nil {
		t.Fatal("accepted damaged backup")
	}
	s.db.Close()
	os.Remove(filepath.Join(dir, "secrets.key"))
	if _, err = Open(dir); err == nil {
		t.Fatal("silently regenerated lost key")
	}
}
func TestPlaintextCredentialMigration(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.db.Exec(`insert into accounts(kind,username,password,token,created_at) values('jm','legacy','legacy-password','legacy-token','2026-01-01T00:00:00Z')`)
	s.db.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.GetAccount(1)
	if err != nil || a.Password != "legacy-password" || a.Token != "legacy-token" {
		t.Fatalf("migration error: %v", err)
	}
	var raw string
	s.db.QueryRow(`select password from accounts`).Scan(&raw)
	if !strings.HasPrefix(raw, secretPrefix) {
		t.Fatal("legacy password not encrypted")
	}
}
func TestSnapshotsPreserveGoodDataOnPartialRefresh(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.BeginFavorites(1)
	s.SaveFavorites(1, []*source.Comic{{ComicID: "a", Title: "Alpha", ChaptersCount: 2}}, nil)
	s.BeginFavorites(1)
	s.SaveFavorites(1, []*source.Comic{{ComicID: "b", Title: "Beta"}}, errors.New("page unavailable"))
	items, total, err := s.FavoritePage(1, "", 1, 20)
	if err != nil || total != 2 || items[0].ChaptersCount != 2 {
		t.Fatalf("snapshot lost data: %d %v", total, err)
	}
	state, _ := s.FavoriteState(1)
	if !state.HasSnapshot || state.Error == "" {
		t.Fatal("partial refresh not visible")
	}
	s.BeginFavorites(1)
	s.SaveFavorites(1, []*source.Comic{}, nil)
	_, total, _ = s.FavoritePage(1, "", 1, 20)
	if total != 0 {
		t.Fatal("authoritative empty snapshot ignored")
	}
}
func TestCheckpointPersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.PrepareSyncItems(1, []*source.Comic{{ComicID: "done"}, {ComicID: "pending"}, {ComicID: "failed"}})
	s.FinishSyncItem(1, "done", nil)
	s.FinishSyncItem(1, "failed", errors.New("unavailable"))
	s.db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingSyncItems(1, false)
	if err != nil || len(pending) != 2 {
		t.Fatalf("resume: %v", err)
	}
	failed, err := s.PendingSyncItems(1, true)
	if err != nil || len(failed) != 1 || failed[0].ComicID != "failed" {
		t.Fatalf("retry: %v", err)
	}
}

func TestLegacyBackupWithoutKey(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`insert into accounts(kind,username,password,token,created_at) values('jm','legacy','legacy-password','legacy-token','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	s.db.Close()
	if err = os.Remove(filepath.Join(dir, "secrets.key")); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if err = Backup(dir, backup); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if err = RestoreBackup(backup, "", dest); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.db.Close()
	a, err := restored.GetAccount(1)
	if err != nil || a.Password != "legacy-password" {
		t.Fatalf("legacy restore failed: %v", err)
	}
}
