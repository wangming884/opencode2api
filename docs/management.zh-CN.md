# 管理控制台：模型轮转、Go 额度与 API Key

本文说明新增管理页面的使用方式、配置和错误处理。

## 模型与额度

管理页面 `/#model-quotas`，登录后读取 `GET /api/model-quotas`。

- 模型来自网关同步的 OpenCode Go 目录，支持搜索和复制模型 ID。
- 额度由网关使用已配置的 Go Key 查询 `https://opencode.ai/zen/go/v1/usage`。浏览器不会收到上游 Key。
- 显示 5 小时、每周、每月窗口的剩余百分比和重置时间。接口只提供共享套餐窗口，不提供独立的逐模型余额，不推算金额或可用次数。多个 Key 的额度不相加。
- 成功响应缓存 60 秒，失败缓存 15 秒；失败时保留上次成功结果并明确标记历史数据。没有成功数据时显示未知。
- 页面打开期间每 60 秒查询一次；离开页面或隐藏浏览器时停止周期查询。手动刷新也受服务端缓存约束。

依据：[OpenCode 官方额度接口源码](https://github.com/anomalyco/opencode/blob/dev/packages/console/app/src/routes/zen/go/v1/usage.ts)。该接口未列入正式 API 文档，若响应格式变化，会显示查询异常而非虚构额度。

## API Key 管理

管理页面 `/#api-keys`，支持列表、生成、自定义、重命名、启用、停用、删除和创建后复制环境变量。

- 管理的是网关的 `server_keys`，不修改 OpenCode Go 的上游 Key。
- 自动生成 192 位随机密钥；自定义密钥为 16–256 位英文字母、数字、`-`、`_` 或 `.`。
- 列表只显示名称、尾码、状态、用量和创建时间。创建后可复制完整密钥及环境变量。
- 用量按认证命中的 Key 统计请求数和 token，分近 1 小时和累计两个窗口；`GET /api/monitor` 的 `usage.server_keys` 给出同一份数据，各项之和等于聚合口径。Playground 调试流量单独归入 `playground`，不计入任何 Key。与其他监控数据一样只存于内存：热重载保留，进程重启后清零。
- 变更热应用；停用和删除后，新请求返回 401。在途请求可以完成。至少保留一个启用的 Key。
- 所有管理接口需要登录，写操作需要 CSRF Token。
- 元数据以 `server_key_metadata` 保存在原配置中，按 Key 指纹关联；已有 `server_keys` 自动兼容。

### 按 Key 限流与配额

每个 Key 可单独设置策略，保存在 `server_key_metadata` 的 `limits` 字段；省略即不限。

| 字段             | 含义                                                        |
| ---------------- | ----------------------------------------------------------- |
| `rpm`            | 任意滚动 60 秒内的请求上限。                                |
| `daily_requests` | 每个 UTC 自然日的请求上限。                                 |
| `daily_tokens`   | 每个 UTC 自然日的 token 额度。                              |
| `models`         | 允许的模型 ID 列表；`gpt-*` 这样的后缀通配按前缀匹配。      |

- 前两项在**路由前**拒绝，返回 429 并带 `Retry-After`，不触达上游。第三项是预算而非硬上限：
  用量只有上游上报后才知道，所以跨过额度的那次请求仍会完成，之后的请求被拒绝。
- 计数按 Key 指纹保存，配置热重载不会清零（共享同一个计数器）；每 30 秒与关闭时写入
  `config.json.limits.json`，因此重启进程不会重置当日额度。滚动分钟窗口不落盘，载入时
  早于当前 UTC 日的记录会被丢弃。
- 只有推理端点计入额度：`/v1/chat/completions`、`/v1/responses`、`/v1/messages`、
  `/v1/systemone`、`/v1/embeddings`。`/v1/models`、`/healthz`、`/metrics` 与 Playground
  调试流量不占额度。
- 因模型白名单被拒绝的请求会**退还**它占用的额度：策略拒绝不是调用方的流量，否则客户端
  用错模型名就会把自己的配额耗尽并连带锁死正常请求。
- 管理接口：`PATCH /api/api-keys/{id}` 传 `limits` 设置，传 `{}` 清除；省略则保持原值。
  `GET /api/api-keys` 的每项返回 `limits` 与 `today`（当前滚动分钟请求数、当日请求数与
  token 数），页面据此显示「今日 / 限额」。

### 到期时间

- `server_key_metadata` 的 `expires_at`（RFC3339）为到期时刻；省略表示永不过期。
- 到期与停用相互独立：停用是主动开关，到期是时间到点。两者都会让请求返回 401，但提示不同
  （`this API key has expired` 与 `invalid local API key`），页面状态列显示「已过期」。
- Key 不会被自动删除，延长有效期即可恢复，无需重新签发。
- 同一 `PATCH` 接口：`expires_at` 省略保持不变、传空字符串清除、传时间则设置。创建时也可
  直接带上。页面用 `datetime-local` 输入，按浏览器时区显示，提交时转换为 UTC。

## 监控指标与 embeddings

管理页 `/#metrics`，同一页面管理两个独立能力：Prometheus 抓取端点和 OpenAI 兼容的
embeddings 中继。

### Prometheus 抓取

- 配置项 `metrics.enabled`，默认 `false`；关闭时 `GET /metrics` 返回 404。
- 端点挂在 **API 监听端口**上，因此 WebUI 关闭时依然可抓取；单端口部署时
  `/metrics` 与 `/v1`、`/healthz` 一起归 API 处理。
- 抓取使用已配置的 `server_keys` 认证：`Authorization: Bearer <server_key>`。暴露内容
  包含层级、上游 Key 与流量信息，因此所有启用的 Key 都能读取，请只在可信网络中开启。
- 输出为标准文本格式（version 0.0.4），不依赖任何客户端库。计数为进程生命周期累计值，
  重启后归零，与 WebUI 监控一致；延迟分位只覆盖保留的近一小时，且为分桶上界。
- `/metrics` 与 `/healthz` 都不计入请求统计。否则抓取器每十几秒一次的请求会抬高它自己
  读取的成功率与延迟。
- 管理接口 `GET /api/metrics` 读取状态、`PUT /api/metrics` 开关、`GET /api/metrics/preview`
  渲染页面展示的文本。三者都需要登录，写操作需要 CSRF；预览在端点关闭时仍可用。
- 花费指标 `opencode2api_cost_usd_total` 与 `opencode2api_server_key_cost_usd_total` 按模型
  目录价格逐请求计算：未命中缓存的输入按输入价、缓存读写按各自价格、输出按输出价；目录未
  定价的模型记 0，缺少缓存价格时回退按输入价计算。概览页与 API Key 列表也会显示对应金额。

### embeddings 中继

- 配置项 `embeddings.enabled` 控制开关，关闭时 `POST /v1/embeddings` 返回 404。
- 请求与响应都按 OpenAI 结构原样转发，不做协议转换；只做路由、Key 轮换、层级回退和统计。
- 模型 ID 不经过模型目录过滤：embedding 模型不是聊天模型，目录的协议过滤恰好会拒掉这个
  端点要服务的 ID。能否生成向量由上游判断，上游的拒绝原样返回。
- `stream` 参数直接返回 400，因为 OpenAI embeddings 从不流式返回。
- 两种上游：
  - 未配置 `embeddings.base_url`：走 Zen / Go Key 池，按 `prefer` 选择层级并轮换 Key。
    **OpenCode Zen 本身没有该端点**，因此这种情况下中继会如实返回上游的 404。
  - 配置 `embeddings.base_url`：转发到该 OpenAI 兼容地址并使用 `embeddings.api_key`，
    仅在 429 与 5xx 重试，每次重试轮换代理出口。地址可写 `https://host/v1` 或
    `https://host/v1/embeddings`。
- 响应体上限 64 MiB；超过时返回 502，而不是把截断的 JSON 当成成功结果下发。
- 用量按响应中的 `usage.prompt_tokens` 计入 token 统计；独立上游的尝试记为
  `tier=external` / `channel=external`，不混入 zen / go 的统计。
- 管理接口 `PUT /api/embeddings`，请求体 `{"enabled":true,"base_url":"…","api_key":"…"}`。
  `api_key` 省略表示保持原值，传空字符串表示清除；页面永远不会收到已保存的 Key。

注意：从配置中心保存配置不会影响这两项设置，它们由各自的接口管理。

## 验证和构建

```bash
go test ./...
go test -race ./...
go vet ./...
for f in webui/js/*.js webui/js/pages/*.js; do node --input-type=module --check < "$f"; done
go build -trimpath -o opencode2api ./cmd/opencode2api
```

升级前请备份程序、配置及轮转状态，再停止服务并替换可执行文件。旧版本不认识新增的 `server_key_metadata` 和 `rotation` 字段；回退时应同时恢复旧程序和旧配置。

## SOTA / 甜点模型轮转

管理页：`/#rotation`。调用时继续使用 网关的 Base URL（默认
`http://127.0.0.1:8080/v1`）和已有 API Key，将 `model` 设为 `sota` 或
`sweet`。两个别名列入 `/v1/models`，支持 Chat Completions、Responses 和
Messages，以及这三种协议的流式输出。真实模型 ID 的原有调用路径保持不变。

- SOTA 固定成员：`deepseek-v4.1-flash`、`glm-5.3`、`grok-4.6`、`kimi-k3`、
  `qwen3.8-max`，默认按上述顺序。
- 甜点组包含其余 Go 可用模型，初次以 `glm-5.3-flash` 为首，随后按名称排序。
  新模型追加在末尾；下架或协议不受支持的模型保留在列表，但不参与调用。
- 每组共享一个当前模型。成功后保持位置；失败切换后，下次从新位置开始。
  并发请求通过位置版本号避免旧请求把游标移回去；手动切换不打断在途回答。
- 普通错误每模型最多 3 次（包含首次），每请求最多消耗 3 个普通错误模型预算。
  模型耗尽重试后暂停 15 秒。分组请求不使用原有 Key 重试、跨 Tier 降级、
  自动重定向或 stale-reasoning 补发，也不因单个模型故障暂停整个 Go Key。
- 429 或明确额度错误直接跳到下一模型，不新增普通错误模型计数；已经发生的
  普通错误仍计数。允许遍历整组，同一请求不重复模型。暂停时间采用 Retry-After，
  未提供时为 60 秒。共享套餐额度只用于展示，不能保证换模型后就有余额。
- 整体超时沿用 `retry.timeout_seconds`。`rotation.first_output_seconds` 默认
  30 秒，限制每次等待首个有效输出的时间，不限制已经开始的正常流式回答。
- 流式 start/role/usage 前导事件会等到有效输出或正常结束后再提交。提交前错误
  可重试；提交后中断会保留已输出内容、追加协议对应的 SSE 错误并结束，随后
  推进当前位置，下一次请求使用下一个模型。不会在同一响应里拼接替代答案。
  调用方取消不会触发暂停或切换。
- 响应 JSON / SSE 的 `model` 保持请求别名，`X-Resolved-Model` 标记实际模型。
  用量按实际上游模型记录。轮转日志与管理页面仅保留分类后的错误原因，
  不记录上游错误正文、请求正文或密钥。最近保留 100 条尝试和每组 30 条切换。

配置保存于 `config.json` 的 `rotation` 字段：`sota` / `sweet` 各有 `alias`
和 `order`；状态单独原子写入 `config.json.rotation.json`（0600），包括当前
位置、版本、动态追加的顺序、暂停期限和最近记录。修改 Key 或其他配置时
沿用同一个轮转状态管理器。页面展示状态写入失败提示，首次无法写入则拒绝启动。

管理接口均需要现有管理 Session，写入另需 CSRF：

- `GET /api/rotation`：分组配置、可用状态、暂停期限、切换和尝试记录。
- `PUT /api/rotation`：提交 `sota` / `sweet` 的 `alias`、完整 `order`，以及
  `first_output_seconds`；别名不得重复或与真实模型 ID 冲突，成员不得跨组。
- `POST /api/rotation/current`：`{"group":"sota","model":"glm-5.3"}`；立即
  修改当前模型并解除该模型的临时暂停。

验证：`go test -race ./...`、`go vet ./...`、`for f in webui/js/*.js webui/js/pages/*.js; do node --input-type=module --check < "$f"; done`。
模拟上游测试覆盖 9 次普通错误上限、超过 3 个额度跳过、混合错误预算、
Retry-After、单个模型故障不影响 Key、三种入口协议的跨协议转换、成功保持、
别名发现与用量归属、畸形响应、输出前错误、首个输出超时、输出后故障、
调用方取消、在途回答不受手动切换影响、并发版本保护、下架和重启恢复。
上线前建议使用独立配置及端口验证管理登录、CSRF、页面排序与保存、别名冲突、
Key 配置热更新和真实 Go 请求。测试与正式配置、凭证、日志及运行状态不应提交到仓库。
