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
		c.sitemap = make(map[string]map[string]struct{})
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
			res, err := c.getURL(ctx, req.url)
			if err != nil && err.retryable && req.attempts < c.maxAttempts {
				c.workerWg.Add(1)
				time.AfterFunc((2<<uint(req.attempts))*10*time.Millisecond, func() {
					toCrawl <- crawlReq{
						url:      req.url,
						attempts: req.attempts + 1,
					}
				})
			}

			if res == nil {
				c.workerWg.Done()
				continue
			}

			c.sitemapMtx.Lock()

			for k := range res.links {
				_, ok := c.sitemap[k]
				if !ok {
					fmt.Printf("adding %s\n", k)
					c.sitemap[k] = make(map[string]struct{})
					c.workerWg.Add(1)
					toCrawl <- crawlReq{url: k}
				}
			}

			c.sitemap[res.finalURL] = res.links
			c.sitemapMtx.Unlock()

			c.workerWg.Done()

		case <-ctx.Done():
			return
		}
	}

}

func (c *crawler) getURL(ctx context.Context, url string) (*crawlResult, *crawlerError) {
	fmt.Printf("attempting to get %s\n", url)

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

	urls := normaliseURLs(c.root.Host, finalURL, links)

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
