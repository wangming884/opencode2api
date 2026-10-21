// Package gateway routes inference requests through managed upstream pools.
//
// Upstreams fall into two lanes, named after the OpenCode client generation
// whose request shape they expect, not after their URL path: both lanes
// append the /v1/... route segment. The v1 lane is the https://opencode.ai/zen
// origin (models.V1Base, also the default upstream.zen), which
// models.IsV1Upstream detects. It expects the OpenCode 1.x client shape: the
// 1.x User-Agent, the x-opencode-request header, a git-commit-shaped project
// and, on the free tier, the single "read" tool. The v2 lane is the /inference
// origin (upstream.anonymous) and the Go plan's /zen/go origin, which expect
// the OpenCode 2.x CLI shape. The anonymous free tier tries the configured v2
// /inference origin first and falls back to the v1 origin.
package gateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/identity"
	"opencode2api/internal/jsonutil"
	"opencode2api/internal/limits"
	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
	"opencode2api/internal/rotation"
	"opencode2api/internal/telemetry"
)

const maxRequestBody = 32 << 20

// anonymousZenKey is the shared credential the anonymous free tier sends as
// its API key.
const anonymousZenKey = "public"

// attemptDeadline returns the per-attempt deadline, capped at the request
// timeout. It is zero unless attempt_timeout_seconds is set: AttemptTimeout
// would otherwise fall back to the request timeout, which the request context
// already enforces.
func attemptDeadline(cfg config.Config) time.Duration {
	if cfg.Performance.AttemptTimeoutSeconds <= 0 {
		return 0
	}
	return cfg.Performance.AttemptTimeout(time.Duration(cfg.Retry.TimeoutSeconds) * time.Second)
}

// statusClientClosedRequest is nginx's 499: the client went away before the
// gateway could answer, so the failure is neither upstream nor ours.
const statusClientClosedRequest = 499

// proxyUnavailableMessage is the 503 message sent when the error a request
// finally failed with is tagged errProxyUnavailable: its last attempt never
// reached the upstream because the proxy route failed, or the anonymous lane
// had no proxy to try while none was healthy. Only that final error is
// inspected; earlier attempts may have failed for other reasons. The message
// points operators at their proxies instead of the upstream.
const proxyUnavailableMessage = "all proxies are unavailable; the upstream could not be reached"

// Gateway serves the local inference API for one configuration. It owns the
// proxy transports and the key and anonymous node pools built from that
// config; applying a new config builds a new Gateway (see RuntimeController)
// instead of mutating this one.
type Gateway struct {
	rotation *rotation.Manager
	// limiter holds the per-server-key counters. It is shared with every other
	// gateway built from the same runtime controller, so applying a
	// configuration does not hand each key a fresh quota.
	limiter      *limits.Manager
	cfg          config.Config
	logger       *slog.Logger
	transports   *proxyTransports
	zenNodes     *upstreamNodes
	goNodes      *upstreamNodes
	anonymous    *anonymousNodes
	catalog      *models.ModelCatalog
	monitor      *telemetry.Metrics
	availability *models.AvailabilityRegistry
	// attemptTimeout bounds a whole upstream attempt (dial + write + headers
	// and, when the first-event wait is enabled, the first event), not just
	// the header wait. Zero arms no per-attempt deadline: each attempt is then
	// bounded only by the request-level budget.
	attemptTimeout time.Duration
}

