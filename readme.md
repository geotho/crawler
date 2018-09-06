# Crawler

This is a web crawler.

## Usage

Install the web crawler with `go get -u github.com/geotho/crawler`.

If you've got `$GOPATH/bin` in your `$PATH`, you should be able to run `crawler`.

The binary takes two flag arguments (well three if you count `--help`):

1. `--workers` sets the maximum amount of concurrent workers (default 4).
2. `--root` specifies the starting domain (default geotho.github.io).

## Implementation

A crawler creates a fixed set of workers. Each worker reads URLs to parse from a shared channel (a work queue).
The channel is seeded with the root domain. The crawler holds the sitemap, which is implemented as a `map[string]map[string]struct{}` and is protected by a mutex. The sitemap is used to store the links and also as a guard against pages being enqueued or fetched more than once.

The crawler waits on a waitgroup before it can exit. The waitgroup is incremented whenever a URL is added to the channel, or whenever a URL is scheduled to be added to the channel.

Failures are categorised into retryable and non-retryable errors. Retriable errors (5XX or 429 HTTP status codes, or timeouts) are retried up to a configurable limit. Retries exponentially back off.

## Edgecases

The following edgecases are handled:

- Retryable errors (5XX, 429 status codes and timeouts)
- Redirects
- Not parsing non-HTML files
- Normalising URL fragments (e.g. example.com#anchor == example.com)
