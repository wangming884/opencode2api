package admin

import (
	"bytes"
	"net"
	"net/http"
	"strings"

	"opencode2api/internal/buildinfo"
	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
	"opencode2api/internal/telemetry"
)

// MetricsStatus is what the metrics page needs to describe the scrape
// endpoint: whether it is on, where it lives, and how a scraper authenticates.
type MetricsStatus struct {
	Enabled    bool                 `json:"enabled"`
	Path       string               `json:"path"`
	URL        string               `json:"url"`
	Auth       string               `json:"auth"`
	Embeddings EmbeddingsStatusView `json:"embeddings"`
}

// EmbeddingsStatusView describes the embeddings relay as the page shows it. The
// key is never returned, only whether one is stored.
type EmbeddingsStatusView struct {
	Enabled  bool   `json:"enabled"`
	BaseURL  string `json:"base_url"`
	HasKey   bool   `json:"has_key"`
	Upstream string `json:"upstream"`
}

// handleMetricsStatus reports the scrape endpoint state and the embeddings
// relay configuration.
func (a *Server) handleMetricsStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cfg := a.manager.Config()
	upstream := "Zen / Go 池"
	if cfg.Embeddings.BaseURL != "" {
		upstream = config.RedactURL(cfg.Embeddings.BaseURL)
	}
	httpx.WriteJSON(w, http.StatusOK, MetricsStatus{
		Enabled: cfg.Metrics.Enabled,
		Path:    "/metrics",
		URL:     metricsURL(cfg, r),
		Auth:    "Authorization: Bearer <server_key>",
		Embeddings: EmbeddingsStatusView{
			Enabled: cfg.Embeddings.Enabled, BaseURL: cfg.Embeddings.BaseURL,
			HasKey: cfg.Embeddings.APIKey != "", Upstream: upstream,
		},
	})
}

// handleSaveEmbeddings updates the embeddings relay: its switch, its dedicated
// origin and, when the field is present, its credential. An omitted api_key
// keeps the stored one, so a page that never reveals secrets cannot clear it by
// accident; sending an empty string clears it.
func (a *Server) handleSaveEmbeddings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool    `json:"enabled"`
		BaseURL string  `json:"base_url"`
		APIKey  *string `json:"api_key,omitempty"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, err := a.manager.Update(func(cfg *config.Config) error {
		cfg.Embeddings.Enabled = input.Enabled
		cfg.Embeddings.BaseURL = input.BaseURL
		if input.APIKey != nil {
			cfg.Embeddings.APIKey = *input.APIKey
		}
		return nil
	}); err != nil {
		writeAdminError(w, http.StatusBadRequest, "embeddings_rejected", a.manager.Redact(err.Error()))
		return
	}
	a.handleMetricsStatus(w, r)
}

// handleSaveMetrics turns the scrape endpoint on or off. Only that one setting
// changes, so the rest of the running configuration is preserved as is.
func (a *Server) handleSaveMetrics(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, err := a.manager.Update(func(cfg *config.Config) error {
		cfg.Metrics.Enabled = input.Enabled
		return nil
	}); err != nil {
		writeAdminError(w, http.StatusBadRequest, "metrics_rejected", a.manager.Redact(err.Error()))
		return
	}
	a.handleMetricsStatus(w, r)
}

// handleMetricsPreview renders the current exposition so the page can show
// what a scraper would receive without the caller holding a server key. The
// text is the same renderer the public endpoint uses.
func (a *Server) handleMetricsPreview(w http.ResponseWriter, r *http.Request) {
	var buffer bytes.Buffer
	if err := a.monitor.WritePrometheus(&buffer, a.clusterInfo()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "metrics_failed", "failed to render metrics")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"text": buffer.String(), "bytes": buffer.Len()})
}

// clusterInfo builds the resource view the exposition needs from the running
// runtime, without reaching into the gateway package.
func (a *Server) clusterInfo() telemetry.PrometheusCluster {
	resources := a.manager.Resources()
	cluster := telemetry.PrometheusCluster{
		Version:       buildinfo.Version,
		ModelsTotal:   resources.Models.Total,
		ModelsExposed: resources.Models.Exposed,
		ModelsStale:   resources.Models.Stale,
	}
	for _, key := range resources.Keys {
		if key.Tier == string(config.TierZen) {
			cluster.ZenKeys++
		} else {
			cluster.GoKeys++
		}
		if key.CooldownUntil != nil {
			cluster.KeysCooling++
		}
	}
	for _, proxy := range resources.Proxies {
		if proxy.Healthy {
			cluster.ProxiesHealthy++
		} else {
			cluster.ProxiesUnhealthy++
		}
	}
	return cluster
}

// metricsURL builds the scrape address shown on the page. A wildcard listener
// is replaced by the host the page was reached through, since 0.0.0.0 is not
// an address a scraper can dial.
func metricsURL(cfg config.Config, r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		host, port = "", "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if requestHost := r.Host; requestHost != "" {
			if name, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
				host = name
			} else {
				host = requestHost
			}
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port) + "/metrics"
}
