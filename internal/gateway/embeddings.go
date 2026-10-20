package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
	"opencode2api/internal/identity"
	"opencode2api/internal/jsonutil"
	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
	"opencode2api/internal/telemetry"
)

// maxEmbeddingResponse bounds the buffered embeddings reply. The body is read
// whole so token usage can be reported; a larger reply is reported as an
// upstream error instead of being partially relayed.
const maxEmbeddingResponse = 64 << 20

// externalEmbeddingsChannel labels the upstream attempts of a request served by
// a dedicated embeddings origin, so monitoring separates it from Zen/Go traffic.
const externalEmbeddingsChannel = "external"

// handleEmbeddings serves the OpenAI embeddings endpoint as a relay.
//
// Nothing is translated: an embeddings request and its reply already share one
// shape, so the client's body is forwarded unchanged and the upstream's answer
// — success or error — is returned as it arrived. Only routing, key rotation,
// tier fallback and telemetry are the gateway's own work.
//
// Two upstreams are possible. When embeddings.base_url is set the request goes
// to that dedicated OpenAI-compatible origin, which is the only way to serve
// embeddings when the main upstream does not implement the endpoint. Otherwise
// it goes through the Zen/Go pools like any other inference call.
//
// The model is not resolved against the catalog. Embedding models are not chat
// models, so the catalog's protocol filtering would reject exactly the IDs this
// endpoint exists to serve; the upstream is the authority on whether a model
// can embed, and its rejection is relayed verbatim.
func (g *Gateway) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if !g.cfg.Embeddings.Enabled {
		wire.WriteError(w, wire.Embeddings, http.StatusNotFound, "embeddings are disabled; set embeddings.enabled to true", "invalid_request_error", "")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		wire.WriteError(w, wire.Embeddings, http.StatusBadRequest, "request body is too large or unreadable", "invalid_request_error", "")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		wire.WriteError(w, wire.Embeddings, http.StatusBadRequest, "request body must be a JSON object", "invalid_request_error", "")
		return
	}
	model := jsonutil.StringAt(payload, "model")
	meta := telemetry.MetaFromRequest(r)
	if meta != nil {
		meta.Model = model
	}
	if model == "" {
		wire.WriteError(w, wire.Embeddings, http.StatusBadRequest, "model is required", "invalid_request_error", "model")
		return
	}
	if _, ok := payload["input"]; !ok {
		wire.WriteError(w, wire.Embeddings, http.StatusBadRequest, "input is required", "invalid_request_error", "input")
		return
	}
	if !g.cfg.ServerKeyAllowsModel(callerScope(r.Context()), model) {
		g.limiter.Release(callerScope(r.Context()))
		wire.WriteError(w, wire.Embeddings, http.StatusForbidden, fmt.Sprintf("model %q is not allowed for this API key", model), "permission_error", "model")
		return
	}
	defer g.finishRequest(r)
	// OpenAI embeddings are never streamed. Rejecting the flag here is clearer
	// than forwarding a body the upstream would only ignore or misread.
	if jsonutil.BoolAt(payload, "stream") {
		wire.WriteError(w, wire.Embeddings, http.StatusBadRequest, "embeddings do not support streaming", "invalid_request_error", "stream")
		return
	}
	ids := identity.DeriveRequestIDs(r, payload, callerScope(r.Context()))
	if meta != nil {
		meta.Request = ids.Request
		meta.Session = ids.Session
		meta.Protocol = wire.Embeddings
	}
	requestCtx, cancel := context.WithTimeout(r.Context(), time.Duration(g.cfg.Retry.TimeoutSeconds)*time.Second)
	defer cancel()
	if g.cfg.Embeddings.BaseURL != "" {
		g.forwardExternalEmbeddings(w, r, requestCtx, body, model, ids, meta)
		return
	}
	g.forwardPoolEmbeddings(w, r, requestCtx, body, model, ids, meta)
}

// forwardPoolEmbeddings relays an embeddings request through the Zen/Go key
// pools, with the usual tier fallback and key rotation.
func (g *Gateway) forwardPoolEmbeddings(w http.ResponseWriter, r *http.Request, ctx context.Context, body []byte, model string, ids identity.RequestIDs, meta *telemetry.RequestMeta) {
	route, err := g.embeddingsRoute(model)
	if err != nil {
		wire.WriteError(w, wire.Embeddings, http.StatusBadRequest, err.Error(), "invalid_request_error", "model")
		return
	}
	if meta != nil {
		meta.Tier = string(route.Tier)
	}
	// The same body serves every tier: embeddings carry no per-protocol
	// encoding, so a tier fallback never needs a re-encode. The Zen entry is
	// always present because the anonymous lane reads bodies[TierZen].
	bodies := make(map[config.Tier][]byte, len(route.KeyTiers)+1)
	bodies[config.TierZen] = body
	for _, tier := range route.KeyTiers {
		bodies[tier] = body
	}
	resp, upstreamRoute, err := g.doUpstream(ctx, route, bodies, ids)
	if err != nil {
		g.writeEmbeddingsFailure(w, r, ctx, err, ids, meta, route.Tier)
		return
	}
	if meta != nil {
		meta.Tier = string(upstreamRoute.Tier)
	}
	g.writeEmbeddingsResponse(w, resp, ids, meta)
}

