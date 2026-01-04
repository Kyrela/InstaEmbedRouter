package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	TotalRequests = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "instaembed",
			Name:      "requests_total",
			Help:      "Total number of incoming requests",
		},
	)

	ResolverRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "instaembed",
			Name:      "resolver_requests_total",
			Help:      "Number of requests handled by each resolver",
		},
		[]string{"resolver"},
	)

	SuccessfulEmbeds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "instaembed",
			Name:      "resolver_success_total",
			Help:      "Number of successful embeds per resolver",
		},
		[]string{"resolver"},
	)

	ResolverLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "instaembed",
			Name:      "resolver_latency_seconds",
			Help:      "Latency of resolver responses",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"resolver"},
	)
)

func Init() {
	prometheus.MustRegister(
		TotalRequests,
		ResolverRequests,
		SuccessfulEmbeds,
		ResolverLatency,
	)
}
