package source

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// StoredPageCount reads the last downloaded manifest, not the number of files.
// It is a local snapshot; it cannot discover new remote pages.
func StoredPageCount(dir, kind string) int {
	f, err := os.Open(filepath.Join(dir, ".pages.json"))
	if err != nil {
		return 0
	}
	defer f.Close()
	var ids []string
	if json.NewDecoder(io.LimitReader(f, 4<<20)).Decode(&ids) != nil {
		return 0
	}
	offset := 1
	if kind == "pica" {
		offset = 2
	}
	n := len(ids) - offset
	if n < 1 || n > 5000 {
		return 0
	}
	return n
}

// InspectImage is read-only, including when no checksum receipt exists.
// Full decode detects truncation even when a file has a matching old receipt.
func InspectImage(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case imageProcessingSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-imageProcessingSlots }()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("图片头损坏: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
		return fmt.Errorf("图片尺寸超出校验上限")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return err
	}
	if _, _, err = image.Decode(f); err != nil {
		return fmt.Errorf("图片解码失败: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	receipt, err := os.ReadFile(path + ".sha256")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if strings.TrimSpace(string(receipt)) != fmt.Sprintf("%x", h.Sum(nil)) {
		return fmt.Errorf("图片校验值不匹配")
	}
	return nil
}