// forwardExternalEmbeddings relays an embeddings request to the dedicated
// origin from embeddings.base_url. There is no key pool there, so the retry
// budget applies to that one endpoint: a transport failure, a rate limit or a
// server error is retried, and any other answer is final.
func (g *Gateway) forwardExternalEmbeddings(w http.ResponseWriter, r *http.Request, ctx context.Context, body []byte, model string, ids identity.RequestIDs, meta *telemetry.RequestMeta) {
	if meta != nil {
		meta.Tier = externalEmbeddingsChannel
	}
	endpoint := externalEmbeddingsEndpoint(g.cfg.Embeddings.BaseURL)
	// The tier label is the channel, not Zen: these attempts never touch a key
	// pool, and reporting them under a pool would misattribute the traffic.
	externalTier := config.Tier(externalEmbeddingsChannel)
	route := models.Route{ID: model, Tier: externalTier, Protocol: wire.Embeddings}
	if len(g.transports.items) == 0 {
		wire.WriteError(w, wire.Embeddings, http.StatusServiceUnavailable, "no proxy route is available for the embeddings upstream", "proxy_error", ids.Request)
		return
	}
	attempts := max(1, g.cfg.Retry.MaxAttempts)
	var lastResponse *http.Response
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if ctx.Err() != nil {
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			break
		}
		// The route rotates with the attempt, so a retry after a proxy failure
		// does not walk straight back into the same dead exit.
		proxy := g.embeddingsProxy(attempt - 1)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			break
		}
		req.Header.Set("Content-Type", "application/json")
		if g.cfg.Embeddings.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+g.cfg.Embeddings.APIKey)
		}
		if proxy.pool {
			req.Close = true
		}
		started := time.Now()
		resp, err := doInferenceAttempt(proxy.client, req, 0, g.attemptTimeout)
		g.recordUpstreamAttempt(ctx, route, ids, attempt, externalEmbeddingsChannel, externalEmbeddingsChannel, false, proxy, resp, err, time.Since(started))
		if err == nil && (resp.StatusCode/100 == 2 || !retryableEmbeddingStatus(resp.StatusCode)) {
			g.writeEmbeddingsResponse(w, resp, ids, meta)
			return
		}
		if resp != nil {
			if lastResponse != nil {
				httpx.DrainAndClose(lastResponse.Body)
			}
			lastResponse = resp
		}
		lastErr = err
		g.logger.Debug("embeddings upstream attempt failed", "component", "upstream", "event", "embeddings_attempt_failed", "request_id", ids.Request, "attempt", attempt, "endpoint", config.RedactURL(endpoint), "error", err)
	}
	if lastResponse != nil {
		g.writeEmbeddingsResponse(w, lastResponse, ids, meta)
		return
	}
	g.writeEmbeddingsFailure(w, r, ctx, lastErr, ids, meta, externalTier)
}

