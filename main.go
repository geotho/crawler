package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
)

const (
	defaultWorkers = 4
)

func main() {
	err := run()
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
}

func run() error {

	args := os.Args[1:]
	if len(args) != 1 {
		return fmt.Errorf("crawler accepts exactly one argument <root-domain>")
	}

	root := args[0]
	u, err := url.Parse(root)
	if err != nil {
		return fmt.Errorf("error parsing <root-domain>: %s", err)
	}

	crawler := &crawler{
		maxAttempts: 5,
		workers:     defaultWorkers,
		root:        *u,
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

	return nil
}
