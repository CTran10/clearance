package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultService = "clearance"

type OperationalSnapshot struct {
	OutboxPending                 float64
	OutboxDeadLettered            float64
	OutboxOldestPendingAgeSeconds float64
	DeadLettersOpen               float64
	ProcessedEvents               float64
	PostgresPoolOpen              float64
	PostgresPoolIdle              float64
	PostgresPoolInUse             float64
}

type SnapshotProvider interface {
	OperationalMetrics(context.Context) (OperationalSnapshot, error)
}

type Registry struct {
	service string
	handler http.Handler

	httpRequests          *prometheus.CounterVec
	httpDuration          *prometheus.HistogramVec
	kafkaPublished        *prometheus.CounterVec
	outboxEventsTotal     *prometheus.CounterVec
	outboxPublishDuration *prometheus.HistogramVec
	consumerMessages      *prometheus.CounterVec
	consumerDuration      *prometheus.HistogramVec
	consumerRetries       *prometheus.CounterVec
	consumerCommitFailure *prometheus.CounterVec
	outboxEvents          *prometheus.GaugeVec
	outboxOldestAge       *prometheus.GaugeVec
	deadLettersOpen       *prometheus.GaugeVec
	processedEvents       *prometheus.GaugeVec
	postgresPool          *prometheus.GaugeVec
}

var (
	defaultMu sync.RWMutex
	Default   = NewRegistry(defaultService)
)

func NewRegistry(service string) *Registry {
	if service == "" {
		service = defaultService
	}
	registry := prometheus.NewRegistry()
	r := &Registry{
		service: service,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "clearance_http_requests_total",
			Help: "Total HTTP requests handled by method, normalized path, and status.",
		}, []string{"method", "path", "service", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "clearance_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "service"}),
		kafkaPublished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "clearance_kafka_messages_published_total",
			Help: "Total Kafka publish attempts by topic and result.",
		}, []string{"result", "service", "topic"}),
		outboxEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "clearance_outbox_publish_attempts_total",
			Help: "Total outbox publish attempts by result.",
		}, []string{"result", "service"}),
		outboxPublishDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "clearance_outbox_publish_duration_seconds",
			Help:    "Outbox publish attempt duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result", "service"}),
		consumerMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "clearance_consumer_messages_total",
			Help: "Total Kafka messages completed by consumer, topic, and result.",
		}, []string{"consumer", "result", "service", "topic"}),
		consumerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "clearance_consumer_message_duration_seconds",
			Help:    "Kafka message handling duration including retries.",
			Buckets: prometheus.DefBuckets,
		}, []string{"consumer", "result", "service", "topic"}),
		consumerRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "clearance_consumer_retries_total",
			Help: "Total Kafka consumer retry attempts.",
		}, []string{"consumer", "service", "topic"}),
		consumerCommitFailure: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "clearance_consumer_offset_commit_failures_total",
			Help: "Total Kafka consumer offset commit failures.",
		}, []string{"consumer", "service", "topic"}),
		outboxEvents: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "clearance_outbox_events",
			Help: "Current outbox event count by status.",
		}, []string{"service", "status"}),
		outboxOldestAge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "clearance_outbox_oldest_pending_age_seconds",
			Help: "Age of the oldest pending outbox event in seconds.",
		}, []string{"service"}),
		deadLettersOpen: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "clearance_dead_letters_open",
			Help: "Current number of open dead-letter records.",
		}, []string{"service"}),
		processedEvents: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "clearance_processed_events",
			Help: "Current number of processed-event deduplication records.",
		}, []string{"service"}),
		postgresPool: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "clearance_postgres_pool_connections",
			Help: "Current PostgreSQL connection-pool size by state.",
		}, []string{"service", "state"}),
	}
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
		r.httpRequests,
		r.httpDuration,
		r.kafkaPublished,
		r.outboxEventsTotal,
		r.outboxPublishDuration,
		r.consumerMessages,
		r.consumerDuration,
		r.consumerRetries,
		r.consumerCommitFailure,
		r.outboxEvents,
		r.outboxOldestAge,
		r.deadLettersOpen,
		r.processedEvents,
		r.postgresPool,
	)
	r.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return r
}

