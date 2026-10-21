// fetch wrapper: JSON handling, CSRF for writes, 401 routed to the login view.
import * as store from "./store.js";
import { t } from "./i18n.js";

let csrf = "";
let onUnauthorized = null;

export function setCsrf(value) {
  csrf = value || "";
}

export function clearCsrf() {
  csrf = "";
}

export function hasSession() {
  return csrf !== "";
}

export function setUnauthorizedHandler(handler) {
  onUnauthorized = handler;
}

// A hung backend used to leave the console on its loading state forever: the
// button stayed disabled and "reading…" never resolved. Every request now has a
// deadline. The playground is the one endpoint that legitimately runs long,
// because it waits on a real upstream call.
const DEFAULT_TIMEOUT_MS = 30_000;
const TIMEOUT_BY_PATH = { "/api/debug/inference": 180_000 };

export async function api(path, options = {}) {
  // timeout is ours, not fetch's; keep it out of the request init.
  const { timeout, ...init } = options;
  const headers = { ...(init.headers || {}), Accept: "application/json" };
  if (init.body) headers["Content-Type"] = "application/json";
  if (csrf && !["GET", "HEAD"].includes((init.method || "GET").toUpperCase()))
    headers["X-CSRF-Token"] = csrf;
  const limit = timeout ?? TIMEOUT_BY_PATH[path] ?? DEFAULT_TIMEOUT_MS;
  const controller = new AbortController();
  // The deadline covers the body as well: fetch resolves as soon as the headers
  // arrive, so clearing the timer there would leave a stalled body hanging
  // forever on a proxy that never finishes the response.
  const timer = setTimeout(() => controller.abort(), limit);
  try {
    const response = await fetch(path, { ...init, headers, signal: controller.signal });
    if (response.status === 204) return null;
    const data = await response.json().catch(() => ({}));
    if (!response.ok) {
      // The login endpoint answers 401 for bad credentials: surface its message
      // instead of treating the attempt as an expired session.
      if (response.status === 401 && path !== "/api/auth/login") {
        if (onUnauthorized) onUnauthorized();
        throw new Error(t("登录已失效", "error.session"));
      }
      throw new Error(data.error?.message || `HTTP ${response.status}`);
    }
    return data;
  } catch (error) {
    if (error?.name === "AbortError") {
      // Keep the fraction for sub-second deadlines instead of rounding them to
      // "0 seconds"; the shipped limits are whole seconds, so they stay clean.
      const seconds = limit % 1000 === 0 ? String(limit / 1000) : (limit / 1000).toFixed(1);
      throw new Error(t(`请求超时（${seconds} 秒）`, "error.timeout", seconds));
    }
    throw error;
  } finally {
    clearTimeout(timer);
  }
}

// Client-facing API base derived from the configured listen address.
export function derivedAPIBase() {
  const config = store.get("config");
  const listen = config?.effective?.listen || config?.listen || ":8080";
  const match = listen.match(/^\[.*\]:(\d+)$/) || listen.match(/:(\d+)$/);
  const port = match?.[1] || "8080";
  return `${location.protocol}//${location.hostname}:${port}`;
}
