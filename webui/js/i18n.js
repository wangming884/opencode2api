// Interface language handling for the management console.
//
// The console ships with Simplified Chinese as its original language. That copy
// stays in index.html and the page modules and is used whenever the active
// locale has no entry in DICTIONARY, so switching languages never rewrites the
// originals.
//
// Usage from markup (static text is re-applied on every switch):
//   <h2 data-i18n="config.title">…original…</h2>
//   <input data-i18n-attr="placeholder:logs.search" />
// Usage from a page module:
//   import { t } from "../i18n.js";
//   t("配置已保存", "toast.config.saved")
//   t(`${n} 个`, "apikeys.count", n)
const STORAGE_KEY = "opencode2api.ui.lang";
const DEFAULT_LOCALE = "zh-CN";
// Flags are deliberately not used here: Windows ships no colour emoji font for
// regional-indicator pairs, so 🇺🇸 renders as the two letters "us" and the
// picker looks broken. Short codes render identically everywhere, and each
// option still carries its full name as the accessible label.
const SHORT = { "zh-CN": "中文", en: "EN", vi: "VI" };
const SUPPORTED = ["zh-CN", "en", "vi"];
const LABELS = { "zh-CN": "简体中文", en: "English", vi: "Tiếng Việt" };
const DATE_LOCALES = { "zh-CN": "zh-CN", en: "en-US", vi: "vi-VN" };
const listeners = [];
// Snapshot of the authored copy, so switching back restores it untouched.
const originals = new WeakMap();

