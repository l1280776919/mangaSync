package source

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"testing"
)

func TestJMJPEGKeepsFullRangeColors(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 32))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{80, 110, 150, 255}), image.Point{}, draw.Src)
	var raw bytes.Buffer
	if err := jpeg.Encode(&raw, src, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(raw.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	want := image.NewRGBA(src.Bounds())
	draw.Draw(want, want.Bounds(), decoded, image.Point{}, draw.Src)
	unchanged, ext, err := jmEncodePage(raw.Bytes(), 0, "lossless")
	if err != nil || ext != "jpg" || !bytes.Equal(unchanged, raw.Bytes()) {
		t.Fatal("unmodified JPEG mislabeled or changed")
	}
	b, _, err := jmEncodePage(raw.Bytes(), 2, "lossless")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 16; x++ {
			// Two equal-height bands are reversed. Compare against the standard JPEG conversion.
			r, g, b, _ := got.At(x, y).RGBA()
			wr, wg, wb, _ := want.At(x, (y+16)%32).RGBA()
			if r != wr || g != wg || b != wb {
				t.Fatalf("JPEG color changed at %d,%d: %d %d %d vs %d %d %d", x, y, r, g, b, wr, wg, wb)
			}
		}
	}
}
func TestJMWebPConversionRemainsLimitedRange(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 2, 2), image.YCbCrSubsampleRatio444)
	for i := range src.Y {
		src.Y[i] = 16
		src.Cb[i] = 128
		src.Cr[i] = 128
	}
	if got := jmWebPToRGBA(src).RGBAAt(0, 0); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("WebP black changed: %v", got)
	}
	if got := jmToRGBA(src).RGBAAt(0, 0); got != (color.RGBA{16, 16, 16, 255}) {
		t.Fatalf("JPEG black changed: %v", got)
	}
}
func TestJMSegNumUsesWholeStem(t *testing.T) {
	// Hash the full stem, matching upstream last-dot filename extraction.
	sum := md5Hex("46164700001.extra")
	want := int(sum[len(sum)-1])%8*2 + 2
	if got := jmSegNum(220980, 461647, "00001.extra.webp"); got != want {
		t.Fatalf("got %d want %d", got, want)
	}
}
