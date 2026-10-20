// Page: API Key 管理 — create, rename, enable/disable and delete caller keys.
import { api, derivedAPIBase, hasSession } from "../api.js";
import { loadConfig } from "../data.js";
import * as store from "../store.js";
import { t, onChange, dateLocale } from "../i18n.js";
import { $, el, emptyRow, formatCost, formatNumber, localTime, confirmDialog, copyText, openDialog, promptDialog, toast } from "../ui.js";

let keysLoading = false;
let createdKey = "";
let lastKeys = null;

async function load() {
  if (keysLoading) return;
  keysLoading = true;
  $("#api-key-refresh").disabled = true;
  try {
    const data = await api("/api/api-keys");
    if (hasSession()) {
      lastKeys = data.keys;
      render(data.keys);
    }
  } catch (error) {
    $("#api-key-status").textContent = t(
      `读取失败：${error.message}`,
      "apikeys.loadFailed",
      error.message,
    );
  } finally {
    keysLoading = false;
    $("#api-key-refresh").disabled = false;
  }
}

async function mutate(id, method, body, button) {
  button.disabled = true;
  try {
    const data = await api(`/api/api-keys/${encodeURIComponent(id)}`, {
      method,
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (!hasSession()) return;
    lastKeys = data.keys;
    render(data.keys);
    await loadConfig();
    toast(t("已保存并立即生效", "toast.apikeys.saved"));
  } catch (error) {
    toast(error.message, true);
  } finally {
    button.disabled = false;
  }
}

// toLocalInput formats a stored timestamp for a datetime-local input, which
// works in the browser's zone; the server always stores UTC.
function toLocalInput(value) {
  if (!value) return "";
  const date = new Date(value);
  const pad = (part) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

// limitsSummary renders a key's policy in one line, or "unlimited" when it has
// none. Every field is optional, so the line lists only what is set.
function limitsSummary(limits) {
  if (!limits) return t("不限", "apikeys.limits.unlimited");
  const parts = [];
  if (limits.rpm) parts.push(t(`${limits.rpm} 次/分`, "apikeys.limits.rpmShort", limits.rpm));
  if (limits.daily_requests)
    parts.push(
      t(`${formatNumber(limits.daily_requests)} 次/日`, "apikeys.limits.dailyShort", formatNumber(limits.daily_requests)),
    );
  if (limits.daily_tokens)
    parts.push(
      t(`${formatNumber(limits.daily_tokens)} tokens/日`, "apikeys.limits.tokensShort", formatNumber(limits.daily_tokens)),
    );
  if (limits.models?.length)
    parts.push(t(`${limits.models.length} 个模型`, "apikeys.limits.modelsShort", limits.models.length));
  return parts.length ? parts.join(" · ") : t("不限", "apikeys.limits.unlimited");
}

// openLimitsDialog edits one key's policy. Saving sends the whole object;
// clearing sends an empty one, which the server normalizes away.
function openLimitsDialog(key, button) {
  const limits = key.limits || {};
  const numberField = (id, label, value, hint) => {
    const input = el("input", { id, type: "number", min: "0", step: "1", value: value || "" });
    return el("div", { class: "field" }, [
      el("label", { for: id, text: label }),
      input,
      el("p", { class: "section-caption", text: hint }),
    ]);
  };
  const modelsInput = el("textarea", {
    id: "limit-models",
    rows: "4",
    placeholder: t("每行一个模型 ID，支持 gpt-* 这样的前缀", "apikeys.limits.modelsPlaceholder"),
  });
  modelsInput.value = (limits.models || []).join("\n");
  const expiryInput = el("input", {
    id: "limit-expiry",
    type: "datetime-local",
    value: toLocalInput(key.expires_at),
  });

  const body = el("div", { class: "stack" }, [
    numberField("limit-rpm", t("每分钟请求上限", "apikeys.limits.rpm"), limits.rpm, t("0 表示不限。", "apikeys.limits.zeroHint")),
    numberField(
      "limit-daily",
      t("每日请求上限", "apikeys.limits.daily"),
      limits.daily_requests,
      t("按 UTC 自然日统计，0 表示不限。", "apikeys.limits.dailyHint"),
    ),
    numberField(
      "limit-tokens",
      t("每日 token 额度", "apikeys.limits.tokens"),
      limits.daily_tokens,
      t("只统计上游上报的用量；超出后拒绝后续请求，0 表示不限。", "apikeys.limits.tokensHint"),
    ),
    el("div", { class: "field" }, [
      el("label", { for: "limit-models", text: t("模型白名单", "apikeys.limits.models") }),
      modelsInput,
      el("p", { class: "section-caption", text: t("留空表示允许全部模型。", "apikeys.limits.modelsHint") }),
    ]),
    el("div", { class: "field" }, [
      el("label", { for: "limit-expiry", text: t("到期时间", "apikeys.expiry.field") }),
      expiryInput,
      el("p", { class: "section-caption", text: t("留空表示永不过期；到期后该 Key 立即停止认证。", "apikeys.expiry.hint") }),
    ]),
  ]);

  const collect = () => ({
    limits: {
      rpm: Number($("#limit-rpm").value) || 0,
      daily_requests: Number($("#limit-daily").value) || 0,
      daily_tokens: Number($("#limit-tokens").value) || 0,
      models: modelsInput.value
        .split(/[\n,]/)
        .map((value) => value.trim())
        .filter(Boolean),
    },
    // An empty field clears the expiry; the server keeps the stored one only
    // when the field is absent, which this form never does.
    expires_at: expiryInput.value ? new Date(expiryInput.value).toISOString() : "",
  });

  openDialog({
    title: t(`限额与有效期 · ${key.name}`, "apikeys.limits.title", key.name),
    body,
    actions: [
      { label: t("取消", "common.cancel") },
      {
        label: t("清除限额", "apikeys.limits.clear"),
        onClick: (close) => {
          close();
          mutate(key.id, "PATCH", { limits: {}, expires_at: "" }, button);
        },
      },
      {
        label: t("保存", "common.save"),
        kind: "btn-primary",
        onClick: (close) => {
          close();
          const values = collect();
          mutate(key.id, "PATCH", values, button);
        },
      },
    ],
  });
}

function render(keys) {
  keys = keys || [];
  $("#api-key-count").textContent = t(`${keys.length} 个`, "apikeys.count", keys.length);
  $("#api-key-status").textContent = t(
    `${keys.filter((key) => key.enabled).length} 个已启用`,
    "apikeys.enabledCount",
    keys.filter((key) => key.enabled).length,
  );
  const host = $("#api-key-list");
  host.replaceChildren();
  for (const key of keys) {
    const usage = key.usage || {};
    const summary = (value = {}) =>
      `${formatNumber(value.requests)} ${t("次", "apikeys.times")} · ${formatNumber(value.total_tokens)} tokens · ${formatCost(value.cost_usd)}`;
    const rename = el("button", { type: "button", class: "btn btn-sm btn-ghost", text: t("重命名", "common.rename") });
    rename.addEventListener("click", async () => {
      const name = await promptDialog({
        title: t("重命名 API Key", "common.rename"),
        label: t("密钥名称", "apikeys.namePrompt"),
        value: key.name,
        confirmLabel: t("保存", "common.save"),
      });
      if (name !== null) mutate(key.id, "PATCH", { name }, rename);
    });
    const toggle = el("button", {
      type: "button",
      class: "btn btn-sm btn-ghost",
      text: key.enabled ? t("停用", "apikeys.disable") : t("启用", "apikeys.enable"),
    });
    toggle.addEventListener("click", () => mutate(key.id, "PATCH", { enabled: !key.enabled }, toggle));
    const remove = el("button", { type: "button", class: "btn btn-sm btn-danger", text: t("删除", "common.delete") });
    const limitsButton = el("button", {
      type: "button",
      class: "btn btn-sm btn-ghost",
      text: t("限额", "apikeys.limits.button"),
    });
    limitsButton.addEventListener("click", () => openLimitsDialog(key, limitsButton));
    remove.addEventListener("click", async () => {
      const ok = await confirmDialog({
        title: t("删除 API Key", "common.delete"),
        message: t(
          `删除「${key.name}」后，使用它的应用将无法继续访问。确定删除？`,
          "apikeys.confirmDelete",
          key.name,
        ),
        confirmLabel: t("删除", "common.delete"),
        danger: true,
      });
      if (ok) mutate(key.id, "DELETE", null, remove);
    });
    host.append(
      el("tr", {}, [
        el("td", { text: key.name }),
        el("td", { class: "mono", text: key.display }),
        el("td", {
          class: `status ${key.expired || !key.enabled ? "warn" : "ok"}`,
          text: key.expired
            ? t("已过期", "apikeys.expired")
            : key.enabled
              ? t("已启用", "state.enabled")
              : t("已停用", "state.disabled"),
        }),
        el("td", { text: `${summary(usage.last_hour)} / ${summary(usage.lifetime)}` }),
        el("td", {}, [
          el("div", {
            text: t(
              `今日 ${formatNumber(key.today?.day_requests)} 次 · ${formatNumber(key.today?.day_tokens)} tokens`,
              "apikeys.limits.today",
              formatNumber(key.today?.day_requests),
              formatNumber(key.today?.day_tokens),
            ),
          }),
          el("div", {
            class: "section-caption",
            text: `${limitsSummary(key.limits)} · ${
              key.expires_at
                ? t(
                    `至 ${new Date(key.expires_at).toLocaleDateString(dateLocale())}`,
                    "apikeys.expiry.until",
                    new Date(key.expires_at).toLocaleDateString(dateLocale()),
                  )
                : t("永不过期", "apikeys.expiry.never")
            }`,
          }),
        ]),
        el("td", { text: key.created_at ? localTime(key.created_at) : t("已有配置", "apikeys.col.legacy") }),
        el("td", {}, [el("div", { class: "key-actions" }, [limitsButton, rename, toggle, remove])]),
      ]),
    );
  }
  if (!keys.length) emptyRow(host, 7, t("暂无 API Key", "apikeys.empty"));
}

async function create(event) {
  event.preventDefault();
  const button = $("#api-key-create");
  button.disabled = true;
  try {
    const data = await api("/api/api-keys", {
      method: "POST",
      body: JSON.stringify({ name: $("#api-key-name").value, value: $("#api-key-value").value }),
    });
    if (!hasSession()) return;
    createdKey = data.key;
    const goModels = store.get("debug")?.models?.filter((item) => item.available_go) || [];
    const model =
      goModels.find((item) => item.model === "glm-5.3-flash")?.model ||
      goModels[0]?.model ||
      "YOUR_MODEL";
    $("#api-key-env").textContent = `OPENAI_API_KEY=${data.key}\nOPENAI_BASE_URL=${derivedAPIBase()}/v1\nOPENAI_MODEL=${model}`;
    $("#api-key-created").classList.remove("hidden");
    $("#api-key-form").reset();
    lastKeys = data.keys;
    render(data.keys);
    await loadConfig();
    toast(t("API Key 已创建", "toast.apikeys.created"));
  } catch (error) {
    toast(error.message, true);
  } finally {
    button.disabled = false;
  }
}

function resetCreated() {
  createdKey = "";
  lastKeys = null;
  $("#api-key-env").textContent = "";
  $("#api-key-created").classList.add("hidden");
  $("#api-key-value").value = "";
}

export default {
  id: "api-keys",
  title: "API Key 管理",
  sub: "给每个调用方独立的访问密钥。",
  init() {
    $("#api-key-form").addEventListener("submit", create);
    $("#api-key-refresh").addEventListener("click", load);
    $("#api-key-dismiss").addEventListener("click", resetCreated);
    $("#api-key-copy").addEventListener("click", () => copyText(createdKey));
    $("#api-key-copy-env").addEventListener("click", () => copyText($("#api-key-env").textContent));
    // Re-render the rows in place so the key that was just created stays on
    // screen instead of being wiped by a reload.
    onChange(() => {
      if (lastKeys) render(lastKeys);
    });
  },
  show: load,
  refresh: load,
  reset: resetCreated,
};
