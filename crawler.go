package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/geotho/crawler/parse"
)

// siteMap is an adjacency list of links in the site.
// an entry siteMap[a][b] means a links to b.
type siteMap map[string]map[string]struct{}

type crawler struct {
	workers     int
	root        url.URL
	maxAttempts int
	sitemap     siteMap
	sitemapMtx  sync.Mutex
	workerWg    sync.WaitGroup
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

func (c *crawler) crawl(ctx context.Context) siteMap {
	if c.sitemap == nil {
		c.sitemap = make(siteMap)
	}

	ctx, cancel := context.WithCancel(ctx)
	urls := make(chan crawlReq, 1000)

	for i := 0; i < c.workers; i++ {
		go c.workerCrawl(ctx, urls)
	}

	c.workerWg.Add(1)
	urls <- crawlReq{
		url:      c.root.String(),
		attempts: 0,
	}

	c.workerWg.Wait()
	cancel()

	return c.sitemap

}

func (c *crawler) workerCrawl(ctx context.Context, toCrawl chan crawlReq) {
	for {
		select {
		case req := <-toCrawl:
			c.doCrawlReq(ctx, req, toCrawl)
		case <-ctx.Done():
			return
		}
	}
}

func (c *crawler) doCrawlReq(ctx context.Context, req crawlReq, newWork chan<- crawlReq) {
	defer c.workerWg.Done()

	fmt.Printf("fetching %s\n", req.url)
	res, err := c.getURL(ctx, req.url)
	if err != nil && err.retryable && req.attempts < c.maxAttempts {
		c.workerWg.Add(1)
		time.AfterFunc((2<<uint(req.attempts))*10*time.Millisecond, func() {
			newWork <- crawlReq{
				url:      req.url,
				attempts: req.attempts + 1,
			}
		})
	}

	if res == nil {
		return
	}

	c.sitemapMtx.Lock()
	defer c.sitemapMtx.Unlock()

	for k := range res.links {
		_, ok := c.sitemap[k]
		if !ok {
			fmt.Printf("found %s\n", k)
			c.sitemap[k] = make(map[string]struct{})
			c.workerWg.Add(1)
			newWork <- crawlReq{url: k}
		}
	}

	c.sitemap[res.finalURL] = res.links
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

	// Let's not read the body if what we have is definitely not HTML.
	if !shouldReadBody(resp.Header) {
		return &crawlResult{
			finalURL: finalURL,
		}, nil
	}

	links, err := parse.HTML(resp.Body)
	defer resp.Body.Close() // want to make sure the body is closed regardless.
	if err != nil {
		return nil, &crawlerError{
			err:       fmt.Errorf("could not get %s: %v", url, err),
			retryable: true,
		}
	}

	urls := normaliseURLs(c.root.Host, finalURL, links)

	cErr := makeCrawlerError(resp.StatusCode)

	crawlResult := &crawlResult{
		finalURL: finalURL,
		links:    urls,
	}

	return crawlResult, cErr

}

// normaliseURLs normalises a list of URLs relative to a sourcePage. It also filters out
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

func shouldReadBody(header http.Header) bool {
	contentType := header.Get("Content-Type")

	if contentType == "" {
		return true
	}

	if strings.Contains(contentType, "text/html") {
		return true
	}

	return false
}

func makeCrawlerError(statusCode int) *crawlerError {
	var cErr *crawlerError

	switch {
	case 500 <= statusCode, statusCode == http.StatusTooManyRequests, statusCode == http.StatusRequestTimeout:
		cErr = &crawlerError{
			err:       fmt.Errorf("got a %d response code", statusCode),
			retryable: true,
		}
	case 400 <= statusCode && statusCode < 500:
		cErr = &crawlerError{
			err:       fmt.Errorf("got a %d response code", statusCode),
			retryable: false,
		}
	}

	return cErr
}
