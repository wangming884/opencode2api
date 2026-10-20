package telemetry

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// PrometheusCluster is the runtime resource view the exposition needs but the
// metrics store does not own: the build version, the catalog and pool sizes.
// Callers fill it from the gateway runtime, so telemetry stays independent of
// the gateway package.
type PrometheusCluster struct {
	Version          string
	ModelsTotal      int
	ModelsExposed    int
	ModelsStale      bool
	ZenKeys          int
	GoKeys           int
	KeysCooling      int
	ProxiesHealthy   int
	ProxiesUnhealthy int
}

// WritePrometheus renders the metrics in the Prometheus text exposition
// format (version 0.0.4) so a standard scrape config can read them without
// any client library dependency.
//
// Counters are process-lifetime totals, matching the WebUI monitor. Latency
// quantiles describe the last hour, because the histogram is kept per minute
// and only that window is retained; they are upper bucket bounds, not exact
// quantiles.
func (m *Metrics) WritePrometheus(w io.Writer, cluster PrometheusCluster) error {
	snapshot := m.Snapshot()
	e := &prometheusEncoder{w: w}

	e.metric("opencode2api_build_info", "gauge", "Build information of the running gateway.", func() {
		e.sample("opencode2api_build_info", map[string]string{"version": cluster.Version}, 1)
	})
	e.metric("opencode2api_start_time_seconds", "gauge", "Unix time the process started.", func() {
		e.sample("opencode2api_start_time_seconds", nil, float64(snapshot.StartedAt.Unix()))
	})
	e.metric("opencode2api_uptime_seconds", "gauge", "Seconds since the process started.", func() {
		e.sample("opencode2api_uptime_seconds", nil, float64(snapshot.UptimeSeconds))
	})
	e.metric("opencode2api_requests_active", "gauge", "Requests currently being served.", func() {
		e.sample("opencode2api_requests_active", nil, float64(snapshot.Active))
	})
	e.metric("opencode2api_streams_active", "gauge", "Response streams currently open.", func() {
		e.sample("opencode2api_streams_active", nil, float64(snapshot.ActiveStreams))
	})

	lifetime := snapshot.Lifetime
	e.metric("opencode2api_requests_total", "counter", "Completed requests by outcome since start.", func() {
		e.sample("opencode2api_requests_total", map[string]string{"result": "success"}, float64(lifetime.Success))
		e.sample("opencode2api_requests_total", map[string]string{"result": "error"}, float64(lifetime.Errors))
	})
	e.metric("opencode2api_requests_success_ratio", "gauge", "Share of requests that succeeded since start.", func() {
		e.sample("opencode2api_requests_success_ratio", nil, lifetime.SuccessRate)
	})

	e.counters("opencode2api_requests_by_endpoint_total", "Requests by endpoint path since start.", "endpoint", snapshot.Endpoints)
	e.counters("opencode2api_requests_by_model_total", "Requests by requested model since start.", "model", snapshot.Models)
	e.counters("opencode2api_requests_by_tier_total", "Requests by serving tier since start.", "tier", snapshot.Tiers)
	e.counters("opencode2api_requests_by_status_total", "Requests by HTTP status since start.", "status", snapshot.Statuses)

	// The histogram is aggregated per minute, so only the retained hour can
	// answer these; each value is the upper bound of its bucket.
	window := snapshot.Window
	e.metric("opencode2api_request_duration_seconds", "gauge", "Request latency over the last hour, as upper histogram bucket bounds.", func() {
		if window.Total == 0 {
			return
		}
		e.sample("opencode2api_request_duration_seconds", map[string]string{"quantile": "0.5"}, float64(window.P50MS)/1000)
		e.sample("opencode2api_request_duration_seconds", map[string]string{"quantile": "0.95"}, float64(window.P95MS)/1000)
		e.sample("opencode2api_request_duration_seconds", map[string]string{"quantile": "0.99"}, float64(window.P99MS)/1000)
	})
	e.metric("opencode2api_request_duration_seconds_avg", "gauge", "Mean request latency over the last hour.", func() {
		if window.Total == 0 {
			return
		}
		e.sample("opencode2api_request_duration_seconds_avg", nil, window.AverageMS/1000)
	})

	usage := snapshot.Usage.Lifetime
	e.metric("opencode2api_tokens_total", "counter", "Reported tokens since start by type.", func() {
		e.sample("opencode2api_tokens_total", map[string]string{"type": "input"}, float64(usage.Tokens.Input))
		e.sample("opencode2api_tokens_total", map[string]string{"type": "output"}, float64(usage.Tokens.Output))
		e.sample("opencode2api_tokens_total", map[string]string{"type": "cached"}, float64(usage.Tokens.Cached))
		e.sample("opencode2api_tokens_total", map[string]string{"type": "reasoning"}, float64(usage.Tokens.Reasoning))
		e.sample("opencode2api_tokens_total", map[string]string{"type": "total"}, float64(usage.Tokens.Total))
	})
	e.metric("opencode2api_token_usage_coverage_ratio", "gauge", "Share of routed requests whose upstream reported usage.", func() {
		e.sample("opencode2api_token_usage_coverage_ratio", nil, usage.Coverage)
	})
	e.metric("opencode2api_cost_usd_total", "counter", "Spend since start, in USD, priced from the model catalog.", func() {
		e.sample("opencode2api_cost_usd_total", nil, usage.Cost)
	})
	e.metric("opencode2api_server_key_requests_total", "counter", "Requests since start by local server key fingerprint.", func() {
		ids := sortedKeys(usage.ServerKeys)
		for _, id := range ids {
			e.sample("opencode2api_server_key_requests_total", map[string]string{"server_key": id}, float64(usage.ServerKeys[id].Requests))
		}
	})
	e.metric("opencode2api_server_key_tokens_total", "counter", "Tokens since start by local server key fingerprint.", func() {
		ids := sortedKeys(usage.ServerKeys)
		for _, id := range ids {
			e.sample("opencode2api_server_key_tokens_total", map[string]string{"server_key": id}, float64(usage.ServerKeys[id].Total))
		}
	})
	e.metric("opencode2api_server_key_cost_usd_total", "counter", "Spend since start by local server key fingerprint.", func() {
		ids := sortedKeys(usage.ServerKeys)
		for _, id := range ids {
			e.sample("opencode2api_server_key_cost_usd_total", map[string]string{"server_key": id}, usage.ServerKeys[id].Cost)
		}
	})

	upstream := snapshot.Upstream.Lifetime
	e.metric("opencode2api_upstream_attempts_total", "counter", "Upstream attempts since start by outcome.", func() {
		e.sample("opencode2api_upstream_attempts_total", map[string]string{"result": "success"}, float64(upstream.Success))
		e.sample("opencode2api_upstream_attempts_total", map[string]string{"result": "failed"}, float64(upstream.Failed))
	})
	e.metric("opencode2api_upstream_attempt_success_ratio", "gauge", "Share of upstream attempts that succeeded since start.", func() {
		e.sample("opencode2api_upstream_attempt_success_ratio", nil, upstream.SuccessRate)
	})
	e.attemptCounters("opencode2api_upstream_attempts_by_tier_total", "Upstream attempts since start by tier and outcome.", "tier", upstream.Tiers)
	e.attemptCounters("opencode2api_upstream_attempts_by_key_total", "Upstream attempts since start by key and outcome.", "key", upstream.Keys)
	e.attemptCounters("opencode2api_upstream_attempts_by_channel_total", "Upstream attempts since start by channel and outcome.", "channel", upstream.Channels)

	e.metric("opencode2api_models_total", "gauge", "Models known to the catalog.", func() {
		e.sample("opencode2api_models_total", nil, float64(cluster.ModelsTotal))
	})
	e.metric("opencode2api_models_exposed", "gauge", "Models servable with the current configuration.", func() {
		e.sample("opencode2api_models_exposed", nil, float64(cluster.ModelsExposed))
	})
	e.metric("opencode2api_models_stale", "gauge", "1 when the catalog is past its staleness threshold.", func() {
		stale := 0.0
		if cluster.ModelsStale {
			stale = 1
		}
		e.sample("opencode2api_models_stale", nil, stale)
	})
	e.metric("opencode2api_upstream_keys", "gauge", "Configured upstream keys by pool.", func() {
		e.sample("opencode2api_upstream_keys", map[string]string{"pool": "zen"}, float64(cluster.ZenKeys))
		e.sample("opencode2api_upstream_keys", map[string]string{"pool": "go"}, float64(cluster.GoKeys))
	})
	e.metric("opencode2api_upstream_keys_cooling", "gauge", "Upstream keys currently in failure cooldown.", func() {
		e.sample("opencode2api_upstream_keys_cooling", nil, float64(cluster.KeysCooling))
	})
	e.metric("opencode2api_proxies", "gauge", "Configured proxy routes by health.", func() {
		e.sample("opencode2api_proxies", map[string]string{"health": "healthy"}, float64(cluster.ProxiesHealthy))
		e.sample("opencode2api_proxies", map[string]string{"health": "unhealthy"}, float64(cluster.ProxiesUnhealthy))
	})
	return e.err
}

