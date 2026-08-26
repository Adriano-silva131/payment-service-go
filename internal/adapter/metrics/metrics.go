package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	CheckoutsStarted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_checkouts_started_total",
			Help: "Number of checkout sessions successfully started, by gateway.",
		},
		[]string{"gateway"},
	)

	WebhooksProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_webhooks_processed_total",
			Help: "Number of gateway webhook calls processed, by gateway and outcome.",
		},
		[]string{"gateway", "outcome"},
	)
)

func init() {
	prometheus.MustRegister(CheckoutsStarted, WebhooksProcessed)
}
