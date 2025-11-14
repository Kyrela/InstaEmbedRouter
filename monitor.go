package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

const checkInterval = 3 // In minutes, the interval to check the resolvers
const logInterval = 15  // Same, but for the logging of resolvers latency
var lastLogTime time.Time

func loadResolvers(filename string) ([]Resolver, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var resolvers []Resolver
	if err := json.Unmarshal(file, &resolvers); err != nil {
		return nil, err
	}
	log.Printf("Successfully loaded %d resolvers!", len(resolvers))

	return resolvers, nil
}

// To set the best resolver, we simply  put the one with the least latency as the 1st element of the Resolvers array
// this way, the proxy logic will try the best one 1st
func electBestResolver(resolvers []Resolver) {
	if len(resolvers) == 0 {
		return
	}

	bestIndex := -1
	for i := range resolvers {
		if !resolvers[i].IsUp {
			continue
		}
		if bestIndex == -1 || resolvers[i].Latency < resolvers[bestIndex].Latency {
			bestIndex = i
		}
	}

	if bestIndex > 0 {
		resolvers[0], resolvers[bestIndex] = resolvers[bestIndex], resolvers[0]
		log.Printf("Best resolver moved to index 0: %s", resolvers[0].Url)
	}
}

func monitorResolvers(resolvers []Resolver) {
	ticker := time.NewTicker(checkInterval * time.Minute)
	defer ticker.Stop()

	for {
		for i := range resolvers {
			func(r *Resolver) {

				// This method returns true if the resolver is up AND evaluates the resolver's latency
				isUp, err := r.IsHttpUp()

				if isUp {
					if !r.IsUp {
						// If the resolver was previously down, start the uptime
						r.UptimeStart = time.Now()
					}
					r.LastChecked = time.Now()
				} else {
					r.UptimeStart = time.Time{} // reset
					r.LastChecked = time.Now()
				}
				r.IsUp = isUp

				if err != nil {
					log.Printf("The http health check request failed : %v", err)
					return
				}

				// We're logging once every logInterval min
				if time.Since(lastLogTime) >= logInterval*time.Minute {
					log.Printf("%s is %s since %d seconds, and pings %s", r.Url, upStatus(r.IsUp), int(time.Since(r.UptimeStart).Seconds()), r.Latency)
					lastLogTime = time.Now()
				}

			}(&resolvers[i])
		}
		// We check for a new best resolver each time we're monitoring the resolvers
		electBestResolver(resolvers)
		<-ticker.C
	}
}

// just a string formatting function for log purposes
func upStatus(isUp bool) string {
	if isUp {
		return "up"
	}
	return "not up"
}
