package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func tinyImage() []byte {
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.Black)
	var b bytes.Buffer
	png.Encode(&b, im)
	return b.Bytes()
}
func TestPicaRejectsMissingImagePages(t *testing.T) {
	for _, bad := range []string{`{"code":200,"data":{"pages":{"total":2,"pages":2,"docs":"broken"}}}`, `{"code":200,"data":{"pages":{"total":2,"pages":2,"docs":[{"media":{}}]}}}`, `{"code":200,"data":{"pages":{"total":2,"pages":2,"docs":[]}}}`} {
		p := NewPica("", "", 1)
		p.api.Transport = picaAuthTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Query().Get("page") == "1" {
				return picaAuthResponse(200, `{"code":200,"data":{"pages":{"total":2,"pages":2,"docs":[{"media":{"fileServer":"https://example.invalid","path":"first.png"}}]}}}`), nil
			}
			return picaAuthResponse(200, bad), nil
		})
		if _, err := p.chapterImageURLs(context.Background(), &Cred{}, "book", 1); err == nil {
			t.Fatal("accepted incomplete pages")
		}
	}
}
func TestPicaResumeOnlyFetchesMissingOrCorruptImages(t *testing.T) {
	dir := t.TempDir()
	data := tinyImage()
	os.WriteFile(filepath.Join(dir, "001.png"), data, 0644)
	urls := []string{"https://example.invalid/1.png", "https://example.invalid/2.png"}
	p := NewPica("", "", 1)
	calls := 0
	p.img.Transport = picaAuthTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	n, _, err := p.downloadImages(context.Background(), urls, dir, Hooks{}, Progress{})
	if err != nil || n != 2 || calls != 1 {
		t.Fatalf("n=%d calls=%d err=%v", n, calls, err)
	}
	os.WriteFile(filepath.Join(dir, "002.png"), []byte("corrupted"), 0644)
	n, _, err = p.downloadImages(context.Background(), urls, dir, Hooks{}, Progress{})
	if err != nil || n != 2 || calls != 2 {
		t.Fatalf("corrupt file not repaired: calls=%d err=%v", calls, err)
	}
}
func TestResumeManifestInvalidatesReorderedPages(t *testing.T) {
	dir := t.TempDir()
	preparePageManifest(dir, []string{"a", "b"})
	os.WriteFile(filepath.Join(dir, "001.png"), tinyImage(), 0644)
	if err := preparePageManifest(dir, []string{"b", "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "001.png")); !os.IsNotExist(err) {
		t.Fatal("stale page survived changed manifest")
	}
}
func TestComicSnapshotRoundtrip(t *testing.T) {
	for _, c := range []Comic{{ComicID: "a", ChaptersCount: 3}, {ComicID: "b", Chapters: []Chapter{{Order: 1}}}} {
		raw, _ := json.Marshal(c)
		var got Comic
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got.Chapters) != fmt.Sprint(c.Chapters) || got.ChaptersCount != c.ChaptersCount {
			t.Fatalf("snapshot mismatch: %+v", got)
		}
	}
}
