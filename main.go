package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Knoppiix/InstagramEmbedResolver/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var errorLog = log.New(os.Stderr, "ERROR: ", log.LstdFlags)

var routes = []string{"/p/", "/reels/", "/reel/"}
var defaultRes Resolver

func startServer(resolvers []Resolver, port int) {
	template := template.Must(template.ParseFiles("templates/index.html"))
	// home page handler
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		template.Execute(w, "")
	})

	for _, route := range routes {
		mux.HandleFunc("GET "+route, reqHandler(resolvers))
	}
	// expose the prometheus metrics route
	mux.Handle("GET /metrics", promhttp.Handler())

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}

func reqHandler(resolvers []Resolver) http.HandlerFunc {
	// handler function for proxifying the requests to the resolvers
	return func(w http.ResponseWriter, r *http.Request) {
		routeHit := ""
		for _, route := range routes {
			if strings.HasPrefix(r.URL.Path, route) {
				routeHit = route
				break
			}
		}
		if routeHit == "" {
			http.NotFound(w, r)
			return
		}

		// If the request is from discord OR telegram, we proxify the request through the resolver
		ua := r.Header.Get("User-Agent")
		if strings.Contains(ua, "Discordbot") || strings.Contains(ua, "Telegram") {
			proxyRequest(w, r, resolvers)
			return
		}
		// Else, we simply redirect the user to the instagram post

		// Extract the post ID
		id := strings.TrimPrefix(r.URL.Path, routeHit)
		// Construct the redirect URL
		redirectURL := "https://www.instagram.com" + routeHit + id

		// Send HTTP 302 redirect
		http.Redirect(w, r, redirectURL, http.StatusFound)
		log.Printf("User redirection toward %s", redirectURL)
	}
}

func findDefaultResolver(res []Resolver) (Resolver, error) {
	for _, res := range res {
		if ok, err := res.isDefault(); err == nil && ok {
			return res, nil
			//fmt.Printf("%s is the default resolver.", res.Url)
		}
	}
	return Resolver{}, errors.New("No default resolver was found. Default resolving error behaviour is falling back to a HTTP return.")
}

func main() {
	port := flag.Int("p", 8080, "port to run the server on")
	flag.Parse()
	log.SetOutput(os.Stdout)
	resolvers, err := loadResolvers("resolvers.json")
	if err != nil {
		log.Fatalf("Error reading the file: %v", err)
	}
	defaultRes, err = findDefaultResolver(resolvers)
	if err != nil {
		fmt.Printf("WARNING - No default resolver was specified.")
	}
	metrics.Init()
	go monitorResolvers(resolvers)
	startServer(resolvers, *port)
}
