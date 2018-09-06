package main

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"text/template"

	"github.com/kr/pretty"
)

const html = `
<html>
	<body>
	{{- range $key, $value := . }}
		<a href="{{ $key }}"></a>
	{{- end}}
	</body>
</html>
`

var tmpl = template.Must(template.New("html").Parse(html))

func BenchmarkCrawler(b *testing.B) {
	b.ReportAllocs()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("testdata")))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root, err := url.Parse(srv.URL)
	if err != nil {
		b.Errorf("could not parse URL %s: %s", srv.URL, err)
		return
	}

	crawler := crawler{
		maxAttempts: 1,
		workers:     4,
		root:        *root,
	}

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sm := crawler.crawl(ctx)
		_ = sm
	}
}

func TestCrawlerTestRedirects(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	expectedSiteMap := map[string]map[string]struct{}{
		srv.URL: map[string]struct{}{
			srv.URL + "/a": struct{}{},
		},
		srv.URL + "/": map[string]struct{}{
			srv.URL + "/a": struct{}{},
		},
		srv.URL + "/a": map[string]struct{}{},
		srv.URL + "/b": map[string]struct{}{
			srv.URL + "/": struct{}{},
		},
	}

	mux.Handle("/a", http.RedirectHandler("/b", http.StatusPermanentRedirect))
	mux.Handle("/b", htmlHandler("<a href='/'></a>"))
	mux.Handle("/", htmlHandler("<a href='/a'></a>"))

	root, err := url.Parse(srv.URL)
	if err != nil {
		t.Errorf("could not parse URL %s: %s", srv.URL, err)
		return
	}

	crawler := crawler{
		maxAttempts: 1,
		workers:     1,
		root:        *root,
	}

	actualSiteMap := crawler.crawl(context.Background())
	assertSiteMapEqual(t, expectedSiteMap, actualSiteMap)
}
func TestCrawlerRetryTransientErrors(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	const maxAttempts = 5

	expectedSiteMap := map[string]map[string]struct{}{
		srv.URL + "/retry-transient": map[string]struct{}{
			srv.URL + "/": struct{}{},
		},
		srv.URL + "/retry-fail": map[string]struct{}{},
		srv.URL: map[string]struct{}{
			srv.URL + "/retry-transient": struct{}{},
			srv.URL + "/retry-fail":      struct{}{},
		},
		srv.URL + "/": map[string]struct{}{
			srv.URL + "/retry-transient": struct{}{},
			srv.URL + "/retry-fail":      struct{}{},
		},
	}

	calls := 0
	mtx := sync.Mutex{}

	mux.Handle("/retry-transient", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mtx.Lock()
		defer mtx.Unlock()
		if calls < maxAttempts-1 {
			calls++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Write([]byte("<a href='/'></a>"))
	}))

	mux.Handle("/retry-fail", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	mux.Handle("/", htmlHandler("<a href='/retry-transient'></a><a href='/retry-fail'></a>"))

	root, err := url.Parse(srv.URL)
	if err != nil {
		t.Errorf("could not parse URL %s: %s", srv.URL, err)
		return
	}

	crawler := crawler{
		maxAttempts: maxAttempts,
		workers:     1,
		root:        *root,
	}

	actualSiteMap := crawler.crawl(context.Background())
	assertSiteMapEqual(t, expectedSiteMap, actualSiteMap)
}

func TestCrawlerRandomSites(t *testing.T) {
	type testcase struct {
		name        string
		workers     int
		siteMapSize int
		linkDensity float64
	}

	testcases := []testcase{
		testcase{
			name:        "siteNoPagesNoLinks",
			siteMapSize: 0,
			linkDensity: 0,
			workers:     1,
		},
		testcase{
			name:        "smallSite",
			siteMapSize: 2,
			linkDensity: 2,
			workers:     1,
		},
		testcase{
			name:        "bigSiteSparseLinksMultipleWorkers",
			siteMapSize: 1,
			linkDensity: 0.1,
			workers:     4,
		},
		testcase{
			name:        "veryBigSiteDenseLinksMultipleWorkers",
			siteMapSize: 1000,
			linkDensity: 1,
			workers:     4,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			srv := httptest.NewServer(mux)
			defer srv.Close()

			expectedSiteMap := generateSiteMap(srv.URL, tt.siteMapSize, tt.linkDensity)

			handler := generateHandler(srv.URL, expectedSiteMap)
			mux.Handle("/", handler)

			root, err := url.Parse(srv.URL)
			if err != nil {
				t.Errorf("could not parse URL %s: %s", srv.URL, err)
				return
			}

			crawler := crawler{
				maxAttempts: 1,
				workers:     tt.workers,
				root:        *root,
			}

			actualSiteMap := crawler.crawl(context.Background())
			assertSiteMapEqual(t, expectedSiteMap, actualSiteMap)
		})
	}
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

		b := &bytes.Buffer{}
		tmpl.Execute(b, links)

		mux.Handle(pageURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// Have to create and store the page beforehand because we can't concurrently iterate the map.
			w.Write(b.Bytes())
		}))
	}

	return mux
}

func assertSiteMapEqual(t *testing.T, expected, actual siteMap) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("siteMaps differ in length. len(expected)=%d len(actual)=%d", len(expected), len(actual))
		pretty.Print(actual)
		pretty.Print(expected)
		// t.Logf("%+v", actual)
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

func htmlHandler(body string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	})
}