// New builds a Gateway for cfg: one proxy transport per runtime proxy (a
// single direct transport when none is configured), Zen and Go key pools
// bound to those transports, the anonymous node pool and a model catalog. Its
// rotation manager is not persisted; RuntimeController replaces it with the
// shared one.
func New(cfg config.Config, logger *slog.Logger, monitor *telemetry.Metrics) (*Gateway, error) {
	cfg.Rotation.Defaults()
	rotationManager, err := rotation.New("", cfg.Rotation)
	if err != nil {
		return nil, err
	}
	transports, err := transportPool(cfg.RuntimeProxies(), cfg.Performance, cfg.Performance.AttemptTimeout(time.Duration(cfg.Retry.TimeoutSeconds)*time.Second))
	if err != nil {
		return nil, err
	}
	cooldown := time.Duration(cfg.Performance.FailureCooldownSeconds) * time.Second
	zenNodes, err := nodePool(cfg.ZenKeys, transports, cooldown)
	if err != nil {
		return nil, fmt.Errorf("zen node pool: %w", err)
	}
	goNodes, err := nodePool(cfg.GoKeys, transports, cooldown)
	if err != nil {
		return nil, fmt.Errorf("go node pool: %w", err)
	}
	catalog := models.Catalog(cfg.Prefer, cfg.Models.Protocols)
	catalog.SetRefreshInterval(time.Duration(cfg.Models.RefreshSeconds) * time.Second)
	catalog.SetFreeOnly(cfg.Models.FreeOnly)
	return &Gateway{
		cfg:            cfg,
		rotation:       rotationManager,
		limiter:        limits.New(),
		logger:         logger,
		transports:     transports,
		zenNodes:       zenNodes,
		goNodes:        goNodes,
		anonymous:      anonymousPool(cfg.Anonymous, transports, cooldown),
		catalog:        catalog,
		monitor:        monitor,
		attemptTimeout: attemptDeadline(cfg),
	}, nil
}

// Handler returns the gateway's HTTP API: the server-key authenticated /v1
// model, inference and System One routes plus the open /healthz, wrapped in
// panic recovery.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", g.authenticate(g.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", g.authenticate(g.handleInference(wire.Chat)))
	mux.HandleFunc("POST /v1/responses", g.authenticate(g.handleInference(wire.Responses)))
	mux.HandleFunc("POST /v1/messages", g.authenticate(g.handleInference(wire.Anthropic)))
	mux.HandleFunc("POST /v1/systemone", g.authenticate(g.handleSystemOne))
	mux.HandleFunc("POST /v1/embeddings", g.authenticate(g.handleEmbeddings))
	mux.HandleFunc("GET /metrics", g.authenticate(g.handleMetrics))
	mux.HandleFunc("GET /healthz", g.handleHealth)
	return telemetry.Recover(g.logger, mux)
}

// authenticate admits a request whose x-api-key header or Bearer token
// matches an enabled server key, and stores the caller's fingerprint in the
// request context and telemetry metadata. Anything else gets a 401 shaped for
// the endpoint: Anthropic for /v1/messages, Chat otherwise.
func (g *Gateway) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidates := []string{strings.TrimSpace(r.Header.Get("x-api-key"))}
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			candidates = append(candidates, strings.TrimSpace(auth[7:]))
		}
		matched := ""
		for _, key := range g.cfg.ServerKeys {
			if !g.cfg.ServerKeyEnabled(key) {
				continue
			}
			for _, candidate := range candidates {
				if len(candidate) == len(key) && subtle.ConstantTimeCompare([]byte(candidate), []byte(key)) == 1 {
					matched = key
				}
			}
		}
		if matched == "" {
			wire.WriteError(w, requestProtocol(r.URL.Path), http.StatusUnauthorized, "invalid local API key", "authentication_error", "")
			return
		}
		// The Playground borrows the first enabled key to pass this check, so
		// its traffic gets its own bucket instead of being billed to that key.
		caller := config.Fingerprint(matched)
		if isDiagnosticRequest(r.Context()) {
			caller = telemetry.PlaygroundServerKey
		} else if g.cfg.ServerKeyExpired(caller) {
			// Expiry is checked here rather than in the match loop so the
			// caller is told the key ran out instead of that it is unknown.
			wire.WriteError(w, requestProtocol(r.URL.Path), http.StatusUnauthorized, "this API key has expired", "authentication_error", "")
			return
		}
		// Only inference is metered: discovery, health and metrics probes are
		// not part of a key's traffic budget.
		if meteredPath(r.URL.Path) {
			decision := g.limiter.Allow(caller, g.cfg.ServerKeyLimitsOf(caller))
			if !decision.Allowed {
				g.logger.Debug("request refused by key policy", "component", "limits", "event", "key_limited", "path", r.URL.Path, "server_key_id", caller, "reason", decision.Reason, "retry_after", decision.RetryAfter)
				w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfter))
				wire.WriteError(w, requestProtocol(r.URL.Path), http.StatusTooManyRequests, decision.Message, "rate_limit_exceeded", "")
				return
			}
		}
		if meta := telemetry.MetaFromRequest(r); meta != nil {
			meta.ServerKeyID = caller
		}
		next(w, r.WithContext(context.WithValue(r.Context(), callerContextKey{}, caller)))
	}
}

