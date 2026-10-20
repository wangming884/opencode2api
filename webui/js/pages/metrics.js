// Page: 监控指标 — the Prometheus scrape endpoint and the embeddings relay.
import { api, derivedAPIBase, hasSession } from "../api.js";
import { t, onChange } from "../i18n.js";
import { $, copyText, toast } from "../ui.js";

let status = null;
let preview = "";
let saving = false;

function embeddingsExample() {
  const base = derivedAPIBase();
  return [
    `curl ${base}/v1/embeddings \\`,
    `  -H "Authorization: Bearer <server_key>" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '{"model":"MODEL_ID","input":"hello world"}'`,
  ].join("\n");
}

function render() {
  const enabled = Boolean(status?.enabled);
  const toggle = $("#metrics-toggle");
  toggle.textContent = enabled ? t("停用", "metrics.disable") : t("启用", "metrics.enable");
  toggle.classList.toggle("btn-primary", !enabled);
  toggle.disabled = saving;
  $("#metrics-url").textContent = status?.url || "—";
  $("#metrics-status").textContent = enabled
    ? t("已启用：抓取器可用上方地址读取指标，改动立即生效。", "metrics.enabled")
    : t("未启用：/metrics 返回 404。启用后立即生效，无需重启。", "metrics.disabled");
  $("#metrics-preview").textContent = preview || "—";
  $("#metrics-preview-size").textContent = preview
    ? t(`${preview.split("\n").length} 行`, "metrics.preview.lines", preview.split("\n").length)
    : "";
  $("#metrics-embeddings").textContent = embeddingsExample();
  renderEmbeddings();
}

function renderEmbeddings() {
  const embeddings = status?.embeddings || {};
  $("#embeddings-enabled").checked = Boolean(embeddings.enabled);
  // The stored base URL is only written back when the form is not being
  // edited, so re-rendering on a language switch cannot overwrite typing.
  if (document.activeElement !== $("#embeddings-base")) $("#embeddings-base").value = embeddings.base_url || "";
  $("#embeddings-key-hint").textContent = embeddings.has_key
    ? t("已保存上游 Key；留空保持不变，输入新值则替换。", "metrics.embeddings.keySaved")
    : t("尚未保存上游 Key；留空则不带鉴权头。", "metrics.embeddings.keyEmpty");
  $("#embeddings-status").textContent = embeddings.enabled
    ? t(`已启用，上游：${embeddings.upstream}`, "metrics.embeddings.on", embeddings.upstream)
    : t("未启用：/v1/embeddings 返回 404。", "metrics.embeddings.off");
}

async function loadPreview() {
  try {
    const value = await api("/api/metrics/preview");
    if (!hasSession()) return;
    preview = value.text || "";
    $("#metrics-preview").textContent = preview || "—";
    $("#metrics-preview-size").textContent = preview
      ? t(`${preview.split("\n").length} 行`, "metrics.preview.lines", preview.split("\n").length)
      : "";
  } catch (error) {
    $("#metrics-preview").textContent = error.message;
  }
}

async function load() {
  try {
    status = await api("/api/metrics");
    if (!hasSession()) return;
    render();
    await loadPreview();
  } catch (error) {
    $("#metrics-status").textContent = error.message;
  }
}

async function toggle() {
  if (!status || saving) return;
  saving = true;
  render();
  try {
    status = await api("/api/metrics", {
      method: "PUT",
      body: JSON.stringify({ enabled: !status.enabled }),
    });
    toast(status.enabled ? t("指标已启用", "metrics.toast.on") : t("指标已停用", "metrics.toast.off"));
  } catch (error) {
    toast(error.message, true);
  } finally {
    saving = false;
    render();
    await loadPreview();
  }
}

async function saveEmbeddings(event) {
  event.preventDefault();
  const button = $("#embeddings-save");
  button.disabled = true;
  try {
    const body = {
      enabled: $("#embeddings-enabled").checked,
      base_url: $("#embeddings-base").value.trim(),
    };
    // An omitted api_key keeps the stored one: the page never receives the
    // secret, so it must not send an empty string that would clear it.
    const key = $("#embeddings-key").value.trim();
    if (key) body.api_key = key;
    status = await api("/api/embeddings", { method: "PUT", body: JSON.stringify(body) });
    $("#embeddings-key").value = "";
    render();
    toast(t("embeddings 设置已保存", "metrics.embeddings.saved"));
  } catch (error) {
    toast(error.message, true);
  } finally {
    button.disabled = false;
  }
}

export default {
  id: "metrics",
  title: "监控指标",
  sub: "Prometheus 抓取端点与 embeddings 中继的接入状态。",
  init() {
    $("#metrics-toggle").addEventListener("click", toggle);
    $("#metrics-refresh").addEventListener("click", async () => {
      await load();
      toast(t("状态已刷新", "toast.refreshed"));
    });
    $("#metrics-copy-url").addEventListener("click", () => copyText($("#metrics-url").textContent));
    $("#metrics-copy-preview").addEventListener("click", () => copyText(preview));
    $("#metrics-copy-embeddings").addEventListener("click", () =>
      copyText($("#metrics-embeddings").textContent),
    );
    $("#embeddings-form").addEventListener("submit", saveEmbeddings);
    // Only the rendered values are rebuilt on a language switch: a toggle that
    // is mid-request keeps its disabled state.
    onChange(() => render());
  },
  show() {
    load();
  },
  refresh() {
    load();
  },
};
