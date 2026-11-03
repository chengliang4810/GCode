package providerlogo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fyne-io/image/ico"
)

func logoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	img.Set(8, 8, color.NRGBA{R: 255, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestFetchProviderLogoUsesWebsiteOriginWithoutCredentials(t *testing.T) {
	pngData := logoPNG(t)
	var requests atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.RawQuery != "" {
			t.Error("website request carried credentials")
		}
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Set-Cookie", "private=website-cookie")
			_, _ = w.Write([]byte(`<html><link rel="shortcut icon" href="/assets/logo.png"></html>`))
		case "/assets/logo.png":
			_, _ = w.Write(pngData)
		default:
			t.Errorf("API path was requested: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	value, err := Fetch(t.Context(), site.URL+"/v1/custom/path")
	if err != nil || Validate(value) != nil || !strings.HasPrefix(value, "data:image/png;base64,") || requests.Load() != 2 {
		t.Fatalf("website icon discovery failed: value=%q requests=%d err=%v", value, requests.Load(), err)
	}
}

func TestFetchProviderLogoICOAndSVG(t *testing.T) {
	var encoded bytes.Buffer
	if err := ico.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, prefix string
		data               []byte
	}{
		{"ICO", "/favicon.ico", "data:image/png;", encoded.Bytes()},
		{"SVG", "/favicon.svg", "data:image/svg+xml;", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#f00" d="M0 0h24v24H0z"/></svg>`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(tc.data)
			}))
			defer site.Close()
			value, err := Fetch(t.Context(), site.URL+"/v1")
			if err != nil || !strings.HasPrefix(value, tc.prefix) || Validate(value) != nil {
				t.Fatalf("could not normalize %s: %v %q", tc.name, err, value)
			}
		})
	}
}

func TestFetchProviderLogoRestrictsReferencesAndCancellation(t *testing.T) {
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignRequests.Add(1)
		_, _ = w.Write(logoPNG(t))
	}))
	defer foreign.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<link rel="icon" href="` + foreign.URL + `/logo.png">`))
			return
		}
		http.Redirect(w, r, foreign.URL+"/logo.png", http.StatusFound)
	}))
	defer site.Close()
	if _, err := Fetch(t.Context(), site.URL); err == nil || foreignRequests.Load() != 0 {
		t.Fatal("cross-origin icon or redirect was fetched")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Fetch(ctx, site.URL); err == nil {
		t.Fatal("canceled logo request succeeded")
	}
	for _, endpoint := range []string{"file:///tmp/icon", "http://user:secret@example.com/v1", "https://example.com/v1?key=secret"} {
		if _, err := Fetch(t.Context(), endpoint); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid or credential-bearing URL was accepted or exposed")
		}
	}
}

func TestProviderLogoRejectsUnsafeAndOversizedImages(t *testing.T) {
	hugeICO := make([]byte, 22)
	copy(hugeICO, []byte{0, 0, 1, 0, 1, 0})
	binary.LittleEndian.PutUint32(hugeICO[14:18], ^uint32(0))
	for _, data := range [][]byte{
		hugeICO, bytes.Repeat([]byte("x"), MaxBytes+1),
		[]byte(`<svg><script>alert(1)</script></svg>`),
		[]byte(`<svg><image href="https://example.com/a.png"/></svg>`),
		[]byte(`<svg><use href="file:///etc/passwd"/></svg>`),
		[]byte(`<!DOCTYPE svg SYSTEM "file:///etc/passwd"><svg/>`),
		[]byte(`<html>not a logo</html>`),
	} {
		if _, err := normalize(data); err == nil {
			t.Fatalf("accepted invalid logo: %.80q", data)
		}
	}
	if err := Validate("not-a-brand"); err == nil {
		t.Fatal("unknown built-in brand was accepted")
	}
	corrupt := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("invalid"))
	if err := Validate(corrupt); err == nil {
		t.Fatal("invalid persisted image was accepted")
	}
}