const DICTIONARY = {
    en: {
      "doc.title": "opencode2api · Operations Handbook",
      "lang.label": "Language",
      "a11y.skip": "Skip to main content",
      "a11y.nav": "Page navigation",
      "a11y.tabs.overview": "Overview views",
      "a11y.tabs.diagnostics": "Diagnostics views",
      "a11y.tabs.quotas": "Model views",
      "a11y.tabs.config": "Configuration sections",
      "a11y.logLevel": "Log level",

      "login.title": "Admin sign-in",
      "login.username": "Username",
      "login.password": "Password",
      "login.submit": "Enter the Operations Handbook",

      "nav.overview": "Operations desk",
      "nav.access": "Integration handbook",
      "nav.diagnostics": "Routing diagnostics",
      "nav.config": "Configuration center",
      "nav.account": "Account security",
      "nav.model-quotas": "Models and quotas",
      "nav.api-keys": "API Key management",
      "nav.rotation": "Model rotation",
      "nav.metrics": "Metrics",
      "nav.group.runtime": "Runtime",
      "nav.group.integration": "Integration",
      "nav.group.models": "Models",
      "nav.group.system": "System",
      "overview.tab.summary": "Summary",
      "overview.tab.usage": "Usage",
      "overview.checklist.title": "Readiness checks",
      "overview.checklist.caption": "State comes from the current configuration and live resources.",
      "diagnostics.tab.playground": "Request test",
      "diagnostics.tab.ledger": "Request ledger",
      "diagnostics.tab.logs": "Event stream",
      "diagnostics.tab.route": "Routing and eligibility",
      "quotas.tab.quota": "Plan quota",
      "quotas.tab.free": "Free models",
      "config.tab.service": "Service",
      "config.tab.keys": "Secrets and anonymous",
      "config.tab.proxies": "Proxies",
      "config.tab.upstream": "Upstream and models",
      "config.tab.reasoning": "Reasoning and performance",
      "config.tab.logging": "Logging",
      "usage.hour": "Last 60 minutes",

      "topbar.refresh": "Refresh now",
      "topbar.logout": "Sign out",

      "page.rotation.title": "Model rotation",
      "page.rotation.sub": "Shared current model, failover on error, and attempt history.",
      "page.overview.title": "Operations desk",
      "page.overview.sub": "The last hour, process totals, and current resources.",
      "page.access.title": "Integration handbook",
      "page.access.sub": "Chat, Responses, Anthropic, and SDK examples.",
      "page.diagnostics.title": "Routing diagnostics",
      "page.diagnostics.sub": "Metadata, model eligibility, and each upstream attempt.",
      "page.config.title": "Configuration center",
      "page.config.sub": "Validate, then save atomically and hot-apply the fields that can change.",
      "page.account.title": "Account security",
      "page.account.sub": "A successful update revokes every admin session.",
      "page.model-quotas.title": "Models and quotas",
      "page.model-quotas.sub": "Available models, remaining shared plan quota, and reset times.",
      "page.api-keys.title": "API Key management",
      "page.api-keys.sub": "A separate access key for each caller.",

      "availability.title": "Probe free models",
      "availability.refresh": "Refresh status",
      "availability.desc":
        "Probes models whose metadata name contains free or whose price is 0 and that have not been excluded. The anonymous channel is tried first, and a Zen Key only after that; a model is disabled once it is genuinely broken and retried after 24 hours. Transient errors do not add extra disabled models: an enabled model is retried after 1 hour, and a disabled one keeps its old reason and is retried after 24 hours. When a model is re-enabled or switched to manual control it stays enabled and is no longer probed automatically; when it switches back to automatic, the next scheduler tick schedules the probe. System One uses its own dedicated 1+1 decision question set. With the effort set to auto, the highest probed and verified effort is shown.",
      "availability.col.model": "Model",
      "availability.col.state": "State",
      "availability.col.channel": "Channel / result",
      "availability.col.control": "Control",
      "availability.col.effort": "Verified reasoning effort",
      "availability.col.checked": "Last probe",
      "availability.col.next": "Next probe",
      "availability.col.action": "Action",

      "overview.requests": "Requests in the last hour",
      "overview.success": "Success rate",
      "overview.p95": "P95 latency",
      "overview.active": "Active / streaming",
      "overview.hour": "Last 60 minutes",
      "overview.http2xx": "HTTP 2xx–3xx",
      "overview.endToEnd": "End-to-end milliseconds",
      "overview.concurrent": "Concurrent requests",
      "overview.cost": "Total spend",
      "overview.costNote": "Estimated from catalog prices",
      "overview.proxy.title": "Proxy nodes",
      "overview.proxy.col.node": "Node",
      "overview.proxy.col.health": "Health",
      "overview.proxy.col.binding": "Binding",
      "overview.keys.title": "Key pool",
      "overview.keys.col.tail": "Key tail",
      "overview.keys.col.tier": "Tier",
      "overview.keys.col.state": "State",
      "overview.distribution.title": "Request distribution",
      "overview.distribution.caption":
        "Aggregated over the last hour by endpoint, model, Tier, and status code.",

      "guide.step.serverkey": "Configure a Server Key",
      "guide.step.serverkey.desc": "Clients use the Server Key to reach the Gateway.",
      "guide.step.upstream": "Pick an upstream channel",
      "guide.step.upstream.desc": "At least one Zen / Go Key, or the anonymous channel enabled.",
      "guide.step.proxy": "Check the proxies",
      "guide.step.proxy.desc": "At least one proxy node is still healthy.",
      "guide.step.models": "Wait for the model catalog",
      "guide.done": "✓ Ready",
      "guide.todo": "○ Needs work",

      "access.base.title": "Base address",
      "access.base.caption": "The examples never contain a real Server Key.",
      "access.base.label": "Derived API Base",
      "access.model.label": "Example model",
      "access.protocols.title": "Three protocol endpoints",
      "access.sdk.title": "Python and JavaScript",
      "access.notes.title": "Notes for integration",
      "access.notes.auth": "OpenAI-compatible clients use <code>Authorization: Bearer YOUR_API_KEY</code>.",
      "access.notes.anthropic":
        "Anthropic clients can use <code>x-api-key</code> with the <code>/v1/messages</code> path.",
      "access.notes.stream":
        "Streaming requests keep the events of the original protocol; the Gateway converts when the protocols differ.",

      "usage.coverage": "Usage coverage",
      "usage.coverage.note": "Requests reporting token or reasoning usage",
      "usage.total": "Total tokens",
      "usage.cached": "Cached tokens",
      "usage.cached.note": "Cache hits reported by the upstream",
      "usage.reasoning": "Reasoning tokens",
      "usage.reasoning.note": "Reasoning usage reported by the upstream",
      "usage.trend.title": "Per-minute token trend",
      "usage.trend.caption": "Last 60 minutes; hover to see input, output, and total.",
      "usage.models.title": "Model ranking",
      "usage.models.caption": "Sorted by total_tokens.",
      "usage.models.col.model": "Model",
      "usage.models.col.input": "Input",
      "usage.models.col.output": "Output",
      "usage.models.col.total": "Total",
      "usage.tiers.title": "Tier distribution",
      "usage.tiers.caption": "Where Zen / Go tokens are going.",
      "usage.trend.peak": "Peak {0} tokens / minute · last {1} minutes",
      "usage.trend.empty": "No trend data yet",

      "playground.title": "Three-protocol request bench",
      "playground.caption": "The server forces non-streaming and routes through the real Gateway.",
      "playground.protocol": "Input protocol",
      "playground.model": "Model",
      "playground.key": "Upstream Key",
      "playground.key.auto": "Automatic routing (anonymous / Tier fallback / Key rotation)",
      "playground.key.caption":
        "When a specific key is chosen, only that key is tried: no switch to another key, another Tier, or the anonymous channel, and no change to the key's cooldown state or the proxy's health.",
      "playground.request": "Request JSON",
      "playground.reset": "Reset sample",
      "playground.run": "Send diagnostic request",
      "playground.result.title": "Diagnostic result",
      "playground.result.idle": "No request sent yet.",
      "playground.output": "Raw protocol response",

      "diagnostics.metadata.title": "Model metadata",
      "diagnostics.metadata.caption":
        "Status of the price cache from the OpenCode metadata endpoint; a price of 0 or a name containing free is enough to qualify for the anonymous channel.",
      "diagnostics.models.title": "Model routing table",
      "diagnostics.models.caption":
        "Explicit protocol, anonymous eligibility source, and how cost is determined.",
      "diagnostics.models.filter": "Filter models",
      "diagnostics.models.col.model": "Model",
      "diagnostics.models.col.protocol": "Native protocol",
      "diagnostics.models.col.route": "Route",
      "diagnostics.models.col.eligibility": "Anonymous eligibility",
      "diagnostics.models.col.source": "Evaluation source",
      "diagnostics.models.col.cost": "Input / output cost",
      "diagnostics.requests.title": "Request routing",
      "diagnostics.requests.caption":
        "The key tail or anonymous channel actually used per request; only the 100 most recent records are shown.",
      "diagnostics.attempts.title": "Upstream attempts",
      "diagnostics.attempts.caption":
        "Key tail, anonymous channel, and proxy node are masked; only the 100 most recent records are shown.",
      "diagnostics.col.time": "Time",
      "diagnostics.col.model": "Model",
      "diagnostics.col.tier": "Tier",
      "diagnostics.col.channel": "Channel",
      "diagnostics.col.tail": "Key tail",
      "diagnostics.col.attempts": "Attempts",
      "diagnostics.col.state": "State",
      "diagnostics.col.duration": "Duration",
      "diagnostics.col.proxy": "Proxy node",

      "config.server.title": "Server and admin console",
      "config.server.caption": "Listen addresses require a restart after saving.",
      "config.server.prefer": "Preferred Tier for authentication keys",
      "config.server.prefer.go": "Go (fall back to Zen on error)",
      "config.server.prefer.zen": "Zen (fall back to Go on error)",
      "config.server.session": "Session TTL (minutes)",
      "config.server.enabled": "Enable WebUI (restart required after change)",
      "config.secrets.title": "Secrets and anonymous channel",
      "config.secrets.caption":
        "Sensitive values are masked by default; a cleared value only takes effect after saving.",
      "config.secrets.reveal": "Reveal after re-verification",
      "config.secrets.anonymous": "Enable the Zen anonymous channel",
      "config.secrets.add": "One key per line",
      "config.proxy.title": "Proxy nodes",
      "config.proxy.add": "Add proxies (one per line)",
      "config.upstream.title": "Upstream and protocol routing",
      "config.upstream.caption":
        "anonymous will use every available proxy; an explicit protocol mapping wins over inferring the protocol from the model name.",
      "config.upstream.freeOnly": "Only expose free models (even when Zen / Go Keys are configured)",
      "config.upstream.attempts": "Max attempts per Key Tier",
      "config.upstream.timeout": "Request timeout (seconds)",
      "config.upstream.refresh": "Model refresh interval (seconds)",
      "config.upstream.protocols": "Model protocol overrides (JSON)",
      "config.reasoning.title": "Thinking effort",
      "config.reasoning.caption":
        "Applied only when the client does not specify an effort; per-model overrides take priority. Choosing “Off” removes reasoning for requests that do not ask for it.",
      "config.reasoning.default": "Default effort",
      "config.reasoning.follow": "Follow the client",
      "config.reasoning.auto": "auto (probe the highest available effort)",
      "config.reasoning.none": "Off",
      "config.reasoning.byModel": "Per-model overrides (JSON)",
      "config.reasoning.byModelNote":
        "Values may be minimal, low, medium, high, xhigh, max, or none; an empty object means no overrides.",
      "config.performance.title": "Performance and logging",
      "config.performance.caption": "A hot reload still keeps the in-process accumulated totals.",
      "config.performance.idleTimeout": "Idle timeout (seconds)",
      "config.performance.connect": "Connect timeout (seconds)",
      "config.performance.cooldown": "Failure cooldown (seconds)",
      "config.performance.attemptTimeout": "Attempt timeout (seconds)",
      "config.performance.attemptTimeoutHint": "0 = follow the request timeout",
      "config.performance.firstEvent": "First SSE event timeout (seconds, 0 disables)",
      "config.performance.level": "Log level",
      "config.performance.ring": "Log ring size",
      "config.performance.dump":
        "Write outbound request dumps (masked, for tracing retries and conversions only)",
      "config.reload": "Reload from disk",
      "config.save": "Validate, save, and apply",

      "logs.level.all": "All levels",
      "logs.component": "Module",
      "logs.search": "Search events, Request ID, model",
      "logs.pause": "Pause",
      "logs.resume": "Resume",
      "logs.gap": "Older logs were dropped from memory",
      "logs.waiting": "Waiting for log events…",

      "quotas.title": "OpenCode Go plan quota",
      "quotas.caption":
        "Queried directly from OpenCode, cached for 60 seconds. All models share one plan window; percentages are rounded by the upstream.",
      "quotas.refresh": "Refresh quota",
      "quotas.loading": "Reading…",
      "quotas.pending": "The quota will be read when this page opens.",
      "quotas.catalog.title": "Available models",
      "quotas.catalog.caption":
        "Model names are used verbatim for OPENAI_MODEL. The quota column reflects the plan selected above, not a per-model quota.",
      "quotas.catalog.search": "Search model names…",
      "quotas.col.model": "Model name",
      "quotas.col.protocol": "Upstream protocol",
      "quotas.col.hour": "5-hour remaining · shared",
      "quotas.col.week": "Weekly remaining · shared",
      "quotas.col.month": "Monthly remaining · shared",
      "quotas.col.action": "Action",

      "rotation.title": "SOTA / sweet model rotation",
      "rotation.caption":
        "On success the current model is kept. When the quota runs out or an error occurs it moves in order, using OpenCode Go only.",
      "rotation.refresh": "Refresh status",
      "rotation.usage":
        "Keep your existing Base URL and API Key and set OPENAI_MODEL to the alias below. Each model is retried up to 3 times on ordinary errors, and a single request tries at most 3 models; a quota-exhausted model is skipped immediately.",
      "rotation.timeout": "Maximum wait for the first valid output (seconds)",
      "rotation.save": "Save aliases and order",
      "rotation.saveNote":
        "Saving takes effect immediately; reordering never cuts off an answer that is already streaming. Manually selecting the current model takes effect immediately and clears that model's paused state.",
      "rotation.attempts.title": "Recent attempts",
      "rotation.col.time": "Time",
      "rotation.col.request": "Request",
      "rotation.col.alias": "Alias",
      "rotation.col.model": "Actual model",
      "rotation.col.attempt": "Attempt",
      "rotation.col.result": "Result",

      "metrics.title": "Prometheus metrics",
      "metrics.caption":
        "Expose requests, tokens, upstream attempts and pool health to a standard scraper, ready for Prometheus and Grafana.",
      "metrics.enable": "Enable",
      "metrics.disable": "Disable",
      "metrics.refresh": "Refresh",
      "metrics.url": "Scrape address",
      "metrics.auth":
        "Scraping needs a local API Key: Authorization: Bearer <server_key>. Once enabled, every enabled key can read it, so expose it only on a trusted network.",
      "metrics.enabled": "Enabled: a scraper can read the address above. Changes apply immediately.",
      "metrics.disabled": "Disabled: /metrics answers 404. Enabling takes effect immediately, with no restart.",
      "metrics.preview.title": "Live output",
      "metrics.preview.caption":
        "Identical to what a scraper receives, and visible to signed-in operators even while the endpoint is off.",
      "metrics.preview.output": "Exposition text",
      "metrics.preview.lines": "{0} lines",
      "metrics.embeddings.title": "Embeddings relay",
      "metrics.embeddings.caption":
        "A new OpenAI-compatible /v1/embeddings relay: requests and replies keep the OpenAI shape end to end, with tier fallback and key rotation and no protocol translation.",
      "metrics.embeddings.example": "Example call",
      "metrics.embeddings.note":
        "The model ID is not filtered by the model catalog; the upstream decides whether it can embed, and its rejection is relayed verbatim. A stream flag is rejected outright.",
      "metrics.embeddings.enable": "Enable /v1/embeddings",
      "metrics.embeddings.base": "Dedicated upstream (empty uses the Zen / Go pools)",
      "metrics.embeddings.baseNote":
        "OpenCode Zen has no embeddings endpoint, so an empty value relays the request and returns its 404. Fill in any OpenAI-compatible base (OpenAI, SiliconFlow, Ollama, …) to actually produce vectors.",
      "metrics.embeddings.key": "Upstream API key",
      "metrics.embeddings.keySaved":
        "A key is stored; leave this empty to keep it, or type a new one to replace it.",
      "metrics.embeddings.keyEmpty":
        "No key stored; an empty value sends no authorization header.",
      "metrics.embeddings.save": "Save embeddings settings",
      "metrics.embeddings.saved": "Embeddings settings saved",
      "metrics.embeddings.on": "Enabled, upstream: {0}",
      "metrics.embeddings.off": "Disabled: /v1/embeddings answers 404.",
      "metrics.toast.on": "Metrics enabled",
      "metrics.toast.off": "Metrics disabled",

      "apikeys.create.title": "Create an API Key for a caller",
      "apikeys.create.caption":
        "Used to reach the Gateway on this machine. Each application can have its own key; creating, enabling or disabling, and deleting all take effect immediately.",
      "apikeys.create.name": "Name",
      "apikeys.create.namePlaceholder": "e.g. my notes helper",
      "apikeys.create.value": "Custom key value (optional)",
      "apikeys.create.valuePlaceholder":
        "Leave blank to generate one; a custom value must be 16–256 characters",
      "apikeys.create.submit": "Create API Key",
      "apikeys.created.title": "Key created",
      "apikeys.created.caption":
        "The full key is shown only once, right after creation. Save it into the caller's environment variable immediately.",
      "apikeys.created.dismiss": "Saved, close",
      "apikeys.created.copy": "Copy key",
      "apikeys.created.copyEnv": "Copy environment variable",
      "apikeys.list.title": "Access keys",
      "apikeys.list.caption":
        "Manage who may call the Gateway. OpenCode upstream keys stay in the Configuration center.",
      "apikeys.list.refresh": "Refresh list",
      "apikeys.col.name": "Name",
      "apikeys.col.tail": "Key tail",
      "apikeys.col.state": "State",
      "apikeys.col.usage": "Usage (last hour / total)",
      "apikeys.col.limits": "Today / limits",
      "apikeys.col.created": "Created at",
      "apikeys.col.action": "Action",
      "apikeys.col.legacy": "Already in configuration",
      "apikeys.note":
        "Disabling and deleting makes new requests from the matching applications return 401. At least one enabled key must remain. Limits are per key: the per-minute and daily request ceilings refuse before routing, and the daily token budget refuses the requests that follow once it is exceeded. Counters follow the UTC day, are written to disk, and keep accumulating across a restart until the day rolls over. An expiry stops authentication the moment it is reached.",
      "apikeys.limits.button": "Limits",
      "apikeys.limits.title": "Set limits · {0}",
      "apikeys.limits.rpm": "Requests per minute",
      "apikeys.limits.daily": "Requests per day",
      "apikeys.limits.tokens": "Daily token budget",
      "apikeys.limits.models": "Model allowlist",
      "apikeys.limits.modelsPlaceholder": "One model ID per line; a prefix like gpt-* is allowed",
      "apikeys.limits.zeroHint": "0 means unlimited.",
      "apikeys.limits.dailyHint": "Counted per UTC day. 0 means unlimited.",
      "apikeys.limits.tokensHint":
        "Only usage the upstream reports is counted; once exceeded, later requests are refused. 0 means unlimited.",
      "apikeys.limits.modelsHint": "Leave empty to allow every model.",
      "apikeys.limits.clear": "Clear limits",
      "apikeys.limits.unlimited": "Unlimited",
      "apikeys.limits.rpmShort": "{0}/min",
      "apikeys.limits.dailyShort": "{0}/day",
      "apikeys.limits.tokensShort": "{0} tokens/day",
      "apikeys.limits.modelsShort": "{0} models",
      "apikeys.limits.today": "Today {0} requests · {1} tokens",
      "apikeys.expired": "Expired",
      "apikeys.expiry.field": "Expiry",
      "apikeys.expiry.hint":
        "Leave empty to never expire; once reached, the key stops authenticating immediately.",
      "apikeys.expiry.until": "until {0}",
      "apikeys.expiry.never": "no expiry",

      "account.title": "Update the admin account",
      "account.caption": "On success every session is revoked and you must sign in again.",
      "account.user": "New username (blank to keep the current one)",
      "account.current": "Current password",
      "account.new": "New password (blank to keep the current one, at least 10 characters)",
      "account.submit": "Update and sign in again",

      "modal.title": "Sensitive configuration",
      "modal.close": "Close",

      "common.copy": "Copy",
      "common.save": "Save",
      "common.cancel": "Cancel",
      "common.confirm": "Confirm",
      "common.rename": "Rename",
      "common.delete": "Delete",
      "common.view": "Reveal",
      "common.notReady": "Not ready yet",
      "modal.revealTitle": "Reveal sensitive configuration",
      "common.empty": "No data yet",
      "common.remove": "Remove",
      "common.duration": "Duration",
      "common.model": "Model",
      "common.action": "Action",
      "common.yes": "Yes",
      "common.no": "No",
      "common.none": "None",
      "common.on": "On",
      "common.off": "Off",
      "common.allowed": "Allowed",
      "common.denied": "Denied",
      "common.unknown": "Unknown",

      "error.session": "Your session has expired",
      "error.timeout": "The request timed out after {0} s",
      "toast.refreshFailed": "Refresh failed",

      "state.healthy": "Healthy",
      "state.unavailable": "Unavailable",
      "state.enabled": "Enabled",
      "state.disabled": "Disabled",
      "state.cooling": "Cooling down",
      "state.available": "Ready",

      "breakdown.endpoint": "Endpoint",
      "breakdown.status": "Status code",
      "breakdown.model": "Model",
      "breakdown.empty": "No requests yet",

      "guide.models.cached": "Catalog from the {0} cache, waiting for a background refresh.",
      "guide.models.ready": "The upstream catalog has passed its first refresh and has models to serve.",

      "usage.empty.models": "No token usage yet",
      "usage.empty.tiers": "No per-Tier usage yet",

      "playground.key.cooling": " · cooling down until {0}",
      "playground.samplePrompt": "Explain what this model can do in one sentence.",
      "play.httpStatus": "HTTP",
      "play.mode": "Test mode",
      "play.mode.selected": "Specified {0} key",
      "play.mode.auto": "Automatic routing",
      "play.testedKey": "Key tested",
      "play.keyVerdict": "Key verdict",
      "play.keyVerdict.none": "Automatic routing has no dedicated verdict",
      "play.key.usable": "Key usable",
      "play.key.rejected": "Key rejected",
      "play.key.rateLimited": "Key accepted but rate limited",
      "play.key.upstreamError": "Upstream anomaly, no verdict on the key",
      "play.key.transportError": "Network or proxy anomaly, no verdict on the key",
      "play.key.unavailable": "Key or runtime unavailable",
      "play.key.requestError": "Request did not succeed",
      "play.actualRoute": "Actual route",
      "play.nativeProtocol": "Native protocol",
      "play.anonymousVerdict": "Anonymous verdict",
      "play.eligibility.allowed": "Allowed · {0}",
      "play.eligibility.denied": "Denied · {0}",
      "play.route.none": "Not routed",
      "play.state.ok": "Request succeeded · HTTP {0}",
      "play.state.failed": "Request failed · HTTP {0}",
      "play.state.testingKey": "Testing with the specified key…",
      "play.state.running": "Sending the request through the Gateway…",
      "play.state.failedToCall": "Could not reach the diagnostics endpoint",

      "diag.ready": "Ready",
      "diag.modelCount": "Models",
      "diag.updatedAt": "Updated at",
      "diag.nextRefresh": "Next refresh",
      "diag.stale": "Stale",
      "diag.lastError": "Last error",
      "diag.notUpdated": "Not updated",
      "diag.streamError": "Streaming failed",
      "diag.clientCanceled": "Client canceled",
      "diag.noRequests": "No requests have been routed",
      "diag.noAttempts": "No upstream attempts",
      "diag.noMatch": "No matching models",

      "quotas.noKey": "No Go Key configured",
      "quotas.addKeyFirst": "Add an OpenCode Go Key in the Configuration center first.",
      "quotas.queryFailed": "Query failed: {0}. The content shown is only the previous query result.",
      "quotas.connected": "Connected to OpenCode · shared plan quota. Quotas of several keys do not add up.",
      "quotas.error": "{0}; the quota is currently unknown.",
      "quotas.staleError": "{0}; the most recent successful readings are shown below and the quota is currently unknown.",
      "quotas.updated": "Quota updated at: {0} · 60s cache · source: OpenCode Go /v1/usage",
      "quotas.lastSuccess": "Last successful query: {0} · 60s cache · source: OpenCode Go /v1/usage",
      "quotas.window.hour": "5-hour window",
      "quotas.window.week": "Weekly window",
      "quotas.window.month": "Monthly window",
      "quotas.window.aria": "Remaining quota for {0}",
      "quotas.remaining": "remaining",
      "quotas.remainingHistory": "remaining (history)",
      "quotas.rateLimited": "cap reached · ",
      "quotas.resetsAt": "resets at {0}",
      "quotas.waiting": "Waiting for quota data from the upstream",
      "quotas.catalogStale": "The model catalog is not up to date, showing the cache · ",
      "quotas.catalogUpdated": "Catalog updated at: {0}",
      "quotas.history": " (history)",
      "quotas.copyName": "Copy name",
      "quotas.catalogPending": "The model catalog is not ready yet; please refresh later.",

      "apikeys.loadFailed": "Read failed: {0}",
      "apikeys.count": "{0} keys",
      "apikeys.enabledCount": "{0} keys enabled",
      "apikeys.times": "times",
      "apikeys.namePrompt": "Key name",
      "apikeys.enable": "Enable",
      "apikeys.disable": "Disable",
      "apikeys.confirmDelete":
        "After deleting “{0}”, applications using this key will lose access. Delete it for real?",
      "apikeys.empty": "No API Keys yet",

      "rotation.status":
        "Status updated · the two rotation groups are independent and every caller shares the current position.",
      "rotation.sweet": "sweet",
      "rotation.aliasLabel": "Call alias",
      "rotation.current": "Current model: {0}",
      "rotation.waitingModel": "waiting for an available model",
      "rotation.col.order": "Order",
      "rotation.col.state": "State / resume time",
      "rotation.unavailable": "Removed from the catalog / temporarily unavailable",
      "rotation.pausedUntil": "Paused until {0}",
      "rotation.inUse": "In use",
      "rotation.moveUp": "Up",
      "rotation.moveDown": "Down",
      "rotation.setCurrent": "Set as current",
      "rotation.useModel": "Use {0}",
      "rotation.waitingCatalog": "Waiting for the Go model catalog",
      "rotation.recentSwitches": "Recent switches",
      "rotation.noSwitches": "No switch records yet",
      "rotation.attemptsEmpty": "Call through a group alias to see the 100 most recent attempts",
      "rotation.reason.success": "Succeeded",
      "rotation.reason.clientCanceled": "Caller canceled",
      "rotation.reason.quota": "Quota exhausted / rate limited",
      "rotation.reason.transport": "Connection error",
      "rotation.reason.firstOutputTimeout": "First output timeout",
      "rotation.reason.streamError": "Output interrupted",
      "rotation.reason.readError": "Failed to read the response",
      "rotation.reason.conversionError": "Failed to parse or convert the response",
      "rotation.reason.requestConversion": "Failed to convert the request",
      "rotation.reason.upstreamError": "Upstream error",

      "availability.pending": "Awaiting probe",
      "availability.manual": "Manual",
      "availability.auto": "Automatic",
      "availability.unverified": "Unverified",
      "availability.manualControl": "Manual control, not probed automatically",
      "availability.restore": "Re-enable",
      "availability.switchAuto": "Switch back to automatic",
      "availability.switchManual": "Switch to manual",
      "availability.noneEligible":
        "No model in the current catalog qualifies as free.",
      "availability.channel.anonymous": "Anonymous channel",
      "availability.channel.zen": "Zen Key",
      "availability.reason.probeSucceeded": "Probe succeeded",
      "availability.reason.modelUnavailable": "Model unavailable",
      "availability.reason.invalidResponse": "Invalid or incomplete response",
      "availability.reason.probeTimeout": "Probe timed out",
      "availability.reason.transportError": "Network or proxy error",
      "availability.reason.conversionError": "Request conversion failed",
      "availability.reason.creationError": "Could not build the request",
      "availability.reason.unsupportedProbe": "Protocol cannot be probed",
      "availability.reason.noChannel": "No probe channel available",
      "availability.reason.notFree": "Not a free model",
      "availability.reason.canceled": "Cancelled",
      "availability.reason.manuallyEnabled": "Enabled manually",
      "availability.reason.autoProbe": "Automatic probe",
      "availability.reason.legacyRecheck": "Legacy record, recheck pending",
      "availability.reason.http": "HTTP {0}",

      "common.unsaved":
        "Unsaved changes",
      "config.restartNotice":
        "Saved but not in effect yet: {0}. The current API is {1}, the WebUI is {2}. Please restart the process.",
      "config.revealPrompt": "Re-enter the admin password",

      "toast.refreshed": "Status refreshed",
      "toast.copied": "Copied",
      "toast.copyDenied": "The browser denied the copy; select the text and copy it manually",
      "toast.example.copied": "Example copied",
      "toast.config.saved": "Configuration saved and already in effect",
      "toast.config.restart": "Applied; restart required for {0}",
      "toast.config.reloaded": "Configuration reloaded from disk",
      "toast.config.reloadedRestart":
        "Configuration reloaded from disk; listen fields require a restart",
      "toast.account.updated": "Account updated, please sign in again",
      "toast.play.invalidJson": "The request JSON is invalid: {0}",
      "toast.play.copied": "Raw response copied",
      "toast.apikeys.saved": "Saved and already in effect",
      "toast.apikeys.created": "API Key created",
      "toast.rotation.saved": "Model rotation configuration saved and in effect",
      "toast.rotation.switched": "Current model switched",
      "toast.rotation.dirty": "There are unsaved changes; save before refreshing",
    },

    vi: {
      "doc.title": "opencode2api · Sổ tay vận hành",
      "lang.label": "Ngôn ngữ",
      "a11y.skip": "Chuyển tới nội dung chính",
      "a11y.nav": "Điều hướng trang",
      "a11y.tabs.overview": "Chế độ xem tổng quan",
      "a11y.tabs.diagnostics": "Chế độ xem chẩn đoán",
      "a11y.tabs.quotas": "Chế độ xem mô hình",
      "a11y.tabs.config": "Phần cấu hình",
      "a11y.logLevel": "Mức log",

      "login.title": "Đăng nhập quản trị",
      "login.username": "Tài khoản",
      "login.password": "Mật khẩu",
      "login.submit": "Vào Sổ tay vận hành",

      "nav.overview": "Bàn vận hành",
      "nav.access": "Sổ tay tích hợp",
      "nav.diagnostics": "Chẩn đoán định tuyến",
      "nav.config": "Trung tâm cấu hình",
      "nav.account": "Bảo mật tài khoản",
      "nav.model-quotas": "Mô hình và hạn mức",
      "nav.api-keys": "Quản lý API Key",
      "nav.rotation": "Luân chuyển mô hình",
      "nav.metrics": "Chỉ số giám sát",
      "nav.group.runtime": "Vận hành",
      "nav.group.integration": "Tích hợp",
      "nav.group.models": "Mô hình",
      "nav.group.system": "Hệ thống",
      "overview.tab.summary": "Tổng quan",
      "overview.tab.usage": "Mức dùng",
      "overview.checklist.title": "Kiểm tra sẵn sàng",
      "overview.checklist.caption": "Trạng thái lấy từ cấu hình hiện tại và tài nguyên thời gian thực.",
      "diagnostics.tab.playground": "Thử request",
      "diagnostics.tab.ledger": "Luồng request",
      "diagnostics.tab.logs": "Luồng sự kiện",
      "diagnostics.tab.route": "Định tuyến và điều kiện",
      "quotas.tab.quota": "Hạn mức gói",
      "quotas.tab.free": "Mô hình miễn phí",
      "config.tab.service": "Dịch vụ",
      "config.tab.keys": "Khóa và ẩn danh",
      "config.tab.proxies": "Proxy",
      "config.tab.upstream": "Upstream và mô hình",
      "config.tab.reasoning": "Suy luận và hiệu năng",
      "config.tab.logging": "Nhật ký",
      "usage.hour": "60 phút gần nhất",

      "topbar.refresh": "Làm mới ngay",
      "topbar.logout": "Đăng xuất",

      "page.rotation.title": "Luân chuyển mô hình",
      "page.rotation.sub": "Mô hình hiện tại, chuyển đổi khi lỗi và lịch sử thử.",
      "page.overview.title": "Bàn vận hành",
      "page.overview.sub": "Một giờ gần nhất, số liệu tích lũy của tiến trình và tài nguyên hiện tại.",
      "page.access.title": "Sổ tay tích hợp",
      "page.access.sub": "Ví dụ Chat, Responses, Anthropic và SDK.",
      "page.diagnostics.title": "Chẩn đoán định tuyến",
      "page.diagnostics.sub": "Metadata, điều kiện mô hình và từng lần thử upstream.",
      "page.config.title": "Trung tâm cấu hình",
      "page.config.sub": "Kiểm tra rồi lưu nguyên tử, áp dụng nóng các trường có thể thay đổi.",
      "page.account.title": "Bảo mật tài khoản",
      "page.account.sub": "Cập nhật xong sẽ thu hồi toàn bộ session quản trị.",
      "page.model-quotas.title": "Mô hình và hạn mức",
      "page.model-quotas.sub": "Mô hình khả dụng, hạn mức gói dùng chung và thời điểm reset.",
      "page.api-keys.title": "Quản lý API Key",
      "page.api-keys.sub": "Khóa truy cập riêng cho từng bên gọi.",

      "availability.title": "Dò mô hình miễn phí",
      "availability.refresh": "Làm mới trạng thái",
      "availability.desc":
        "Kiểm tra các mô hình có tên chứa free hoặc giá bằng 0 trong metadata và chưa bị loại bỏ. Ưu tiên kênh ẩn danh, sau đó mới thử Zen Key; chỉ vô hiệu hóa khi mô hình hẳn đã hỏng và thử lại sau 24 giờ. Lỗi tạm thời không tạo thêm mô hình bị vô hiệu hóa: mô hình đang bật thử lại sau 1 giờ, mô hình đã tắt giữ nguyên lý do cũ và thử lại sau 24 giờ. Khi khôi phục bật hoặc chuyển sang điều khiển thủ công, mô hình giữ nguyên bật và không còn được dò tự động; khi chuyển lại tự động, lượt định tuyến kế tiếp sẽ sắp xếp lần dò. System One dùng bộ câu hỏi quyết định riêng 1+1. Khi đặt cường độ auto, hiển thị bậc cao nhất đã được dò xác minh.",
      "availability.col.model": "Mô hình",
      "availability.col.state": "Trạng thái",
      "availability.col.channel": "Kênh / Kết quả",
      "availability.col.control": "Điều khiển",
      "availability.col.effort": "Bậc suy luận đã xác minh",
      "availability.col.checked": "Lần dò gần nhất",
      "availability.col.next": "Lần dò kế tiếp",
      "availability.col.action": "Thao tác",

      "overview.requests": "Yêu cầu 1 giờ qua",
      "overview.success": "Tỷ lệ thành công",
      "overview.p95": "Độ trễ P95",
      "overview.active": "Đang chạy / luồng",
      "overview.hour": "60 phút gần nhất",
      "overview.http2xx": "HTTP 2xx–3xx",
      "overview.endToEnd": "Mili giây đầu–cuối",
      "overview.concurrent": "Số request đồng thời",
      "overview.cost": "Tổng chi phí",
      "overview.costNote": "Ước tính theo giá trong danh mục",
      "overview.proxy.title": "Nút Proxy",
      "overview.proxy.col.node": "Nút",
      "overview.proxy.col.health": "Sức khoẻ",
      "overview.proxy.col.binding": "Ràng buộc",
      "overview.keys.title": "Nhóm Key",
      "overview.keys.col.tail": "Đuôi Key",
      "overview.keys.col.tier": "Tier",
      "overview.keys.col.state": "Trạng thái",
      "overview.distribution.title": "Phân bố yêu cầu",
      "overview.distribution.caption":
        "Tổng hợp 1 giờ gần nhất theo endpoint, mô hình, Tier và mã trạng thái.",

      "guide.step.serverkey": "Cấu hình Key cục bộ",
      "guide.step.serverkey.desc": "Client dùng Server Key để truy cập Gateway.",
      "guide.step.upstream": "Chọn kênh upstream",
      "guide.step.upstream.desc": "Có ít nhất một Zen / Go Key, hoặc đã bật kênh ẩn danh.",
      "guide.step.proxy": "Kiểm tra Proxy",
      "guide.step.proxy.desc": "Có ít nhất một nút proxy còn khỏe.",
      "guide.step.models": "Chờ danh mục mô hình",
      "guide.done": "✓ Đã sẵn sàng",
      "guide.todo": "○ Cần xử lý",

      "access.base.title": "Địa chỉ cơ sở",
      "access.base.caption": "Ví dụ không ghi Server Key thật.",
      "access.base.label": "API Base được suy ra",
      "access.model.label": "Mô hình ví dụ",
      "access.protocols.title": "Các cổng vào theo ba giao thức",
      "access.sdk.title": "Python và JavaScript",
      "access.notes.title": "Lưu ý khi tích hợp",
      "access.notes.auth": "Client tương thích OpenAI dùng <code>Authorization: Bearer YOUR_API_KEY</code>.",
      "access.notes.anthropic":
        "Client Anthropic có thể dùng <code>x-api-key</code>, với đường dẫn <code>/v1/messages</code>.",
      "access.notes.stream":
        "Request streaming giữ nguyên sự kiện của giao thức gốc; khi khác giao thức, Gateway sẽ chuyển đổi.",

      "usage.coverage": "Độ phủ sử dụng",
      "usage.coverage.note": "Yêu cầu có token trả về / suy luận",
      "usage.total": "Tổng token",
      "usage.cached": "Token đã cache",
      "usage.cached.note": "Cache hit do upstream báo",
      "usage.reasoning": "Token suy luận",
      "usage.reasoning.note": "Mức dùng suy luận do upstream báo",
      "usage.trend.title": "Xu hướng token mỗi phút",
      "usage.trend.caption": "60 phút gần nhất; di chuột để xem input, output và tổng.",
      "usage.models.title": "Xếp hạng mô hình",
      "usage.models.caption": "Sắp xếp theo total_tokens.",
      "usage.models.col.model": "Mô hình",
      "usage.models.col.input": "Input",
      "usage.models.col.output": "Output",
      "usage.models.col.total": "Tổng",
      "usage.tiers.title": "Phân bố Tier",
      "usage.tiers.caption": "Token của Zen / Go đi về đâu.",
      "usage.trend.peak": "Đỉnh {0} token / phút · {1} phút gần nhất",
      "usage.trend.empty": "Chưa có dữ liệu xu hướng",

      "playground.title": "Bàn gửi request ba giao thức",
      "playground.caption": "Máy chủ ép buộc không streaming và định tuyến qua Gateway thật.",
      "playground.protocol": "Giao thức đầu vào",
      "playground.model": "Mô hình",
      "playground.key": "Key upstream",
      "playground.key.auto": "Định tuyến tự động (ẩn danh / dự phòng Tier / luân phiên Key)",
      "playground.key.caption":
        "Khi chọn một Key cụ thể, chỉ Key đó được thử: không chuyển sang Key khác, Tier khác hay kênh ẩn danh, và không làm thay đổi trạng thái cooldown của Key hay sức khoẻ của proxy.",
      "playground.request": "JSON của request",
      "playground.reset": "Đặt lại mẫu",
      "playground.run": "Gửi request chẩn đoán",
      "playground.result.title": "Kết quả chẩn đoán",
      "playground.result.idle": "Chưa gửi request nào.",
      "playground.output": "Phản hồi giao thức gốc",

      "diagnostics.metadata.title": "Metadata mô hình",
      "diagnostics.metadata.caption":
        "Trạng thái cache giá từ điểm cuối metadata OpenCode; giá bằng 0 hoặc tên chứa free là đủ điều kiện vào kênh ẩn danh.",
      "diagnostics.models.title": "Bảng định tuyến mô hình",
      "diagnostics.models.caption":
        "Giao thức tường minh, nguồn điều kiện ẩn danh và cách xác định chi phí.",
      "diagnostics.models.filter": "Lọc mô hình",
      "diagnostics.models.col.model": "Mô hình",
      "diagnostics.models.col.protocol": "Giao thức gốc",
      "diagnostics.models.col.route": "Định tuyến",
      "diagnostics.models.col.eligibility": "Điều kiện ẩn danh",
      "diagnostics.models.col.source": "Nguồn đánh giá",
      "diagnostics.models.col.cost": "Chi phí input / output",
      "diagnostics.requests.title": "Định tuyến request",
      "diagnostics.requests.caption":
        "Đuôi Key hoặc anonymous thực sự được dùng cho từng request; hiển thị tối đa 100 bản ghi gần nhất.",
      "diagnostics.attempts.title": "Lần thử upstream",
      "diagnostics.attempts.caption":
        "Đuôi Key, kênh ẩn danh và nút proxy đã che; hiển thị tối đa 100 bản ghi gần nhất.",
      "diagnostics.col.time": "Thời gian",
      "diagnostics.col.model": "Mô hình",
      "diagnostics.col.tier": "Tier",
      "diagnostics.col.channel": "Kênh",
      "diagnostics.col.tail": "Đuôi Key",
      "diagnostics.col.attempts": "Lần thử",
      "diagnostics.col.state": "Trạng thái",
      "diagnostics.col.duration": "Thời gian",
      "diagnostics.col.proxy": "Nút proxy",

      "config.server.title": "Máy chủ và bảng quản trị",
      "config.server.caption": "Địa chỉ lắng nghe cần khởi động lại sau khi lưu.",
      "config.server.prefer": "Tier ưu tiên cho Key xác thực",
      "config.server.prefer.go": "Go (lỗi thì dự phòng Zen)",
      "config.server.prefer.zen": "Zen (lỗi thì dự phòng Go)",
      "config.server.session": "Session TTL (phút)",
      "config.server.enabled": "Bật WebUI (cần khởi động lại khi thay đổi)",
      "config.secrets.title": "Khóa và kênh ẩn danh",
      "config.secrets.caption":
        "Giá trị nhạy cảm mặc định chỉ hiện dạng che; chỉ lưu sau khi gỡ bỏ mới có hiệu lực.",
      "config.secrets.reveal": "Xem sau khi xác minh lại",
      "config.secrets.anonymous": "Bật kênh ẩn danh Zen",
      "config.secrets.add": "Mỗi dòng thêm một key",
      "config.proxy.title": "Nút Proxy",
      "config.proxy.add": "Thêm Proxy (mỗi dòng một cái)",
      "config.upstream.title": "Upstream và định tuyến giao thức",
      "config.upstream.caption":
        "anonymous sẽ dùng hết các Proxy khả dụng; ánh xạ protocol tường minh được ưu tiên hơn việc suy luận từ tên mô hình.",
      "config.upstream.freeOnly": "Chỉ công khai mô hình miễn phí (kể cả khi đã cấu hình Zen / Go Key)",
      "config.upstream.attempts": "Số lần thử tối đa cho mỗi Key Tier",
      "config.upstream.timeout": "Timeout của request (giây)",
      "config.upstream.refresh": "Làm mới mô hình (giây)",
      "config.upstream.protocols": "Ghi đè giao thức mô hình (JSON)",
      "config.reasoning.title": "Cường độ Thinking",
      "config.reasoning.caption":
        "Chỉ áp dụng khi client không chỉ định rõ; có thể ghi đè theo từng mô hình. Chọn “Tắt” sẽ bỏ reasoning cho các request không yêu cầu rõ.",
      "config.reasoning.default": "Cường độ mặc định",
      "config.reasoning.follow": "Theo client",
      "config.reasoning.auto": "auto (dò mức cao nhất khả dụng)",
      "config.reasoning.none": "Tắt",
      "config.reasoning.byModel": "Ghi đè theo mô hình (JSON)",
      "config.reasoning.byModelNote":
        "Giá trị có thể là minimal, low, medium, high, xhigh, max hoặc none; đối tượng rỗng nghĩa là không ghi đè.",
      "config.performance.title": "Hiệu năng và nhật ký",
      "config.performance.caption": "Nạp lại nóng vẫn giữ số liệu tích lũy trong bộ nhớ tiến trình.",
      "config.performance.idleTimeout": "Idle Timeout (giây)",
      "config.performance.connect": "Connect Timeout (giây)",
      "config.performance.cooldown": "Failure Cooldown (giây)",
      "config.performance.attemptTimeout": "Attempt Timeout (giây)",
      "config.performance.attemptTimeoutHint": "0 = theo timeout của request",
      "config.performance.firstEvent":
        "Timeout sự kiện SSE đầu tiên (giây, 0 để tắt)",
      "config.performance.level": "Mức log",
      "config.performance.ring": "Kích thước ring log",
      "config.performance.dump":
        "Ghi dump request đi ra (đã che, chỉ dùng để truy vết retry và chuyển đổi)",
      "config.reload": "Nạp lại từ đĩa",
      "config.save": "Kiểm tra, lưu và áp dụng",

      "logs.level.all": "Tất cả mức",
      "logs.component": "Module",
      "logs.search": "Tìm sự kiện, Request ID, mô hình",
      "logs.pause": "Tạm dừng",
      "logs.resume": "Tiếp tục",
      "logs.gap": "Log cũ hơn đã bị xóa khỏi bộ nhớ",
      "logs.waiting": "Đang chờ sự kiện log…",

      "quotas.title": "Hạn mức gói OpenCode Go",
      "quotas.caption":
        "Truy vấn trực tiếp từ OpenCode, cache 60 giây. Mọi mô hình dùng chung một cửa sổ gói, phần trăm do upstream làm tròn.",
      "quotas.refresh": "Làm mới hạn mức",
      "quotas.loading": "Đang đọc…",
      "quotas.pending": "Sẽ đọc hạn mức khi mở trang.",
      "quotas.catalog.title": "Mô hình khả dụng",
      "quotas.catalog.caption":
        "Tên mô hình dùng thẳng cho OPENAI_MODEL. Cột hạn mức ứng với gói đã chọn ở trên, không phải mỗi mô hình có hạn mức riêng.",
      "quotas.catalog.search": "Tìm tên mô hình…",
      "quotas.col.model": "Tên mô hình",
      "quotas.col.protocol": "Giao thức upstream",
      "quotas.col.hour": "Còn lại 5 giờ · dùng chung",
      "quotas.col.week": "Còn lại tuần · dùng chung",
      "quotas.col.month": "Còn lại tháng · dùng chung",
      "quotas.col.action": "Thao tác",

      "rotation.title": "Luân chuyển mô hình SOTA / sweet",
      "rotation.caption":
        "Thành công thì giữ nguyên mô hình hiện tại. Khi hết hạn mức hoặc gặp lỗi sẽ chuyển theo thứ tự, chỉ dùng OpenCode Go.",
      "rotation.refresh": "Làm mới trạng thái",
      "rotation.usage":
        "Giữ nguyên Base URL và API Key hiện có, đặt OPENAI_MODEL là bí danh bên dưới. Mỗi mô hình lỗi thường được thử tối đa 3 lần, mỗi request tối đa thử 3 mô hình; hết hạn mức thì bỏ qua ngay.",
      "rotation.timeout": "Giới hạn chờ output hợp lệ đầu tiên (giây)",
      "rotation.save": "Lưu bí danh và thứ tự",
      "rotation.saveNote":
        "Lưu là có hiệu lực ngay; đổi thứ tự không cắt ngang câu trả lời đang xuất. Chọn thủ công mô hình hiện tại sẽ có hiệu lực ngay và gỡ trạng thái tạm dừng của mô hình đó.",
      "rotation.attempts.title": "Các lần thử gần nhất",
      "rotation.col.time": "Thời gian",
      "rotation.col.request": "Request",
      "rotation.col.alias": "Bí danh",
      "rotation.col.model": "Mô hình thực",
      "rotation.col.attempt": "Lần thử",
      "rotation.col.result": "Kết quả",

      "metrics.title": "Chỉ số Prometheus",
      "metrics.caption":
        "Công bố request, token, số lần thử upstream và tình trạng pool cho bộ thu thập chuẩn, dùng được với Prometheus và Grafana.",
      "metrics.enable": "Bật",
      "metrics.disable": "Tắt",
      "metrics.refresh": "Làm mới",
      "metrics.url": "Địa chỉ thu thập",
      "metrics.auth":
        "Thu thập cần API Key cục bộ: Authorization: Bearer <server_key>. Sau khi bật, mọi key đang bật đều đọc được, chỉ nên mở trong mạng tin cậy.",
      "metrics.enabled": "Đã bật: bộ thu thập có thể đọc địa chỉ bên trên. Thay đổi có hiệu lực ngay.",
      "metrics.disabled": "Chưa bật: /metrics trả 404. Bật lên có hiệu lực ngay, không cần khởi động lại.",
      "metrics.preview.title": "Output trực tiếp",
      "metrics.preview.caption":
        "Giống hệt nội dung bộ thu thập nhận được, người đã đăng nhập vẫn xem được khi endpoint đang tắt.",
      "metrics.preview.output": "Văn bản công bố",
      "metrics.preview.lines": "{0} dòng",
      "metrics.embeddings.title": "Relay Embeddings",
      "metrics.embeddings.caption":
        "Endpoint /v1/embeddings tương thích OpenAI mới: request và phản hồi giữ nguyên cấu trúc OpenAI, có dự phòng theo tier và xoay key, không chuyển đổi giao thức.",
      "metrics.embeddings.example": "Ví dụ gọi",
      "metrics.embeddings.note":
        "ID mô hình không bị lọc qua danh mục; upstream quyết định mô hình có hỗ trợ hay không và từ chối của nó được trả nguyên văn. Tham số stream bị từ chối ngay.",
      "metrics.embeddings.enable": "Bật /v1/embeddings",
      "metrics.embeddings.base": "Upstream riêng (để trống thì dùng pool Zen / Go)",
      "metrics.embeddings.baseNote":
        "OpenCode Zen không có endpoint embeddings, nên để trống sẽ chuyển tiếp request và trả lại 404 của upstream. Điền một địa chỉ tương thích OpenAI (OpenAI, SiliconFlow, Ollama, …) để thực sự tạo vector.",
      "metrics.embeddings.key": "API key upstream",
      "metrics.embeddings.keySaved":
        "Đã lưu key; để trống để giữ nguyên, hoặc nhập key mới để thay thế.",
      "metrics.embeddings.keyEmpty": "Chưa lưu key; để trống thì không gửi header xác thực.",
      "metrics.embeddings.save": "Lưu cấu hình embeddings",
      "metrics.embeddings.saved": "Đã lưu cấu hình embeddings",
      "metrics.embeddings.on": "Đã bật, upstream: {0}",
      "metrics.embeddings.off": "Chưa bật: /v1/embeddings trả 404.",
      "metrics.toast.on": "Đã bật chỉ số",
      "metrics.toast.off": "Đã tắt chỉ số",

      "apikeys.create.title": "Tạo API Key cho bên gọi",
      "apikeys.create.caption":
        "Dùng để truy cập Gateway trên máy này. Mỗi ứng dụng có thể dùng một khoá riêng; tạo, bật/tắt và xoá đều có hiệu lực ngay.",
      "apikeys.create.name": "Tên",
      "apikeys.create.namePlaceholder": "Ví dụ: trợ lý ghi chú của tôi",
      "apikeys.create.value": "Key tuỳ chọn (không bắt buộc)",
      "apikeys.create.valuePlaceholder":
        "Để trống để tự sinh; tuỳ chọn cần 16–256 ký tự",
      "apikeys.create.submit": "Tạo API Key",
      "apikeys.created.title": "Đã tạo khóa",
      "apikeys.created.caption":
        "Khóa đầy đủ chỉ hiển thị sau khi tạo, hãy lưu ngay vào biến môi trường của bên gọi.",
      "apikeys.created.dismiss": "Đã lưu, đóng",
      "apikeys.created.copy": "Sao chép Key",
      "apikeys.created.copyEnv": "Sao chép biến môi trường",
      "apikeys.list.title": "Khóa truy cập",
      "apikeys.list.caption":
        "Quản lý quyền truy cập Gateway. Khoá upstream của OpenCode Go vẫn nằm ở Trung tâm cấu hình.",
      "apikeys.list.refresh": "Làm mới danh sách",
      "apikeys.col.name": "Tên",
      "apikeys.col.tail": "Đuôi khóa",
      "apikeys.col.state": "Trạng thái",
      "apikeys.col.usage": "Mức dùng (1 giờ gần nhất / tích lũy)",
      "apikeys.col.limits": "Hôm nay / hạn mức",
      "apikeys.col.created": "Thời điểm tạo",
      "apikeys.col.action": "Thao tác",
      "apikeys.col.legacy": "Đã có trong cấu hình",
      "apikeys.note":
        "Tắt và xoá sẽ khiến các request mới của ứng dụng tương ứng trả về 401. Phải giữ lại ít nhất một Key đang bật. Hạn mức tính theo từng Key: giới hạn request mỗi phút và mỗi ngày bị chặn trước khi định tuyến, còn ngân sách token mỗi ngày chặn các request tiếp theo sau khi vượt. Bộ đếm theo ngày UTC, được ghi xuống đĩa và tiếp tục tích luỹ qua lần khởi động lại cho đến khi sang ngày mới. Thời điểm hết hạn sẽ dừng xác thực ngay khi tới.",
      "apikeys.limits.button": "Hạn mức",
      "apikeys.limits.title": "Đặt hạn mức · {0}",
      "apikeys.limits.rpm": "Request mỗi phút",
      "apikeys.limits.daily": "Request mỗi ngày",
      "apikeys.limits.tokens": "Ngân sách token mỗi ngày",
      "apikeys.limits.models": "Danh sách mô hình được phép",
      "apikeys.limits.modelsPlaceholder": "Mỗi dòng một ID mô hình; cho phép tiền tố như gpt-*",
      "apikeys.limits.zeroHint": "0 nghĩa là không giới hạn.",
      "apikeys.limits.dailyHint": "Tính theo ngày UTC. 0 nghĩa là không giới hạn.",
      "apikeys.limits.tokensHint":
        "Chỉ tính mức dùng upstream báo về; vượt là các request sau bị từ chối. 0 nghĩa là không giới hạn.",
      "apikeys.limits.modelsHint": "Để trống là cho phép mọi mô hình.",
      "apikeys.limits.clear": "Xoá hạn mức",
      "apikeys.limits.unlimited": "Không giới hạn",
      "apikeys.limits.rpmShort": "{0}/phút",
      "apikeys.limits.dailyShort": "{0}/ngày",
      "apikeys.limits.tokensShort": "{0} token/ngày",
      "apikeys.limits.modelsShort": "{0} mô hình",
      "apikeys.limits.today": "Hôm nay {0} request · {1} token",
      "apikeys.expired": "Đã hết hạn",
      "apikeys.expiry.field": "Thời điểm hết hạn",
      "apikeys.expiry.hint":
        "Để trống là không hết hạn; khi đến thời điểm, Key ngừng xác thực ngay.",
      "apikeys.expiry.until": "đến {0}",
      "apikeys.expiry.never": "không hết hạn",

      "account.title": "Cập nhật tài khoản quản trị",
      "account.caption": "Thành công sẽ thu hồi toàn bộ Session và yêu cầu đăng nhập lại.",
      "account.user": "Tài khoản mới (để trống để giữ nguyên)",
      "account.current": "Mật khẩu hiện tại",
      "account.new": "Mật khẩu mới (để trống để giữ nguyên, tối thiểu 10 ký tự)",
      "account.submit": "Cập nhật và đăng nhập lại",

      "modal.title": "Cấu hình nhạy cảm",
      "modal.close": "Đóng",

      "common.copy": "Sao chép",
      "common.save": "Lưu",
      "common.cancel": "Huỷ",
      "common.confirm": "Xác nhận",
      "common.rename": "Đổi tên",
      "common.delete": "Xoá",
      "common.view": "Xem",
      "common.notReady": "Chưa sẵn sàng",
      "modal.revealTitle": "Xem cấu hình nhạy cảm",
      "common.empty": "Chưa có dữ liệu",
      "common.remove": "Gỡ bỏ",
      "common.duration": "Thời gian",
      "common.model": "Mô hình",
      "common.action": "Thao tác",
      "common.yes": "Có",
      "common.no": "Không",
      "common.none": "Không có",
      "common.on": "Bật",
      "common.off": "Tắt",
      "common.allowed": "Cho phép",
      "common.denied": "Từ chối",
      "common.unknown": "Không rõ",

      "error.session": "Phiên đăng nhập đã hết hạn",
      "error.timeout": "Request quá thời gian sau {0} giây",
      "toast.refreshFailed": "Làm mới thất bại",

      "state.healthy": "Khỏe",
      "state.unavailable": "Không dùng được",
      "state.enabled": "Đang bật",
      "state.disabled": "Đang tắt",
      "state.cooling": "Đang nghỉ (cooldown)",
      "state.available": "Sẵn sàng",

      "breakdown.endpoint": "Endpoint",
      "breakdown.status": "Mã trạng thái",
      "breakdown.model": "Mô hình",
      "breakdown.empty": "Chưa có request",

      "guide.models.cached": "Danh mục lấy từ cache {0}, đang chờ nền làm mới.",
      "guide.models.ready": "Danh mục upstream đã qua lần làm mới đầu tiên và có mô hình để phục vụ.",

      "usage.empty.models": "Chưa có mức dùng token",
      "usage.empty.tiers": "Chưa có mức dùng theo Tier",

      "playground.key.cooling": " · nghỉ đến {0}",
      "playground.samplePrompt": "Giải thích năng lực của mô hình này trong một câu.",
      "play.httpStatus": "HTTP",
      "play.mode": "Chế độ thử",
      "play.mode.selected": "Chỉ định Key {0}",
      "play.mode.auto": "Định tuyến tự động",
      "play.testedKey": "Key được thử",
      "play.keyVerdict": "Kết luận về Key",
      "play.keyVerdict.none": "Định tuyến tự động không có kết luận riêng",
      "play.key.usable": "Key dùng được",
      "play.key.rejected": "Key bị từ chối",
      "play.key.rateLimited": "Key được chấp nhận nhưng bị giới hạn tần suất",
      "play.key.upstreamError": "Upstream bất thường, không kết luận được về Key",
      "play.key.transportError": "Mạng hoặc proxy bất thường, không kết luận được về Key",
      "play.key.unavailable": "Key hoặc môi trường chạy không dùng được",
      "play.key.requestError": "Request không thành công",
      "play.actualRoute": "Định tuyến thực tế",
      "play.nativeProtocol": "Giao thức gốc",
      "play.anonymousVerdict": "Kết luận ẩn danh",
      "play.eligibility.allowed": "Cho phép · {0}",
      "play.eligibility.denied": "Từ chối · {0}",
      "play.route.none": "Chưa định tuyến",
      "play.state.ok": "Request thành công · HTTP {0}",
      "play.state.failed": "Request thất bại · HTTP {0}",
      "play.state.testingKey": "Đang thử với Key được chỉ định…",
      "play.state.running": "Đang gửi request qua Gateway…",
      "play.state.failedToCall": "Không gọi được cổng chẩn đoán",

      "diag.ready": "Sẵn sàng",
      "diag.modelCount": "Số mô hình",
      "diag.updatedAt": "Thời điểm cập nhật",
      "diag.nextRefresh": "Lần làm mới kế tiếp",
      "diag.stale": "Đã cũ",
      "diag.lastError": "Lỗi gần nhất",
      "diag.notUpdated": "Chưa cập nhật",
      "diag.streamError": "Streaming thất bại",
      "diag.clientCanceled": "Client đã huỷ",
      "diag.noRequests": "Chưa có request nào được định tuyến",
      "diag.noAttempts": "Chưa có lần thử upstream",
      "diag.noMatch": "Không có mô hình phù hợp",

      "quotas.noKey": "Chưa cấu hình Go Key",
      "quotas.addKeyFirst": "Hãy thêm OpenCode Go Key trong Trung tâm cấu hình trước.",
      "quotas.queryFailed": "Truy vấn thất bại: {0}. Nội dung đang hiển thị chỉ là kết quả truy vấn trước.",
      "quotas.connected": "Đã kết nối OpenCode · hạn mức gói dùng chung. Hạn mức của nhiều Key không cộng dồn.",
      "quotas.error": "{0}; hiện không rõ hạn mức.",
      "quotas.staleError": "{0}; bên dưới hiển thị dữ liệu thành công gần nhất, hiện không rõ hạn mức.",
      "quotas.updated": "Thời điểm cập nhật hạn mức: {0} · cache 60 giây · nguồn: OpenCode Go /v1/usage",
      "quotas.lastSuccess": "Truy vấn thành công gần nhất: {0} · cache 60 giây · nguồn: OpenCode Go /v1/usage",
      "quotas.window.hour": "Cửa sổ 5 giờ",
      "quotas.window.week": "Cửa sổ theo tuần",
      "quotas.window.month": "Cửa sổ theo tháng",
      "quotas.window.aria": "Hạn mức còn lại của {0}",
      "quotas.remaining": "Còn lại",
      "quotas.remainingHistory": "Còn lại (lịch sử)",
      "quotas.rateLimited": "đã chạm trần · ",
      "quotas.resetsAt": "reset lúc {0}",
      "quotas.waiting": "Đang chờ dữ liệu hạn mức từ upstream",
      "quotas.catalogStale": "Danh mục mô hình chưa cập nhật, đang hiện cache · ",
      "quotas.catalogUpdated": "Danh mục cập nhật lúc: {0}",
      "quotas.history": "（lịch sử）",
      "quotas.copyName": "Sao chép tên",
      "quotas.catalogPending": "Danh mục mô hình chưa sẵn sàng, vui lòng làm mới sau",

      "apikeys.loadFailed": "Đọc thất bại: {0}",
      "apikeys.count": "{0} khóa",
      "apikeys.enabledCount": "{0} khóa đang bật",
      "apikeys.times": "lần",
      "apikeys.namePrompt": "Tên khóa",
      "apikeys.enable": "Bật",
      "apikeys.disable": "Tắt",
      "apikeys.confirmDelete":
        "Sau khi xoá “{0}”, các ứng dụng đang dùng khóa này sẽ không truy cập được. Xoá thật chứ?",
      "apikeys.empty": "Chưa có API Key",

      "rotation.status":
        "Đã cập nhật trạng thái · hai nhóm luân chuyển độc lập, mọi bên gọi dùng chung vị trí hiện tại.",
      "rotation.sweet": "sweet",
      "rotation.aliasLabel": "Bí danh gọi",
      "rotation.current": "Mô hình hiện tại: {0}",
      "rotation.waitingModel": "đang chờ mô hình khả dụng",
      "rotation.col.order": "Thứ tự",
      "rotation.col.state": "Trạng thái / thời điểm hồi phục",
      "rotation.unavailable": "Đã gỡ khỏi danh mục / tạm không dùng được",
      "rotation.pausedUntil": "Tạm dừng đến {0}",
      "rotation.inUse": "Đang dùng",
      "rotation.moveUp": "Lên",
      "rotation.moveDown": "Xuống",
      "rotation.setCurrent": "Đặt làm hiện tại",
      "rotation.useModel": "Dùng {0}",
      "rotation.waitingCatalog": "Đang chờ danh mục mô hình Go",
      "rotation.recentSwitches": "Chuyển đổi gần đây",
      "rotation.noSwitches": "Chưa có bản ghi chuyển đổi",
      "rotation.attemptsEmpty": "Gọi bằng bí danh nhóm để xem 100 lần thử gần nhất",
      "rotation.reason.success": "Thành công",
      "rotation.reason.clientCanceled": "Bên gọi huỷ",
      "rotation.reason.quota": "Hết hạn mức / bị giới hạn tần suất",
      "rotation.reason.transport": "Lỗi kết nối",
      "rotation.reason.firstOutputTimeout": "Timeout output đầu tiên",
      "rotation.reason.streamError": "Output bị gián đoạn",
      "rotation.reason.readError": "Đọc phản hồi thất bại",
      "rotation.reason.conversionError": "Phân tích hoặc chuyển đổi phản hồi thất bại",
      "rotation.reason.requestConversion": "Chuyển đổi request thất bại",
      "rotation.reason.upstreamError": "Lỗi upstream",

      "availability.pending": "Chờ dò",
      "availability.manual": "Thủ công",
      "availability.auto": "Tự động",
      "availability.unverified": "Chưa xác minh",
      "availability.manualControl": "Điều khiển thủ công, không dò tự động",
      "availability.restore": "Khôi phục bật",
      "availability.switchAuto": "Chuyển lại tự động",
      "availability.switchManual": "Chuyển sang thủ công",
      "availability.noneEligible":
        "Danh mục mô hình hiện tại không có mô hình nào đủ điều kiện miễn phí.",
      "availability.channel.anonymous": "Kênh ẩn danh",
      "availability.channel.zen": "Zen Key",
      "availability.reason.probeSucceeded": "Dò thành công",
      "availability.reason.modelUnavailable": "Mô hình không khả dụng",
      "availability.reason.invalidResponse": "Phản hồi không hợp lệ hoặc thiếu",
      "availability.reason.probeTimeout": "Dò quá thời gian",
      "availability.reason.transportError": "Lỗi mạng hoặc proxy",
      "availability.reason.conversionError": "Chuyển đổi request thất bại",
      "availability.reason.creationError": "Không tạo được request",
      "availability.reason.unsupportedProbe": "Giao thức không hỗ trợ dò",
      "availability.reason.noChannel": "Không có kênh dò khả dụng",
      "availability.reason.notFree": "Không phải mô hình miễn phí",
      "availability.reason.canceled": "Đã huỷ",
      "availability.reason.manuallyEnabled": "Bật thủ công",
      "availability.reason.autoProbe": "Dò tự động",
      "availability.reason.legacyRecheck": "Bản ghi cũ, chờ dò lại",
      "availability.reason.http": "HTTP {0}",

      "common.unsaved":
        "Có thay đổi chưa lưu",
      "config.restartNotice":
        "Đã lưu nhưng chưa có hiệu lực: {0}. API hiện tại {1}, WebUI {2}. Vui lòng khởi động lại tiến trình.",
      "config.revealPrompt": "Nhập lại mật khẩu quản trị",

      "toast.refreshed": "Đã làm mới trạng thái",
      "toast.copied": "Đã sao chép",
      "toast.copyDenied": "Trình duyệt không cho phép sao chép, hãy chọn và sao chép thủ công",
      "toast.example.copied": "Đã sao chép ví dụ",
      "toast.config.saved": "Cấu hình đã lưu và có hiệu lực ngay",
      "toast.config.restart": "Đã áp dụng; cần khởi động lại cho {0}",
      "toast.config.reloaded": "Đã nạp lại cấu hình từ đĩa",
      "toast.config.reloadedRestart":
        "Đã nạp lại cấu hình từ đĩa, các trường lắng nghe cần khởi động lại",
      "toast.account.updated": "Đã cập nhật tài khoản, vui lòng đăng nhập lại",
      "toast.play.invalidJson": "JSON của request không hợp lệ: {0}",
      "toast.play.copied": "Đã sao chép phản hồi gốc",
      "toast.apikeys.saved": "Đã lưu và có hiệu lực ngay",
      "toast.apikeys.created": "Đã tạo API Key",
      "toast.rotation.saved": "Đã lưu cấu hình luân chuyển mô hình và có hiệu lực",
      "toast.rotation.switched": "Đã chuyển mô hình hiện tại",
      "toast.rotation.dirty": "Có thay đổi chưa lưu, hãy lưu trước khi làm mới",
    },
  };

