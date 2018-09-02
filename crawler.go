package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/geotho/crawler/parse"
)

// siteMap is an adjacency list of links in the site.
// an entry siteMap[a][b] means a links to b.
type siteMap map[string]map[string]struct{}

type crawler struct {
	workers     int
	root        string
	maxAttempts int
	sitemap     siteMap
	sitemapMtx  sync.Mutex
}

type crawlReq struct {
	url      string
	attempts int
}

type crawlResult struct {
	finalURL string
	links    map[string]struct{}
}

type crawlerError struct {
	err       error
	retryable bool
}

func (e *crawlerError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (c *crawler) crawl(ctx context.Context) {

	urls := make(chan string, 100)

	for i := 0; i < c.workers; i++ {
		go c.workerCrawl(ctx, urls)
	}
}

func (c *crawler) workerCrawl(ctx context.Context, toCrawl <-chan string) {
	for {
		select {
		case url := <-toCrawl:
			// res, err := c.getURL(ctx, url)
		case <-ctx.Done():
			return
		}
	}

}

func (c *crawler) getURL(ctx context.Context, url string) (*crawlResult, *crawlerError) {
	cli := http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, &crawlerError{
			err: fmt.Errorf("could not make request for %s: %v", url, err),
		}
	}
	req = req.WithContext(ctx)

	resp, err := cli.Do(req)
	if err != nil {
		return nil, &crawlerError{
			err:       fmt.Errorf("could not get %s: %v", url, err),
			retryable: true,
		}
	}

	// finalURL is the URL we end up at after potentially following redirects.
	finalURL := resp.Request.URL.String()

	links, err := parse.HTML(resp.Body)
	defer resp.Body.Close() // want to make sure the body is closed regardless.
	if err != nil {
		return nil, &crawlerError{
			err:       fmt.Errorf("could not get %s: %v", url, err),
			retryable: true,
		}
	}

	urls := normaliseURLs(c.root, finalURL, links)

	var cErr *crawlerError

	switch {
	case 500 <= resp.StatusCode:
		// Handle the case where we get redirected to foo.com/500.php. This page
		// still could link to things and we should handle it as such.
		cErr = &crawlerError{
			err:       fmt.Errorf("got a %s response code for %s", resp.Status, url),
			retryable: true,
		}
	case 400 <= resp.StatusCode && resp.StatusCode < 500:
		cErr = &crawlerError{
			err:       fmt.Errorf("got a %s response code for %s", resp.Status, url),
			retryable: false,
		}
	}

	crawlResult := &crawlResult{
		finalURL: finalURL,
		links:    urls,
	}

	return crawlResult, cErr

}

// normaliseURLs normalises a list of URLs relative to a base. It also filters out
// URLs that do not belong to the rootDomain, and de-dupes urls that only differ in fragment.
func normaliseURLs(rootDomain, sourcePage string, urls []string) map[string]struct{} {
	normalised := make(map[string]struct{}, len(urls)/2)

	base, err := url.Parse(sourcePage)
	if err != nil {
		return normalised
	}

	for _, u := range urls {
		uu, err := url.Parse(u)
		if err != nil {
			continue
		}

		full := base.ResolveReference(uu)
		if full.Host != rootDomain {
			continue
		}
		full.Fragment = ""

		normalised[full.String()] = struct{}{}
	}

	return normalised
}