// requestProtocol names the wire protocol of a client path so an error is
// shaped for the endpoint that produced it.
func requestProtocol(path string) wire.Protocol {
	if path == "/v1/messages" {
		return wire.Anthropic
	}
	return wire.Chat
}

// meteredPath reports whether a path counts toward a key's traffic policy.
func meteredPath(path string) bool {
	switch path {
	case "/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/systemone", "/v1/embeddings":
		return true
	}
	return false
}

// finishRequest charges a finished request to its key's daily budget and prices
// it. It runs on the way out, so a streaming call is accounted once its last
// event arrived and meta carries the reported usage.
func (g *Gateway) finishRequest(r *http.Request) {
	meta := telemetry.MetaFromRequest(r)
	if meta == nil || !meta.UsageReported {
		return
	}
	tokens := int64(meta.Usage.Total)
	if tokens <= 0 {
		tokens = int64(meta.Usage.Input + meta.Usage.Output)
	}
	caller := callerScope(r.Context())
	if g.limiter != nil && caller != "" && caller != telemetry.PlaygroundServerKey {
		g.limiter.RecordUsage(caller, tokens)
	}
	meta.CostUSD = g.requestCost(meta.Model, meta.Tier, meta.Usage)
}

// requestCost prices one request from the catalog's per-million prices. A model
// without a published price costs zero rather than a guess.
func (g *Gateway) requestCost(model, tier string, usage wire.Usage) float64 {
	if g.catalog == nil || model == "" {
		return 0
	}
	// A channel label such as "external" is not a tier the catalog prices
	// against, so it falls back to the preferred tier: a model that is priced
	// at all is priced the same everywhere, and the alternative would be
	// reporting zero for every embeddings request.
	tierName := config.Tier(tier)
	if tierName != config.TierZen && tierName != config.TierGo {
		tierName = g.cfg.Prefer
	}
	return priceUsage(g.catalog.MetadataForTier(model, tierName).Cost, usage)
}

// priceUsage converts token counts to dollars. The input count already includes
// cache reads and writes, so those are priced at their own rate and only the
// remainder at the input rate; charging the whole input at the input rate would
// overstate the cost of a cached conversation by an order of magnitude.
//
// When the catalog publishes no cache price but the upstream still reports
// cached tokens, they fall back to the input rate: a cache discount is unknown,
// and pricing them at zero would silently understate the bill.
func priceUsage(cost *models.Cost, usage wire.Usage) float64 {
	if cost == nil {
		return 0
	}
	perMillion := func(price *float64, tokens int) float64 {
		if price == nil || tokens <= 0 {
			return 0
		}
		return *price * float64(tokens) / 1e6
	}
	cacheRead, cacheWrite := cost.CacheRead, cost.CacheWrite
	if cacheRead == nil {
		cacheRead = cost.Input
	}
	if cacheWrite == nil {
		cacheWrite = cost.Input
	}
	uncached := usage.Input - usage.Cached - usage.CacheCreation
	if uncached < 0 {
		uncached = 0
	}
	return perMillion(cost.Input, uncached) +
		perMillion(cacheRead, usage.Cached) +
		perMillion(cacheWrite, usage.CacheCreation) +
		perMillion(cost.Output, usage.Output)
}

type callerContextKey struct{}

// callerScope returns the authenticated caller that request identities are
// scoped to, so different server keys never share an upstream session.
func callerScope(ctx context.Context) string {
	caller, _ := ctx.Value(callerContextKey{}).(string)
	return caller
}

// aliasRouteError names the public model when a routing failure was caused by
// what it resolves to. Without it the caller reads about a model ID they never
// sent, which looks like a gateway bug rather than a stale alias.
func aliasRouteError(requested, served, reason string) string {
	if requested == served {
		return reason
	}
	return fmt.Sprintf("model %q is aliased to %q: %s", requested, served, reason)
}

// unavailableMessage explains a model the catalog cannot serve. A model ID the
// catalog has never seen is a different problem from one it knows and cannot
// speak the protocol of, and only the first is fixed by checking the spelling.
func (g *Gateway) unavailableMessage(requested, served string) string {
	if g.catalog.Known(served) {
		return aliasRouteError(requested, served, "the model uses an upstream protocol that opencode2api does not expose")
	}
	if requested != served {
		return fmt.Sprintf("model %q is aliased to %q, which is not offered by the configured Zen or Go pools", requested, served)
	}
	return fmt.Sprintf("model %q is not offered by the configured Zen or Go pools", served)
}

