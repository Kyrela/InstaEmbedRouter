package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var metaProps = map[string]bool{
	"og:video":              true,
	"og:video:secure_url":   true,
	"twitter:player:stream": true,
	"og:image":              true,
	"twitter:card":          true,
	"twitter:image":         true,
}

func proxyRequest(w http.ResponseWriter, req *http.Request, resolvers []Resolver) {
	client := &http.Client{Timeout: 2 * time.Second}

	for i, resolver := range resolvers {
		log.Printf("Proxifying request to %s", resolver.Url+req.URL.Path)

		resp, err := client.Get(resolver.Url + req.URL.Path)
		if err != nil {
			log.Printf("Error with resolver %s: %v", resolver.Url, err)
			continue // try next resolver
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading body from %s: %v", resolver.Url, err)
			continue
		}

		doc, err := rewriteAndCleanDoc(bodyBytes, resolver.Url)
		if err != nil {
			log.Printf("Error building the document from %s: %v", resolver.Url, err)
			continue
		}

		if hasMetaTags(doc) {
			// Success — render once, return.
			html, err := doc.Html()
			if err != nil {
				http.Error(w, "render error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(html))
			return
		}

		// If not found, loop continues to next resolver
		log.Printf("No video tags found with resolver %d, trying next...", i)
	}
	// If we reach here, all resolvers failed.
	http.Error(w, "No video meta tags found on any resolver", http.StatusBadGateway)

}

func hasMetaTags(doc *goquery.Document) bool {
	found := false

	for prop := range metaProps {
		selector := fmt.Sprintf(`meta[property="%s"]`, prop)
		if doc.Find(selector).Length() > 0 {
			found = true
			break
		}
	}

	return found
}

// Cleaning the HTML body and replacing the relative URLs from the meta video tags to absolute paths
// because relative paths do not work through a proxifier
func rewriteAndCleanDoc(bodyBytes []byte, resolverURL string) (*goquery.Document, error) {
	// Searching for cloudflare's <script> tags that might break the embedding

	re := regexp.MustCompile(`(?s)<script[^>]+static\.cloudflareinsights\.com[^<]+</script>`)
	// And we're removing it
	bodyBytes = re.ReplaceAll(bodyBytes, nil)

	// Parse with goquery
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	// Compute origin (scheme + host)
	u, _ := url.Parse(resolverURL)
	origin := u.Scheme + "://" + u.Host

	// I'm not sure the base tag is interpreted by discord's embedding scrapper
	doc.Find("head").PrependHtml(`<base href="` + origin + `">`)

	// Fix only the specific video-related meta tags

	// relative path to absolute path
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if prop, ok := s.Attr("property"); ok && metaProps[prop] {
			if val, ok := s.Attr("content"); ok && strings.HasPrefix(val, "/") {
				s.SetAttr("content", origin+val)
			}
		}
		// twitter:player:stream may use "name" instead of "property"
		if name, ok := s.Attr("name"); ok && metaProps[name] {
			if val, ok := s.Attr("content"); ok && strings.HasPrefix(val, "/") {
				s.SetAttr("content", origin+val)
			}
		}
	})

	return doc, nil
}