// prometheusEncoder accumulates exposition text, keeping the first write error
// so a failing client connection stops the work without a check per line.
type prometheusEncoder struct {
	w   io.Writer
	err error
}

// metric writes the HELP and TYPE lines, then the samples body builds. An
// empty body still emits the family header, which keeps a scrape stable before
// the first request.
func (e *prometheusEncoder) metric(name, kind, help string, body func()) {
	if e.err != nil {
		return
	}
	e.printf("# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
	body()
}

// counters writes one sample per map entry, ordered by label so successive
// scrapes stay comparable.
func (e *prometheusEncoder) counters(name, help, label string, values map[string]uint64) {
	e.metric(name, "counter", help, func() {
		for _, key := range sortedKeys(values) {
			e.sample(name, map[string]string{label: key}, float64(values[key]))
		}
	})
}

// attemptCounters writes total, success and failed samples per map entry.
func (e *prometheusEncoder) attemptCounters(name, help, label string, values map[string]AttemptCounts) {
	e.metric(name, "counter", help, func() {
		for _, key := range sortedKeys(values) {
			counts := values[key]
			labels := map[string]string{label: key, "result": "success"}
			e.sample(name, labels, float64(counts.Success))
			labels = map[string]string{label: key, "result": "failed"}
			e.sample(name, labels, float64(counts.Failed))
		}
	})
}

// sample writes one series. Labels are sorted by name, which Prometheus
// requires only for stability but keeps diffs readable.
func (e *prometheusEncoder) sample(name string, labels map[string]string, value float64) {
	if e.err != nil {
		return
	}
	e.printf("%s%s %s\n", name, renderLabels(labels), formatFloat(value))
}

func (e *prometheusEncoder) printf(format string, args ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

// renderLabels renders a label set, escaping the characters the format gives
// special meaning: backslash, double quote and newline.
func renderLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	builder.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(name)
		builder.WriteString(`="`)
		builder.WriteString(escapeLabelValue(labels[name]))
		builder.WriteByte('"')
	}
	builder.WriteByte('}')
	return builder.String()
}

func escapeLabelValue(value string) string {
	if !strings.ContainsAny(value, "\\\"\n") {
		return value
	}
	var builder strings.Builder
	builder.Grow(len(value) + 8)
	for _, char := range value {
		switch char {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\n':
			builder.WriteString(`\n`)
		default:
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

// formatFloat renders a value the way the exposition format expects: integers
// without a decimal point, and the non-finite values as their word forms.
func formatFloat(value float64) string {
	switch {
	case value != value:
		return "NaN"
	case value > 1.7976931348623157e308:
		return "+Inf"
	case value < -1.7976931348623157e308:
		return "-Inf"
	}
	// Timestamps and counters read better without an exponent, which 'g' would
	// otherwise switch to past a few significant digits.
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatInt(int64(value), 10)
	}
	// Twelve significant digits keep the exposition readable without turning a
	// sum of small costs into 2.9399999999999996e-05.
	return strconv.FormatFloat(value, 'g', 12, 64)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