// rewriteModelBody returns body with its model field replaced. The decoder
// keeps numbers as literals, so a large integer (seed, timestamp) survives the
// round trip instead of being re-encoded as a float.
func rewriteModelBody(body []byte, model string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("request body must be a JSON object: %w", err)
	}
	doc["model"] = model
	return json.Marshal(doc)
}

// handleInference serves a message endpoint whose wire protocol is external.
// A rotation group alias is handed to handleRotation. Any other model is
// routed through the catalog, encoded per upstream tier, sent through
// doUpstream and translated back into external, streaming or not.
type requestShapeContextKey struct{}

type requestShape struct {
	shaped    bool
	toolAlias string
}

func setRequestShape(ctx context.Context, shaped bool, toolAlias string) {
	if state, ok := ctx.Value(requestShapeContextKey{}).(*requestShape); ok {
		state.shaped, state.toolAlias = shaped, toolAlias
	}
	if meta := telemetry.MetaFromContext(ctx); meta != nil {
		meta.Shaped, meta.ToolAlias = shaped, toolAlias
	}
}

func requestShapeFrom(ctx context.Context) requestShape {
	if state, ok := ctx.Value(requestShapeContextKey{}).(*requestShape); ok && state != nil {
		return *state
	}
	return requestShape{}
}

