package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	FeedFetches        = promauto.NewCounterVec(prometheus.CounterOpts{Name: "ipquality_feed_fetches_total", Help: "Feed source fetch/storage attempts"}, []string{"feed", "source", "status"})
	FeedLastSuccess    = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "ipquality_feed_last_success_timestamp_seconds", Help: "Last successful fetch/store per configured source"}, []string{"feed", "source"})
	FeedDuration       = promauto.NewHistogramVec(prometheus.HistogramOpts{Name: "ipquality_feed_fetch_duration_seconds", Help: "Fetch and storage duration", Buckets: []float64{1, 5, 15, 30, 60, 120, 300}}, []string{"feed", "source"})
	Compilations       = promauto.NewCounterVec(prometheus.CounterOpts{Name: "ipquality_compilations_total", Help: "Compilation attempts by result"}, []string{"status"})
	CompileLastSuccess = promauto.NewGauge(prometheus.GaugeOpts{Name: "ipquality_compile_last_success_timestamp_seconds", Help: "Last successfully published compilation"})
	CompileDuration    = promauto.NewHistogram(prometheus.HistogramOpts{Name: "ipquality_compile_duration_seconds", Help: "Compilation duration", Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600}})
	Reloads            = promauto.NewCounterVec(prometheus.CounterOpts{Name: "ipquality_mmdb_reload_total", Help: "MMDB reload checks by result"}, []string{"status"})
	SnapshotBuiltAt    = promauto.NewGauge(prometheus.GaugeOpts{Name: "ipquality_snapshot_build_timestamp_seconds", Help: "Loaded snapshot build timestamp"})
	SnapshotStale      = promauto.NewGauge(prometheus.GaugeOpts{Name: "ipquality_snapshot_stale", Help: "Snapshot or configured feed exceeds freshness budget"})
)

// Serve exposes pipeline metrics on a dedicated internal listener.
func Serve(ctx context.Context, host string, port int, path string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(stop)
	}()
	go server.Serve(listener)
	return nil
}