let locale = DEFAULT_LOCALE;

const fill = (text, args) =>
  args.reduce((value, item, index) => value.replaceAll(`{${index}}`, String(item)), text);

const lookup = (key) => {
  for (const code of [locale, DEFAULT_LOCALE]) {
    const value = DICTIONARY[code]?.[key];
    if (value !== undefined) return value;
  }
  return undefined;
};

const interpolate = (value, args) => (args.length ? fill(value, args) : value);

// t maps an authored Simplified Chinese string to the active locale. The
// original text is returned whenever the dictionary has no entry, so a page
// module is always readable without consulting the dictionary.
export const t = (original, key, ...args) => {
  const value = lookup(key);
  return interpolate(value === undefined ? original : value, args);
};

// apply re-renders every node carrying a static translation attribute. Any
// node created after a language switch is picked up by the next apply() call.
export const apply = (root = document) => {
  const dictionary = DICTIONARY[locale] || {};
  root.querySelectorAll("[data-i18n]").forEach((node) => {
    if (!originals.has(node)) originals.set(node, node.innerHTML);
    const value = dictionary[node.dataset.i18n];
    node.innerHTML = value === undefined ? originals.get(node) : value;
  });
  root.querySelectorAll("[data-i18n-attr]").forEach((node) => {
    const targets = node.dataset.i18nAttr.split(",").map((pair) => pair.trim().split(":"));
    if (!originals.has(node))
      originals.set(
        node,
        targets.map(([attribute]) => node.getAttribute(attribute) || ""),
      );
    const snapshot = originals.get(node);
    targets.forEach(([attribute, key], index) => {
      const value = dictionary[key];
      node.setAttribute(attribute, value === undefined ? snapshot[index] : value);
    });
  });
};