func (g *Gateway) handleInference(external wire.Protocol) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), requestShapeContextKey{}, &requestShape{}))
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
		if err != nil {
			wire.WriteError(w, external, http.StatusBadRequest, "request body is too large or unreadable", "invalid_request_error", "")
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			wire.WriteError(w, external, http.StatusBadRequest, "request body must be a JSON object", "invalid_request_error", "")
			return
		}
		model := jsonutil.StringAt(payload, "model")
		meta := telemetry.MetaFromRequest(r)
		if meta != nil {
			meta.Model = model
		}
		// The budget is charged once the answer is known, so this runs on the
		// way out, after a stream has finished and meta carries its usage.
		defer g.finishRequest(r)
		if model == "" {
			wire.WriteError(w, external, http.StatusBadRequest, "model is required", "invalid_request_error", "model")
			return
		}
		// models.aliases maps a public name to the model that serves it. The
		// public name is what the caller is billed and logged under; the target
		// is what routing, pricing, reasoning overrides and the upstream see.
		requested := model
		if target := g.cfg.ResolveModel(model); target != model {
			model = target
			payload["model"] = target
			if meta != nil {
				meta.ModelAlias = requested
				meta.Model = target
			}
		}
		if !g.cfg.ServerKeyAllowsRequest(callerScope(r.Context()), requested, model) {
			// The key policy refused this model, not the caller's payload, so
			// the request gives its rate slot back instead of spending the
			// quota on requests that can never succeed.
			g.limiter.Release(callerScope(r.Context()))
			wire.WriteError(w, external, http.StatusForbidden, fmt.Sprintf("model %q is not allowed for this API key", requested), "permission_error", "model")
			return
		}
		if id := g.rotation.GroupID(model); id != "" {
			// The public name is what the reply must report, so an alias that
			// points at a group is not disclosed as the group either.
			g.handleRotation(w, r, external, payload, id, requested)
			return
		}
		if !g.catalog.Supported(model) {
			wire.WriteError(w, external, http.StatusBadRequest, g.unavailableMessage(requested, model), "invalid_request_error", "model")
			return
		}
		route, err := g.catalog.Route(model, len(g.cfg.ZenKeys) > 0, len(g.cfg.GoKeys) > 0, g.cfg.Anonymous)
		if override, selected := debugKeyOverrideFrom(r.Context()); selected {
			// A per-key diagnostic must not silently be served by another key,
			// another tier or the anonymous lane.
			route, err = g.catalog.RouteForTier(model, override.Tier, len(g.cfg.ZenKeys) > 0, len(g.cfg.GoKeys) > 0)
		}
		if err != nil {
			wire.WriteError(w, external, http.StatusBadRequest, aliasRouteError(requested, model, err.Error()), "invalid_request_error", "model")
			return
		}
		if meta != nil {
			meta.Tier = string(route.Tier)
			meta.Protocol = route.Protocol
		}
		// A System One model has no message-shaped equivalent, so a decision
		// payload submitted on a message endpoint is relayed verbatim instead of
		// being converted. This keeps the model reachable for clients that can
		// only address /v1/chat/completions or /v1/responses — for example a
		// gateway whose OpenAI platform pins every request to Responses.
		if route.Protocol == wire.SystemOne {
			if len(jsonutil.MapAt(payload, "questions")) == 0 || payload["state"] == nil {
				wire.WriteError(w, external, http.StatusBadRequest, "this model uses System One decisions, not chat; send state and typed questions to /v1/systemone", "invalid_request_error", "model")
				return
			}
			// The relay is verbatim, so an alias has to be applied to the bytes
			// themselves; the message endpoints re-encode from payload instead.
			relayBody := body
			if requested != model {
				rewritten, rewriteErr := rewriteModelBody(body, model)
				if rewriteErr != nil {
					wire.WriteError(w, external, http.StatusBadRequest, rewriteErr.Error(), "invalid_request_error", "model")
					return
				}
				relayBody = rewritten
			}
			g.forwardSystemOne(w, r, relayBody, payload, model, requested, route)
			return
		}
		bodies, err := g.prepareRouteBodies(external, route, payload)
		if err != nil {
			wire.WriteError(w, external, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
			return
		}
		ids := identity.DeriveRequestIDs(r, payload, callerScope(r.Context()))
		if meta != nil {
			meta.Request = ids.Request
			meta.Session = ids.Session
		}
		stream := jsonutil.BoolAt(payload, "stream")
		requestCtx, cancel := context.WithTimeout(r.Context(), time.Duration(g.cfg.Retry.TimeoutSeconds)*time.Second)
		defer cancel()
		resp, upstreamRoute, err := g.doUpstream(requestCtx, route, bodies, ids)
		if err != nil {
			finalTier := route.Tier
			if meta != nil && meta.Tier != "" {
				finalTier = config.Tier(meta.Tier)
			}
			keyID, channel, anonymous := requestCredential(requestCtx)
			// A downstream disconnect is not an upstream failure: report the
			// real cause so telemetry and clients see a cancel, not a 502.
			if !errors.Is(err, context.DeadlineExceeded) && wire.ClientCanceled(r.Context(), err) {
				g.logger.Debug("client canceled before upstream response", "component", "upstream", "event", "request_canceled", "request_id", ids.Request, "tier", finalTier, "key_id", keyID, "channel", channel, "anonymous", anonymous)
				if meta != nil {
					meta.Outcome = "client_canceled"
				}
				wire.WriteError(w, external, statusClientClosedRequest, "client closed request", "client_canceled", ids.Request)
				return
			}
			g.logger.Warn("all upstream attempts failed", "component", "upstream", "event", "request_failed", "request_id", ids.Request, "tier", finalTier, "key_id", keyID, "channel", channel, "anonymous", anonymous, "error", err)
			// The final attempt never reached the upstream because its proxy
			// route failed; report that instead of blaming the upstream with
			// a 502.
			if errors.Is(err, errProxyUnavailable) {
				wire.WriteError(w, external, http.StatusServiceUnavailable, proxyUnavailableMessage, "proxy_error", ids.Request)
				return
			}
			// An exhausted request budget is a timeout, not a bad gateway: the
			// distinction matters to clients that retry on 502.
			if errors.Is(err, context.DeadlineExceeded) {
				wire.WriteError(w, external, http.StatusGatewayTimeout, "upstream request timed out", "upstream_timeout", ids.Request)
				return
			}
			wire.WriteError(w, external, http.StatusBadGateway, "all upstream attempts failed", "upstream_error", ids.Request)
			return
		}
		defer resp.Body.Close()
		if meta != nil {
			meta.Tier = string(upstreamRoute.Tier)
			meta.Protocol = upstreamRoute.Protocol
		}
		w.Header().Set("x-request-id", ids.Request)
		if requested != model {
			// The caller asked for a public name; naming the model that served
			// it in a header keeps the alias invisible without hiding the truth
			// from an operator reading the response.
			w.Header().Set("X-Resolved-Model", model)
		}
		if resp.StatusCode/100 != 2 {
			copyErrorResponse(w, external, resp, ids.Request)
			return
		}
		shape := requestShapeFrom(r.Context())
		toolAlias := shape.toolAlias
		if stream {
			if meta != nil {
				meta.Stream = true
			}
			if g.monitor != nil {
				g.monitor.BeginStream()
				defer g.monitor.EndStream()
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(resp.StatusCode)
			streamBody := wire.RenameStreamReader(resp.Body, upstreamRoute.Protocol, v1ReadTool, toolAlias)
			var usage wire.Usage
			var usageReported bool
			if external == upstreamRoute.Protocol {
				// A same-protocol stream is copied verbatim, so the alias has to
				// be rewritten frame by frame on the way out.
				usage, usageReported, err = wire.ForwardStream(r.Context(), w, wire.RenameStreamModel(streamBody, upstreamRoute.Protocol, model, requested), upstreamRoute.Protocol, model)
			} else {
				// The transcoder builds every frame itself and stamps the model
				// it is given, so it emits the public name directly.
				usage, usageReported, err = wire.TranscodeStream(r.Context(), w, streamBody, upstreamRoute.Protocol, external, requested)
			}
			if meta != nil {
				meta.Usage, meta.UsageReported = usage, usageReported
				if err != nil {
					meta.Outcome = "stream_error"
					if wire.ClientCanceled(r.Context(), err) {
						meta.Outcome = "client_canceled"
					}
				}
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				g.logger.Warn("downstream stream ended with an error", "component", "stream", "event", "stream_failed", "request_id", ids.Request, "model", model, "tier", upstreamRoute.Tier, "error", err)
			}
			return
		}
		responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			wire.WriteError(w, external, http.StatusBadGateway, "failed to read upstream response", "upstream_error", ids.Request)
			return
		}
		if upstreamRoute.Anonymous || shape.shaped {
			// The anonymous lane, and a key-tier request shaped like it, is
			// force-streamed upstream (see prepareAnonymousBody); collapse
			// the events back into the single document this non-streaming
			// client asked for.
			collapsed, err := wire.CollapseStream(bytes.NewReader(responseBody), upstreamRoute.Protocol, model)
			if err != nil {
				g.logger.Warn("forced stream collapse failed", "component", "conversion", "event", "stream_collapse_failed", "request_id", ids.Request, "model", model, "anonymous", upstreamRoute.Anonymous, "source_protocol", upstreamRoute.Protocol, "error", err)
				wire.WriteError(w, external, http.StatusBadGateway, "unsupported upstream response", "upstream_error", ids.Request)
				return
			}
			responseBody = collapsed
		}
		responseBody = wire.RenameResponseTool(responseBody, upstreamRoute.Protocol, v1ReadTool, toolAlias)
		if usage, reported := wire.ResponseUsage(upstreamRoute.Protocol, responseBody); meta != nil {
			meta.Usage, meta.UsageReported = usage, reported
		}
		if external != upstreamRoute.Protocol {
			responseBody, err = wire.ConvertResponse(upstreamRoute.Protocol, external, responseBody)
			if err != nil {
				g.logger.Warn("response protocol conversion failed", "component", "conversion", "event", "response_conversion_failed", "request_id", ids.Request, "model", model, "source_protocol", upstreamRoute.Protocol, "target_protocol", external, "error", err)
				wire.WriteError(w, external, http.StatusBadGateway, "unsupported upstream response", "upstream_error", ids.Request)
				return
			}
		}
		// The body names the model that answered; report the name the caller
		// asked for instead, matching what the rotation path already does.
		responseBody = wire.RenameResponseModel(responseBody, external, model, requested)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(responseBody)
	}
}

