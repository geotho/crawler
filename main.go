package main

import (
	"context"
	"net/url"
)

const (
	defaultWorkers = 4
)

func main() {
	crawler := &crawler{
		maxAttempts: 5,
		workers:     1,
		root: url.URL{
			Scheme: "https",
			Host:   "monzo.com",
		},
	}

	ctx := context.Background()

	crawler.crawl(ctx)
}
