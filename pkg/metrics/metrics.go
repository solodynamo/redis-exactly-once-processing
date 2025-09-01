package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	WaitingConversationsCount prometheus.Gauge
	TimeoutNotificationsSent  *prometheus.CounterVec
	TimeoutLeaderChanges      prometheus.Counter
	TimeoutCheckDuration      prometheus.Histogram
	RedisOperationDuration    *prometheus.HistogramVec
	LeaderElectionDuration    prometheus.Histogram
	StreamProcessingDuration  prometheus.Histogram
	StreamMessagesProcessed   *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		WaitingConversationsCount: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "waiting_conversations_count",
			Help: "Current number of conversations waiting for customer response",
		}),
		TimeoutNotificationsSent: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "timeout_notifications_sent_total",
			Help: "Total number of timeout notifications sent",
		}, []string{"level"}),
		TimeoutLeaderChanges: promauto.NewCounter(prometheus.CounterOpts{
			Name: "timeout_leader_changes_total",
			Help: "Total number of leader changes",
		}),
		TimeoutCheckDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "timeout_check_duration_seconds",
			Help:    "Time taken to check all timeouts",
			Buckets: prometheus.DefBuckets,
		}),
		RedisOperationDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "redis_operation_duration_seconds",
			Help:    "Time taken for Redis operations",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		LeaderElectionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "leader_election_duration_seconds",
			Help:    "Time taken for leader election operations",
			Buckets: prometheus.DefBuckets,
		}),
		StreamProcessingDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "stream_processing_duration_seconds",
			Help:    "Time taken to process stream messages",
			Buckets: prometheus.DefBuckets,
		}),
		StreamMessagesProcessed: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "stream_messages_processed_total",
			Help: "Total number of stream messages processed",
		}, []string{"status"}),
	}
}
