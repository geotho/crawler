package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"
)

const (
	defaultWorkers = 4
	defaultRoot    = "https://geotho.github.io"
)

var (
	workersFlag = flag.Int("workers", defaultWorkers, "specify the number of workers")
	rootFlag    = flag.String("root", defaultRoot, "root domain to crawl")
)

func init() {
	flag.Parse()
}

func main() {
	err := run()
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	root := *rootFlag
	u, err := url.Parse(root)
	if err != nil {
		return fmt.Errorf("error parsing <root-domain>: %s", err)
	}

	crawler := &crawler{
		maxAttempts: 5,
		workers:     *workersFlag,
		root:        *u,
	}

	ctx := context.Background()

	start := time.Now()
	sitemap := crawler.crawl(ctx)
	elapsed := time.Since(start)

	totalLinks := 0

	for page, links := range sitemap {
		totalLinks += len(links)
		fmt.Printf("%s (%d):\n", page, len(links))
		for link := range links {
			fmt.Printf("\t%s\n", link)
		}
		fmt.Println()
	}

	fmt.Println("Done! ✅")

	fmt.Printf("Crawled %d pages on %s in %v. 🎉 \nFound %d links (average %0.3f links per page).\n", len(sitemap), root, elapsed, totalLinks, float64(totalLinks)/float64(len(sitemap)))

	return nil
}