// handleSystemOne serves System One decision requests on their own endpoint.
// Only models whose upstream protocol is System One can be served here; a chat
// or responses model belongs on its own endpoint, where the bridge can convert
// it.
func (g *Gateway) handleSystemOne(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, "request body is too large or unreadable", "invalid_request_error", "")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, "request body must be a JSON object", "invalid_request_error", "")
		return
	}
	model := jsonutil.StringAt(payload, "model")
	meta := telemetry.MetaFromRequest(r)
	if meta != nil {
		meta.Model = model
	}
	if model == "" {
		wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, "model is required", "invalid_request_error", "model")
		return
	}
	// A decision relay is metered like the message endpoints, so it settles
	// through the same path: without this, its tokens never reach the key's
	// daily budget and its spend is reported as zero.
	defer g.finishRequest(r)
	requested := model
	if target := g.cfg.ResolveModel(model); target != model {
		rewritten, rewriteErr := rewriteModelBody(body, target)
		if rewriteErr != nil {
			wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, rewriteErr.Error(), "invalid_request_error", "model")
			return
		}
		model, body = target, rewritten
		payload["model"] = target
		if meta != nil {
			meta.ModelAlias = requested
			meta.Model = target
		}
	}
	// The key's model policy applies here exactly as it does on the message
	// endpoints; this route used to reach any System One model with any key.
	if !g.cfg.ServerKeyAllowsRequest(callerScope(r.Context()), requested, model) {
		g.limiter.Release(callerScope(r.Context()))
		wire.WriteError(w, wire.SystemOne, http.StatusForbidden, fmt.Sprintf("model %q is not allowed for this API key", requested), "permission_error", "model")
		return
	}
	if !g.catalog.Supported(model) {
		wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, g.unavailableMessage(requested, model), "invalid_request_error", "model")
		return
	}
	route, err := g.catalog.Route(model, len(g.cfg.ZenKeys) > 0, len(g.cfg.GoKeys) > 0, g.cfg.Anonymous)
	if err != nil {
		wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, aliasRouteError(requested, model, err.Error()), "invalid_request_error", "model")
		return
	}
	if route.Protocol != wire.SystemOne {
		wire.WriteError(w, wire.SystemOne, http.StatusBadRequest, aliasRouteError(requested, model, fmt.Sprintf("the model does not use the %s protocol", wire.SystemOne)), "invalid_request_error", "model")
		return
	}
	g.forwardSystemOne(w, r, body, payload, model, requested, route)
}

