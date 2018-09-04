package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCrawler(t *testing.T) {
	mux := http.NewServeMux()

	srv := httptest.NewServer(mux)
	defer srv.Close()

	expectedSiteMap := generateSiteMap(srv.URL, 100, 0.2)

	handler := generateHandler(srv.URL, expectedSiteMap)
	mux.Handle("/", handler)

	root, err := url.Parse(srv.URL)
	if err != nil {
		t.Errorf("could not parse URL %s: %s", srv.URL, err)
		return
	}

	crawler := crawler{
		maxAttempts: 1,
		workers:     4,
		root:        *root,
	}

	actualSiteMap := crawler.crawl(context.Background())
	assertSiteMapEqual(t, expectedSiteMap, actualSiteMap)
}

func generateSiteMap(root string, size int, density float64) siteMap {
	s := make(siteMap, size)
	indexPage := make(map[string]struct{}, size)

	for i := 0; i < size; i++ {
		pageURL := fmt.Sprintf("%s/%d", root, i)
		indexPage[pageURL] = struct{}{}
		s[pageURL] = make(map[string]struct{}, int(float64(size)*density))

		for j := 0; j < size; j++ {
			if rand.Float64() < density {
				linkURL := fmt.Sprintf("%s/%d", root, j)
				s[pageURL][linkURL] = struct{}{}
			}
		}
	}

	s[root] = indexPage

	return s
}

func generateHandler(root string, s siteMap) *http.ServeMux {
	mux := http.NewServeMux()

	for pageURL, links := range s {
		pageURL = pageURL[len(root):]
		if pageURL == "" {
			pageURL = "/"
		}

		pageBody := strings.Builder{}
		pageBody.WriteString("<html><body>")

		for link := range links {
			pageBody.WriteString(fmt.Sprintf("<a href='%s'>%s</a>\n", link, link))
		}

		pageBody.WriteString("</body></html>")
		mux.Handle(pageURL, htmlHander(pageBody.String()))
	}

	return mux
}

func assertSiteMapEqual(t *testing.T, expected, actual siteMap) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("siteMaps differ in length. len(expected)=%d len(actual)=%d", len(expected), len(actual))
	}

	for k, v := range expected {
		assertMapEqual(t, v, actual[k], k)
	}
}

func assertMapEqual(t *testing.T, expected, actual map[string]struct{}, key string) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("maps differ in length. len(expected)=%d len(actual)=%d key=%s", len(expected), len(actual), key)
	}

	for k := range expected {
		_, ok := actual[k]
		if !ok {
			t.Errorf("key %s not present in actual", k)
		}
	}
}

func htmlHander(body string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	})
}