// syncSelectors keeps the language pickers pointing at the active locale. The
// accessible label is left to apply(): it is a translated string ("Language"),
// not the name of the selected language, so overwriting it here made a screen
// reader announce "简体中文" as the control's name.
const syncSelectors = () => {
  document.querySelectorAll("[data-lang-switch]").forEach((node) => {
    node.value = locale;
  });
};

const notify = () => listeners.forEach((listener) => listener(locale));

export const setLocale = (code, persist = true) => {
  const next = SUPPORTED.includes(code) ? code : DEFAULT_LOCALE;
  const changed = next !== locale;
  if (changed) {
    locale = next;
    if (persist) {
      try {
        localStorage.setItem(STORAGE_KEY, locale);
      } catch (_) {}
    }
    document.documentElement.lang = locale;
    apply();
  }
  syncSelectors();
  if (changed) notify();
};

const stored = () => {
  try {
    return localStorage.getItem(STORAGE_KEY) || "";
  } catch {
    return "";
  }
};

const browserLocale = () => {
  const candidates = [...(navigator.languages || []), navigator.language].filter(Boolean);
  for (const candidate of candidates) {
    const normalized = String(candidate).toLowerCase();
    if (normalized.startsWith("zh")) return "zh-CN";
    if (normalized.startsWith("vi")) return "vi";
    if (normalized.startsWith("en")) return "en";
  }
  return "";
};

