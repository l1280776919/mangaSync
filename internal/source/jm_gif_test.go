package source

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

// Regression for live comic 461647: the chapter API returns 00001.gif,
// 00002.gif, etc. GIFs must never enter the ten-band descrambler.
func TestJMGIFPreservesAnimationAndResume(t *testing.T) {
	for _, name := range []string{"00001.gif", "00002.gif", "00003.GIF"} {
		if got := jmSegNum(220980, 461647, name); got != 0 {
			t.Fatalf("%s: got %d segments", name, got)
		}
	}
	palette := color.Palette{color.Black, color.White}
	a := image.NewPaletted(image.Rect(0, 0, 12, 23), palette)
	b := image.NewPaletted(a.Bounds(), palette)
	b.SetColorIndex(3, 4, 1)
	var raw bytes.Buffer
	if err := gif.EncodeAll(&raw, &gif.GIF{Image: []*image.Paletted{a, b}, Delay: []int{5, 9}, LoopCount: 2}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "00001.gif")
	// Reproduce the old bug with a valid receipt: GIF decoded to one scrambled frame,
	// encoded as WebP, but still stored with a .gif suffix.
	old, _, err := jmEncodePage(raw.Bytes(), 10, "lossless")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(dst, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err = writeImageReceipt(dst, old); err != nil {
		t.Fatal(err)
	}
	if !IsLegacyJMGIF(dst) {
		t.Fatal("old scrambled file was not identified")
	}
	if _, ok := validImageFile(dst); ok {
		t.Fatal("old receipt incorrectly allowed resume skip")
	}
	if completeImageFiles(filepath.Dir(dst), 1) {
		t.Fatal("legacy chapter considered complete")
	}
	saved, err := jmSaveImage(raw.Bytes(), jmSegNum(220980, 461647, "00001.gif"), dst)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw.Bytes()) {
		t.Fatal("GIF changed during save")
	}
	if IsLegacyJMGIF(dst) {
		t.Fatal("valid GIF identified as legacy")
	}
	if _, ok := validImageFile(dst); !ok {
		t.Fatal("valid GIF cannot resume")
	}
	if !completeImageFiles(filepath.Dir(dst), 1) {
		t.Fatal("repaired chapter incomplete")
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Image) != 2 || decoded.Delay[1] != 9 || decoded.LoopCount != 2 {
		t.Fatal("animation lost")
	}
	fallback := filepath.Join(t.TempDir(), "00001.png")
	if err := os.WriteFile(fallback, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeImageReceipt(fallback, old); err != nil {
		t.Fatal(err)
	}
	if jmChapterImagesComplete(filepath.Dir(fallback), []string{"00001.gif"}) {
		t.Fatal("old fallback incorrectly considered complete")
	}
	if !jmChapterImagesComplete(filepath.Dir(dst), []string{"00001.gif"}) {
		t.Fatal("repaired GIF chapter not complete")
	}
	if got := jmSegNum(220980, 461647, "00001.webp"); got != 10 {
		t.Fatalf("non-GIF algorithm changed: %d", got)
	}
}