// forwardSystemOne relays a System One decision payload verbatim to the
// upstream systemone endpoint and returns the typed answer document unchanged.
// The payload pairs a free-form state with typed questions, which no
// message-shaped upstream protocol accepts, so it is never translated. Answers
// are non-streaming today; a streaming upstream reply is still relayed.
func (g *Gateway) forwardSystemOne(w http.ResponseWriter, r *http.Request, body []byte, payload map[string]any, model, requested string, route models.Route) {
	meta := telemetry.MetaFromRequest(r)
	if meta != nil {
		meta.Model = model
		meta.Tier = string(route.Tier)
		meta.Protocol = route.Protocol
	}
	// One verbatim body serves every tier: a decision payload has no per-tier
	// encoding, so no protocol conversion is attempted.
	bodies := make(map[config.Tier][]byte, len(route.KeyTiers)+1)
	bodies[route.Tier] = body
	for _, tier := range route.KeyTiers {
		bodies[tier] = body
	}
	ids := identity.DeriveRequestIDs(r, payload, callerScope(r.Context()))
	if meta != nil {
		meta.Request = ids.Request
		meta.Session = ids.Session
	}
	requestCtx, cancel := context.WithTimeout(r.Context(), time.Duration(g.cfg.Retry.TimeoutSeconds)*time.Second)
	defer cancel()
	resp, upstreamRoute, err := g.doUpstream(requestCtx, route, bodies, ids)
	if err != nil {
		finalTier := route.Tier
		if meta != nil && meta.Tier != "" {
			finalTier = config.Tier(meta.Tier)
		}
		keyID, channel, anonymous := requestCredential(requestCtx)
		if !errors.Is(err, context.DeadlineExceeded) && wire.ClientCanceled(r.Context(), err) {
			g.logger.Debug("client canceled before upstream response", "component", "upstream", "event", "request_canceled", "request_id", ids.Request, "tier", finalTier, "key_id", keyID, "channel", channel, "anonymous", anonymous)
			if meta != nil {
				meta.Outcome = "client_canceled"
			}
			wire.WriteError(w, wire.SystemOne, statusClientClosedRequest, "client closed request", "client_canceled", ids.Request)
			return
		}
		g.logger.Warn("all upstream attempts failed", "component", "upstream", "event", "request_failed", "request_id", ids.Request, "tier", finalTier, "key_id", keyID, "channel", channel, "anonymous", anonymous, "error", err)
		if errors.Is(err, errProxyUnavailable) {
			wire.WriteError(w, wire.SystemOne, http.StatusServiceUnavailable, proxyUnavailableMessage, "proxy_error", ids.Request)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			wire.WriteError(w, wire.SystemOne, http.StatusGatewayTimeout, "upstream request timed out", "upstream_timeout", ids.Request)
			return
		}
		wire.WriteError(w, wire.SystemOne, http.StatusBadGateway, "all upstream attempts failed", "upstream_error", ids.Request)
		return
	}
	defer resp.Body.Close()
	if meta != nil {
		meta.Tier = string(upstreamRoute.Tier)
		meta.Protocol = upstreamRoute.Protocol
	}
	w.Header().Set("x-request-id", ids.Request)
	if requested != model {
		w.Header().Set("X-Resolved-Model", model)
	}
	if resp.StatusCode/100 != 2 {
		copyErrorResponse(w, wire.SystemOne, resp, ids.Request)
		return
	}
	if contentType := resp.Header.Get("Content-Type"); strings.HasPrefix(contentType, "text/event-stream") {
		if meta != nil {
			meta.Stream = true
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		wire.WriteError(w, wire.SystemOne, http.StatusBadGateway, "failed to read upstream response", "upstream_error", ids.Request)
		return
	}
	if usage, reported := wire.ResponseUsage(upstreamRoute.Protocol, responseBody); meta != nil {
		meta.Usage, meta.UsageReported = usage, reported
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

// prepareRouteBodies encodes input once for each key tier the route may use,
// the preferred tier first and then its KeyTiers, in that tier's upstream
// protocol and with any forced reasoning effort applied. A fallback tier the
// request cannot be converted for is skipped; only a conversion failure on
// the preferred tier is an error.
func (g *Gateway) prepareRouteBodies(from wire.Protocol, route models.Route, input map[string]any) (map[config.Tier][]byte, error) {
	tiers := make([]config.Tier, 0, len(route.KeyTiers)+1)
	seen := make(map[config.Tier]bool, len(route.KeyTiers)+1)
	addTier := func(tier config.Tier) {
		if tier != config.TierZen && tier != config.TierGo || seen[tier] {
			return
		}
		seen[tier] = true
		tiers = append(tiers, tier)
	}
	addTier(route.Tier)
	for _, tier := range route.KeyTiers {
		addTier(tier)
	}
	if len(tiers) == 0 {
		return nil, errors.New("no usable upstream tier")
	}
	bodies := make(map[config.Tier][]byte, len(tiers))
	for _, tier := range tiers {
		protocol := route.ProtocolFor(tier)
		baseURL := g.cfg.Upstream.Zen
		if tier == config.TierGo {
			baseURL = g.cfg.Upstream.Go
		}
		upstreamPayload, err := wire.PrepareRequest(from, protocol, input, baseURL)
		if err != nil {
			if tier != route.Tier {
				// A fallback tier may use a stricter wire format than the
				// preferred tier. Do not reject a request before the preferred
				// upstream has even been tried; that tier is attempted only if
				// the request actually falls back.
				g.logger.Debug("skipping fallback tier with incompatible request shape", "component", "conversion", "event", "fallback_tier_skipped", "model", jsonutil.StringAt(input, "model"), "tier", tier, "protocol", protocol, "error", err)
				continue
			}
			return nil, fmt.Errorf("prepare %s upstream request: %w", tier, err)
		}
		if effort := g.resolvedEffort(jsonutil.StringAt(upstreamPayload, "model"), tier, protocol); effort != "" {
			wire.ForcedEffort(protocol, upstreamPayload, effort)
		}
		encoded, err := json.Marshal(upstreamPayload)
		if err != nil {
			return nil, errors.New("request contains unsupported JSON values")
		}
		bodies[tier] = encoded
	}
	return bodies, nil
}

// copyErrorResponse relays a non-2xx upstream reply as an error in the
// client's protocol. It keeps the status and any Retry-After, and takes the
// message from a JSON body's error.message or message, falling back to the
// status text.
func copyErrorResponse(w http.ResponseWriter, protocol wire.Protocol, resp *http.Response, requestID string) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	message := http.StatusText(resp.StatusCode)
	var value map[string]any
	if json.Unmarshal(body, &value) == nil {
		message = jsonutil.FirstString(jsonutil.StringAt(value, "error", "message"), jsonutil.StringAt(value, "message"), message)
	}
	wire.WriteError(w, protocol, resp.StatusCode, message, "upstream_error", requestID)
}
