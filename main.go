package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

var errorLog = log.New(os.Stderr, "ERROR: ", log.LstdFlags)

var routes = []string{"/p/", "/reels/", "/reel/"}

func startServer(resolvers []Resolver, port int) {
	handler := func(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Route hit: %s", routeHit)

		// If the request is from discord OR telegram, we proxify the request through the resolver
		ua := r.Header.Get("User-Agent")
		if strings.Contains(ua, "Discordbot") || strings.Contains(ua, "Telegram") {
			proxyRequest(w, r, resolvers)
			return
		}
		// Else, we simply redirect the user to the best resolver

		// ..the most performant resolver is always placed first in the array
		currentBest := resolvers[0]
		// Extract the post ID
		id := strings.TrimPrefix(r.URL.Path, routeHit)
		// Construct the redirect URL
		redirectURL := strings.Trim(currentBest.Url, "/") + routeHit + id

		// Send HTTP 302 redirect
		http.Redirect(w, r, redirectURL, http.StatusFound)
		log.Printf("User redirection toward %s", redirectURL)
	}
	// home page handler
	hpHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Service is running OK!"))

	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", hpHandler)
	for _, route := range routes {
		mux.HandleFunc("GET "+route, handler)
	}

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
}

func main() {
	port := flag.Int("p", 8080, "port to run the server on")
	flag.Parse()
	log.SetOutput(os.Stdout)
	resolvers, err := loadResolvers("resolvers.json")
	if err != nil {
		log.Fatalf("Error reading the file: %v", err)
	}

	go monitorResolvers(resolvers)
	startServer(resolvers, *port)
}
