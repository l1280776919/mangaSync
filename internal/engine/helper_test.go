package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/l1280776919/mangaSync/internal/config"
)

func testCfg(t *testing.T, proxy string) *config.Manager {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"),
		[]byte(`{"picaProxy":"`+proxy+`","downloadRoot":"`+dir+`","picaDir":"p","jmDir":"j"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
