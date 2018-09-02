package parse

import (
	"strings"
	"testing"
)

func TestHTML(t *testing.T) {
	type testcase struct {
		name      string
		html      string
		links     []string
		expectErr bool
	}

	testcases := []testcase{
		testcase{
			name: "emptyString",
		},
		testcase{
			name:  "oneLinkNoClosing",
			html:  `<a href="google.com">`,
			links: []string{"google.com"},
		},
		testcase{
			name:  "oneLinkNoClosingSingleQuotes",
			html:  `<a href='google.com'>`,
			links: []string{"google.com"},
		},
		testcase{
			name:  "oneLinkClosing",
			html:  `<a href='google.com'></a>`,
			links: []string{"google.com"},
		},
		testcase{
			name:  "twoLinks",
			html:  `<a href='google.com'></a><a href="bing.com"></a>`,
			links: []string{"google.com", "bing.com"},
		},
		testcase{
			name:  "nullLink",
			html:  `<a href>`,
			links: []string{""},
		},
		testcase{
			name:  "emptyStringLink",
			html:  `<a href="">`,
			links: []string{""},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := HTML(strings.NewReader(tt.html))
			wasErr := err != nil
			if tt.expectErr != wasErr {
				t.Errorf("expectErr=%v but got err=%v", tt.expectErr, err)
				return
			}

			if len(actual) != len(tt.links) {
				t.Errorf("expected %d links: %v\n got %d links: %v\n", len(tt.links), tt.links, len(actual), actual)
				return
			}

			for i := range actual {
				if actual[i] != tt.links[i] {
					t.Errorf("link %d: expected %s, got %s", i, tt.links[i], actual[i])
				}
			}

		})
	}
}
