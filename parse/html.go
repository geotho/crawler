package parse

import (
	"io"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// HTML parses a HTML file, returning all the links it finds. If the reader returns
// invalid HTML, an error is returned.
func HTML(r io.Reader) ([]string, error) {
	var links []string

	z := html.NewTokenizer(r)
	for {
		tokenType := z.Next()

		if tokenType == html.ErrorToken {
			if z.Err() == io.EOF {
				return links, nil
			}
			return nil, z.Err()
		}

		if tokenType != html.StartTagToken {
			continue
		}

		token := z.Token()
		if token.DataAtom != atom.A {
			continue
		}

		for _, attr := range token.Attr {
			if attr.Key == atom.Href.String() {
				links = append(links, attr.Val)
			}
		}
	}
}
