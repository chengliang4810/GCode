package providerlogo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Fetch reads only the user-supplied website's origin. API paths, credentials,
// cookies and provider authorization headers are never sent.
func Fetch(ctx context.Context, baseURL string) (string, error) {
	origin, err := url.Parse(baseURL)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") ||
		origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return "", errors.New("enter a valid provider Base URL first")
	}
	origin.Path, origin.RawPath = "/", ""
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !sameOrigin(origin, req.URL) {
				return errors.New("website logo redirect leaves the provider website")
			}
			return nil
		},
	}
	get := func(target *url.URL, limit int64) ([]byte, *url.URL, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, nil, errors.New("invalid website logo URL")
		}
		req.Header.Set("Accept", "image/*,text/html;q=0.9,*/*;q=0.1")
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, errors.New("cannot reach the provider website")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, nil, errors.New("provider website returned an unsuccessful response")
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil || int64(len(data)) > limit {
			return nil, nil, errors.New("provider website response exceeds the size limit")
		}
		return data, resp.Request.URL, nil
	}
	candidates := []*url.URL{}
	if page, pageURL, err := get(origin, 512<<10); err == nil {
		if logo, err := normalize(page); err == nil {
			return logo, nil
		}
		parser := html.NewTokenizer(strings.NewReader(string(page)))
		for len(candidates) < 12 {
			kind := parser.Next()
			if kind == html.ErrorToken {
				break
			}
			if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
				continue
			}
			token := parser.Token()
			if token.Data != "link" {
				continue
			}
			var rel, href string
			for _, attr := range token.Attr {
				if attr.Key == "rel" {
					rel = strings.ToLower(attr.Val)
				}
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			icon := false
			for _, value := range strings.Fields(rel) {
				icon = icon || value == "icon" || value == "apple-touch-icon" || value == "apple-touch-icon-precomposed"
			}
			if !icon || href == "" {
				continue
			}
			ref, err := url.Parse(href)
			if err == nil {
				target := pageURL.ResolveReference(ref)
				if sameOrigin(origin, target) && target.User == nil {
					candidates = append(candidates, target)
				}
			}
		}
	}
	for _, path := range []string{"/favicon.ico", "/favicon.png", "/apple-touch-icon.png", "/favicon.svg"} {
		candidates = append(candidates, origin.ResolveReference(&url.URL{Path: path}))
	}
	seen := map[string]bool{}
	for _, target := range candidates {
		if ctx.Err() != nil {
			break
		}
		if seen[target.String()] {
			continue
		}
		seen[target.String()] = true
		data, _, err := get(target, MaxBytes)
		if err != nil {
			continue
		}
		if logo, err := normalize(data); err == nil {
			return logo, nil
		}
	}
	return "", errors.New("no usable website logo found; choose a built-in logo or retry")
}

func sameOrigin(origin, target *url.URL) bool {
	return target.User == nil && strings.EqualFold(origin.Host, target.Host) && strings.EqualFold(origin.Scheme, target.Scheme)
}