// init fills the selectors, applies the stored locale and translates the static
// markup. It never notifies listeners: the shell imports this module after its
// pages are registered, so the first render already happens in the right locale.
export const init = () => {
  document.documentElement.lang = locale;
  document.querySelectorAll("[data-lang-switch]").forEach((node) => {
    node.replaceChildren(
      ...SUPPORTED.map((code) => {
        const option = document.createElement("option");
        option.value = code;
        option.textContent = SHORT[code];
        option.setAttribute("aria-label", LABELS[code]);
        return option;
      }),
    );
    node.onchange = (event) => setLocale(event.target.value);
  });
  const initial = SUPPORTED.includes(stored()) ? stored() : browserLocale();
  if (initial) setLocale(initial, false);
  else apply();
  syncSelectors();
};

export const locales = SUPPORTED;
export const currentLocale = () => locale;
export const localeLabel = (code) => LABELS[code] || code;
export const dateLocale = () => DATE_LOCALES[locale] || locale;

// onChange lets page modules re-render their dynamic text after a switch. It
// returns an unsubscribe function so a page can drop the listener on reset.
export const onChange = (listener) => {
  listeners.push(listener);
  return () => {
    const index = listeners.indexOf(listener);
    if (index >= 0) listeners.splice(index, 1);
  };
};