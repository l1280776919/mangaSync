package source

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectReadOnlyAndRepairForcesReplacement(t *testing.T) {
	p := filepath.Join(t.TempDir(), "001.png")
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 3)))
	os.WriteFile(p, b.Bytes(), 0644)
	if err := InspectImage(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".sha256"); !os.IsNotExist(err) {
		t.Fatal("check wrote receipt")
	}
	writeImageReceipt(p, b.Bytes())
	os.WriteFile(p+".repair", []byte("repair"), 0600)
	if _, ok := validImageFile(p); ok {
		t.Fatal("marked page reused")
	}
	if err := writeImageReceipt(p, b.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, ok := validImageFile(p); !ok {
		t.Fatal("replacement unusable")
	}
	os.WriteFile(p+".sha256", []byte("mismatch"), 0644)
	if err := InspectImage(context.Background(), p); err == nil {
		t.Fatal("checksum mismatch ignored")
	}
	os.WriteFile(p, []byte("broken"), 0644)
	writeImageReceipt(p, []byte("broken"))
	if err := InspectImage(context.Background(), p); err == nil {
		t.Fatal("matching receipt bypassed decode")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// The context must be honored even if the decode slot was available.
	if err := InspectImage(ctx, p); err == nil {
		t.Fatal("canceled inspection succeeded")
	}
}
