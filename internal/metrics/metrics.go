package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Значения метки result для linktracker_link_checks_total
const (
	ResultUpdated   = "updated"
	ResultNoUpdates = "no_updates"
	ResultError     = "error"
)

// Бизнес-метрики
var (
	LinksAdded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "linktracker_links_added_total",
		Help: "Total number of links added to tracking.",
	})

	LinksRemoved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "linktracker_links_removed_total",
		Help: "Total number of links removed from tracking.",
	})

	LinkChecks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "linktracker_link_checks_total",
		Help: "Total number of link checks by source (checker) and result.",
	}, []string{"checker", "result"})

	NotificationsCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "linktracker_notifications_created_total",
		Help: "Total number of notifications created for chats.",
	})

	CheckCycleDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "linktracker_check_cycle_duration_seconds",
		Help:    "Duration of one scheduler cycle that checks all links.",
		Buckets: prometheus.ExponentialBuckets(0.05, 2, 10), // 0.05 с ... 25.6 с
	})
)

// Технические HTTP-метрики
var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "linktracker_http_requests_total",
		Help: "Total number of HTTP requests by method, route and status code.",
	}, []string{"method", "route", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "linktracker_http_request_duration_seconds",
		Help:    "HTTP request duration by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
)

func init() {
	for _, checker := range []string{"github", "stackoverflow"} {
		for _, result := range []string{ResultUpdated, ResultNoUpdates, ResultError} {
			LinkChecks.WithLabelValues(checker, result)
		}
	}
}
