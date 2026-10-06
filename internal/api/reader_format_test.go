package api

import (
	"bytes"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeCachedFileUsesActualImageFormat(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "00001.jpg")
	if err := os.WriteFile(p, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/page/1", nil)
	serveCachedFile(w, r, p, fi)
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("wrong MIME: %s", got)
	}
	if !bytes.Equal(w.Body.Bytes(), data.Bytes()) {
		t.Fatal("image bytes changed")
	}
	r = httptest.NewRequest("GET", "/page/1", nil)
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	serveCachedFile(w, r, p, fi)
	if w.Code != 304 {
		t.Fatalf("conditional caching broken: %d", w.Code)
	}
}
