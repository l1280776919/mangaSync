package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type picaAuthTransport func(*http.Request) (*http.Response, error)

func (f picaAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func picaAuthResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestPicaLoginRejectsInvalidTokenResponses(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"missing data", `{"code":200,"message":"success"}`, "缺少 data"},
		{"null data", `{"code":200,"data":null}`, "缺少 data"},
		{"missing token", `{"code":200,"data":{}}`, "data.token"},
		{"empty token", `{"code":200,"data":{"token":""}}`, "data.token"},
		{"blank token", `{"code":200,"data":{"token":"  "}}`, "data.token"},
		{"null token", `{"code":200,"data":{"token":null}}`, "data.token"},
		{"wrong token type", `{"code":200,"data":{"token":42}}`, "data.token"},
		{"invalid data type", `{"code":200,"data":[]}`, "data.token"},
		{"broken JSON", `{"code":200,"token":"private-value"`, "不是有效 JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			p := NewPica("", "", 1)
			p.api.Transport = picaAuthTransport(func(r *http.Request) (*http.Response, error) { calls++; return picaAuthResponse(200, tc.body), nil })
			info, err := p.Login(context.Background(), "test-user", "test-password")
			if info != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("info=%v error=%v", info, err)
			}
			if calls != 1 {
				t.Fatalf("invalid login made %d requests", calls)
			}
			if strings.Contains(err.Error(), "private-value") {
				t.Fatal("error leaked response contents")
			}
		})
	}
}

func TestPicaLoginProfileValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{name: "valid profile", status: 200, body: `{"code":200,"data":{"user":{"name":"test","level":2}}}`},
		{name: "HTTP unauthorized", status: 401, body: `{}`, wantErr: ErrAuth},
		{name: "envelope unauthorized", status: 200, body: `{"code":401}`, wantErr: ErrAuth},
		{name: "expired token", status: 200, body: `{"code":400,"error":"1005"}`, wantErr: ErrAuth},
		{name: "optional profile unavailable", status: 400, body: `{"code":400,"message":"unavailable"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPica("", "", 1)
			p.api.Transport = picaAuthTransport(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/auth/sign-in":
					return picaAuthResponse(200, `{"code":200,"data":{"token":"test-token"}}`), nil
				case "/users/profile":
					if r.Header.Get("authorization") != "test-token" {
						t.Fatal("profile missing token")
					}
					return picaAuthResponse(tc.status, tc.body), nil
				case "/users/favourite":
					return picaAuthResponse(200, `{"code":200,"data":{"comics":{"total":0,"pages":1,"docs":[]}}}`), nil
				default:
					t.Fatalf("unexpected request %s", r.URL.Path)
					return nil, nil
				}
			})
			info, err := p.Login(context.Background(), "test-user", "test-password")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || info != nil {
					t.Fatalf("info=%v err=%v", info, err)
				}
				return
			}
			if err != nil || info == nil || info.Token != "test-token" {
				t.Fatalf("info=%v err=%v", info, err)
			}
		})
	}
}

type picaBrokenBody struct{}

func (picaBrokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (picaBrokenBody) Close() error             { return nil }
func TestPicaRejectsTruncatedResponse(t *testing.T) {
	p := NewPica("", "", 1)
	p.api.Transport = picaAuthTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: picaBrokenBody{}}, nil
	})
	_, err := p.Login(context.Background(), "test-user", "test-password")
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "读取哔咔响应") {
		t.Fatalf("error=%v", err)
	}
}

func TestPicaLoginDoesNotMaskCanceledProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := NewPica("", "", 1)
	p.api.Transport = picaAuthTransport(func(r *http.Request) (*http.Response, error) {
		cancel()
		return picaAuthResponse(200, `{"code":200,"data":{"token":"test-token"}}`), nil
	})
	info, err := p.Login(ctx, "test-user", "test-password")
	if info != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("info=%v err=%v", info, err)
	}
}