func Configure(service string) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	Default = NewRegistry(service)
}

func Handler() http.Handler {
	return defaultRegistry().Handler()
}

func ObserveHTTPRequest(method string, path string, status string, duration time.Duration) {
	defaultRegistry().ObserveHTTPRequest(method, path, status, duration)
}

func KafkaPublish(topic string, result string) {
	defaultRegistry().KafkaPublish(topic, result)
}

func OutboxPublish(result string, duration time.Duration) {
	defaultRegistry().OutboxPublish(result, duration)
}

func ObserveConsumerMessage(consumer string, topic string, result string, duration time.Duration) {
	defaultRegistry().ObserveConsumerMessage(consumer, topic, result, duration)
}

func IncConsumerRetry(consumer string, topic string) {
	defaultRegistry().IncConsumerRetry(consumer, topic)
}

func IncOffsetCommitFailure(consumer string, topic string) {
	defaultRegistry().IncOffsetCommitFailure(consumer, topic)
}

func SetOperationalSnapshot(snapshot OperationalSnapshot) {
	defaultRegistry().SetOperationalSnapshot(snapshot)
}

func StartSampler(ctx context.Context, interval time.Duration, provider SnapshotProvider) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	collect := func() {
		snapshot, err := provider.OperationalMetrics(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("operational metrics collection failed", "err", err)
			}
			return
		}
		SetOperationalSnapshot(snapshot)
	}
	collect()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collect()
			}
		}
	}()
}

func (r *Registry) Handler() http.Handler {
	return r.handler
}

func (r *Registry) ObserveHTTPRequest(method string, path string, status string, duration time.Duration) {
	r.httpRequests.WithLabelValues(method, path, r.service, status).Inc()
	r.httpDuration.WithLabelValues(method, path, r.service).Observe(duration.Seconds())
}

func (r *Registry) KafkaPublish(topic string, result string) {
	r.kafkaPublished.WithLabelValues(result, r.service, topic).Inc()
}

func (r *Registry) OutboxPublish(result string, duration time.Duration) {
	r.outboxEventsTotal.WithLabelValues(result, r.service).Inc()
	r.outboxPublishDuration.WithLabelValues(result, r.service).Observe(duration.Seconds())
}

func (r *Registry) ObserveConsumerMessage(consumer string, topic string, result string, duration time.Duration) {
	r.consumerMessages.WithLabelValues(consumer, result, r.service, topic).Inc()
	r.consumerDuration.WithLabelValues(consumer, result, r.service, topic).Observe(duration.Seconds())
}

func (r *Registry) IncConsumerRetry(consumer string, topic string) {
	r.consumerRetries.WithLabelValues(consumer, r.service, topic).Inc()
}

func (r *Registry) IncOffsetCommitFailure(consumer string, topic string) {
	r.consumerCommitFailure.WithLabelValues(consumer, r.service, topic).Inc()
}

func (r *Registry) SetOperationalSnapshot(snapshot OperationalSnapshot) {
	r.outboxEvents.WithLabelValues(r.service, "PENDING").Set(snapshot.OutboxPending)
	r.outboxEvents.WithLabelValues(r.service, "DEAD_LETTERED").Set(snapshot.OutboxDeadLettered)
	r.outboxOldestAge.WithLabelValues(r.service).Set(snapshot.OutboxOldestPendingAgeSeconds)
	r.deadLettersOpen.WithLabelValues(r.service).Set(snapshot.DeadLettersOpen)
	r.processedEvents.WithLabelValues(r.service).Set(snapshot.ProcessedEvents)
	r.postgresPool.WithLabelValues(r.service, "open").Set(snapshot.PostgresPoolOpen)
	r.postgresPool.WithLabelValues(r.service, "idle").Set(snapshot.PostgresPoolIdle)
	r.postgresPool.WithLabelValues(r.service, "in_use").Set(snapshot.PostgresPoolInUse)
}

func defaultRegistry() *Registry {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return Default
}
