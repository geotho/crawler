package main

import (
	"context"
	"fmt"
	"net/url"
)

const (
	defaultWorkers = 4
)

func main() {
	crawler := &crawler{
		maxAttempts: 5,
		workers:     4,
		root: url.URL{
			Scheme: "https",
			Host:   "bluevisionlabs.com",
		},
	}

	ctx := context.Background()

	sitemap := crawler.crawl(ctx)

	for page, links := range sitemap {
		fmt.Printf("%s:\n", page)
		for link := range links {
			fmt.Printf("\t%s\n", link)
		}
		fmt.Println()
	}
}
