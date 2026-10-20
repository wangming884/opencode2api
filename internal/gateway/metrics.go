package gateway

import (
	"net/http"
	"time"

	"opencode2api/internal/buildinfo"
	wire "opencode2api/internal/protocol"
	"opencode2api/internal/telemetry"
)

// handleMetrics serves the Prometheus exposition of the same counters the
// WebUI monitor reads. It is registered on the API listener so a scrape works
// whether or not the management interface is enabled, and it authenticates
// with a server key like every other API route: the exposition names tiers,
// upstream keys and traffic, which is operator data rather than caller data.
func (g *Gateway) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	if !g.cfg.Metrics.Enabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"metrics are disabled; set metrics.enabled to true","type":"invalid_request_error"}}`))
		return
	}
	if g.monitor == nil {
		wire.WriteError(w, wire.Chat, http.StatusServiceUnavailable, "metrics are unavailable in this runtime", "internal_error", "")
		return
	}
	_, catalog := g.availableModels()
	proxyTotal, proxyHealthy := g.transports.healthCounts()
	staleAfter := max(2*time.Duration(g.cfg.Models.RefreshSeconds)*time.Second, time.Minute)
	cluster := telemetry.PrometheusCluster{
		Version:          buildinfo.Version,
		ModelsTotal:      catalog.Total,
		ModelsExposed:    catalog.Exposed,
		ModelsStale:      !catalog.UpdatedAt.IsZero() && time.Since(catalog.UpdatedAt) > staleAfter,
		ZenKeys:          g.zenNodes.Len(),
		GoKeys:           g.goNodes.Len(),
		KeysCooling:      coolingKeys(g.zenNodes) + coolingKeys(g.goNodes),
		ProxiesHealthy:   proxyHealthy,
		ProxiesUnhealthy: proxyTotal - proxyHealthy,
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	// A write error here only means the scraper went away mid-body; the
	// connection is already committed, so there is nothing left to report.
	_ = g.monitor.WritePrometheus(w, cluster)
}

// coolingKeys counts the pool's keys currently in failure cooldown.
func coolingKeys(pool *upstreamNodes) int {
	count := 0
	now := time.Now().UnixNano()
	for _, node := range pool.nodes {
		if node.cooldownUntil.Load() > now {
			count++
		}
	}
	return count
}