// writeEmbeddingsResponse relays an upstream answer: a 2xx body is buffered so
// token usage can be recorded, and an error status keeps the upstream's own
// error envelope instead of being rewritten.
func (g *Gateway) writeEmbeddingsResponse(w http.ResponseWriter, resp *http.Response, ids identity.RequestIDs, meta *telemetry.RequestMeta) {
	defer resp.Body.Close()
	w.Header().Set("x-request-id", ids.Request)
	if resp.StatusCode/100 != 2 {
		copyErrorResponse(w, wire.Embeddings, resp, ids.Request)
		return
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxEmbeddingResponse+1))
	if err != nil {
		wire.WriteError(w, wire.Embeddings, http.StatusBadGateway, "failed to read upstream response", "upstream_error", ids.Request)
		return
	}
	// Reading one byte past the limit is what makes truncation detectable: a
	// silently shortened JSON body would otherwise reach the client as a 200.
	if len(responseBody) > maxEmbeddingResponse {
		wire.WriteError(w, wire.Embeddings, http.StatusBadGateway, "upstream embeddings response exceeds the 64 MiB relay limit", "upstream_error", ids.Request)
		return
	}
	if usage, reported := wire.ResponseUsage(wire.Embeddings, responseBody); meta != nil {
		meta.Usage, meta.UsageReported = usage, reported
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

// writeEmbeddingsFailure maps a routing or transport error to the same statuses
// the message endpoints use, so clients see one consistent failure model.
func (g *Gateway) writeEmbeddingsFailure(w http.ResponseWriter, r *http.Request, ctx context.Context, err error, ids identity.RequestIDs, meta *telemetry.RequestMeta, tier config.Tier) {
	keyID, channel, anonymous := requestCredential(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && wire.ClientCanceled(r.Context(), err) {
		g.logger.Debug("client canceled before upstream response", "component", "upstream", "event", "request_canceled", "request_id", ids.Request, "tier", tier, "key_id", keyID, "channel", channel, "anonymous", anonymous)
		if meta != nil {
			meta.Outcome = "client_canceled"
		}
		wire.WriteError(w, wire.Embeddings, statusClientClosedRequest, "client closed request", "client_canceled", ids.Request)
		return
	}
	g.logger.Warn("all upstream attempts failed", "component", "upstream", "event", "request_failed", "request_id", ids.Request, "tier", tier, "key_id", keyID, "channel", channel, "anonymous", anonymous, "error", err)
	if errors.Is(err, errProxyUnavailable) {
		wire.WriteError(w, wire.Embeddings, http.StatusServiceUnavailable, proxyUnavailableMessage, "proxy_error", ids.Request)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		wire.WriteError(w, wire.Embeddings, http.StatusGatewayTimeout, "upstream request timed out", "upstream_timeout", ids.Request)
		return
	}
	wire.WriteError(w, wire.Embeddings, http.StatusBadGateway, "all upstream attempts failed", "upstream_error", ids.Request)
}

// embeddingsRoute builds the tier plan for an embeddings request: the preferred
// tier first, then the other one when it has keys.
func (g *Gateway) embeddingsRoute(model string) (models.Route, error) {
	order := []config.Tier{config.TierGo, config.TierZen}
	if g.cfg.Prefer == config.TierZen {
		order = []config.Tier{config.TierZen, config.TierGo}
	}
	tiers := make([]config.Tier, 0, len(order))
	for _, tier := range order {
		switch {
		case tier == config.TierGo && g.goNodes.Len() > 0:
			tiers = append(tiers, tier)
		case tier == config.TierZen && g.zenNodes.Len() > 0:
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		// With no key at all, the public credential is the only lane left. It
		// is used only in that case: a keyed deployment should not pay an
		// anonymous attempt first, because eligibility cannot be judged for a
		// model the catalog does not know.
		if g.cfg.Anonymous && g.anonymous.Len() > 0 {
			return models.Route{
				ID: model, Tier: config.TierZen, Protocol: wire.Embeddings, Anonymous: true,
				Protocols: map[config.Tier]wire.Protocol{config.TierZen: wire.Embeddings},
			}, nil
		}
		return models.Route{}, errors.New("no Zen or Go key is configured to serve embeddings")
	}
	protocols := make(map[config.Tier]wire.Protocol, len(tiers))
	for _, tier := range tiers {
		protocols[tier] = wire.Embeddings
	}
	return models.Route{ID: model, Tier: tiers[0], Protocol: wire.Embeddings, Protocols: protocols, KeyTiers: tiers}, nil
}

// embeddingsProxy returns the proxy route an external embeddings origin is
// reached through. attempt rotates the starting position so successive retries
// prefer different exits, and a healthy route always wins over an unhealthy one.
func (g *Gateway) embeddingsProxy(attempt int) *proxyTransport {
	items := g.transports.items
	if len(items) == 0 {
		return nil
	}
	for offset := range items {
		item := items[(attempt+offset)%len(items)]
		if item.healthy.Load() {
			return item
		}
	}
	return items[attempt%len(items)]
}

// externalEmbeddingsEndpoint appends /embeddings to a configured base unless it
// already names the endpoint, so both https://host/v1 and
// https://host/v1/embeddings work.
func externalEmbeddingsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(base), "/embeddings") {
		return base
	}
	return base + "/embeddings"
}

// retryableEmbeddingStatus reports whether an error status from the external
// embeddings origin is worth another attempt: throttling and server errors are,
// a rejected request is not.
func retryableEmbeddingStatus(status int) bool {
	return status == http.StatusTooManyRequests || status/100 == 5
}

// isEmbeddingModel reports whether a model ID looks like an embedding model.
// It only decides whether /v1/models advertises the embeddings endpoint; the
// endpoint itself accepts any ID and lets the upstream judge.
func isEmbeddingModel(id string) bool {
	return strings.Contains(strings.ToLower(id), "embed")
}
