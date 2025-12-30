package main

import (
	"bytes"
	"fmt"
	"github.com/Knoppiix/InstagramEmbedResolver/metrics"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
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
	var defaultDoc *goquery.Document
	var defaultContentType string

	for _, resolver := range resolvers {
		u, err := url.Parse(resolver.Url)
		if err != nil {
			panic(err)
		}
		fullUrl := u.Scheme + "://" + getSubdomain(req.Host) + u.Host + req.URL.Path

		// if using a specific mode (gallery, media-only) we check if the current resolver is able to handle it
		if !isResolverEligible(req, resolver) {
			continue
		}
		metrics.TotalRequests.Inc()
		log.Printf("Proxifying request to %s", fullUrl)

		start := time.Now()
		resp, err := client.Get(fullUrl)
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

		// We register the resolver metrics in prometheus
		latency := time.Since(start).Seconds()
		metrics.ResolverRequests.WithLabelValues(resolver.Url).Inc()
		metrics.ResolverLatency.WithLabelValues(resolver.Url).Observe(latency)

		if resolver.IsDefault && defaultDoc == nil {
			defaultDoc = doc
			defaultContentType = resp.Header.Get("Content-Type")
		}

		// the document must have meta tags to be returned. else, means the post resolving was not successful
		if hasMetaTags(doc) {
			sendRespToClient(w, resp.Header.Get("Content-Type"), doc)
			metrics.SuccessfulEmbeds.WithLabelValues(resolver.Url).Inc()
			return
		}

		// If not found, loop continues to next resolver
		log.Printf("No video tags found with current resolver, trying next...")
	}
	fmt.Println("Falling back  to default Resolver!")

	// If we reach here, all resolvers failed. Falling back to sending the query to the default resolver
	if defaultRes != (Resolver{}) && defaultDoc != nil {
		sendRespToClient(w, defaultContentType, defaultDoc)
		return
	}
	http.Error(w, `Post not found`, http.StatusBadGateway)
}

func sendRespToClient(w http.ResponseWriter, contentType string, doc *goquery.Document) error {
	html, err := doc.Html()
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
	return nil
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

func isResolverEligible(req *http.Request, res Resolver) bool {
	host := req.Host
	host = strings.Split(host, ":")[0] // get rid of the port
	sub := getSubdomain(host)

	switch sub {
	// gallery mode
	case "g.":
		if res.Gallery {
			return true
		}
	case "":
		return true
	}
	return false
}

func getSubdomain(host string) string {
	host = strings.Split(host, ":")[0]

	parts := strings.Split(host, ".")
	if len(parts) < 3 || parts[0] == "www" {
		return "" // no subdomain
	}
	return parts[0] + "."
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

	// I'm not sure the base tag is interpreted by discord's embedding scrapper. But in my tests, it SEEMS to work better with it.
	doc.Find("head").PrependHtml(`<base href="` + origin + `">`)

	// relative path to absolute path
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		if prop, ok := s.Attr("property"); ok && metaProps[prop] {
			if val, ok := s.Attr("content"); ok && strings.HasPrefix(val, "/") {
				s.SetAttr("content", origin+val)
			}
		}
		//the meta property twitter:player:stream may use "name" instead of "property"
		if name, ok := s.Attr("name"); ok && metaProps[name] {
			if val, ok := s.Attr("content"); ok && strings.HasPrefix(val, "/") {
				s.SetAttr("content", origin+val)
			}
		}
	})

	return doc, nil
}
