# New API Hamster 补丁技术设计

## 1. 设计目标与边界

本设计以 `QuantumNous/new-api@1721144221ec5c94dd87891a7ae1bee228e7bb63` 为唯一上游基线，在 New API 内实现配置订阅的完整生命周期，并输出 Hamster Switch `format_version: 1` 订阅。

实现只修改 New API、`server-integrations` 的补丁/安装/发布资产和项目文档。Hamster Switch 客户端协议保持不变；空 provider 的专门客户端提示、Claude Code 与 Claude Desktop 的额外映射规则、Linux arm64 产物均不在本轮范围。

旧 `new-api-v1.0.0`、旧 `5a6c53d` 数据表和 `hs_` URL 不参与迁移或兼容。

## 2. 架构边界

### 2.1 New API 后端

- `model`：GORM 实体、跨 SQLite/MySQL/PostgreSQL 约束和事务仓储。
- `service/hamsterswitch`：订阅生命周期、动态 provider 发现、模板渲染、渠道绑定凭证、教程内容和诊断。
- `controller`：普通用户、管理员和公开下载 DTO；控制权限、缓存头、审计和错误状态。
- `router`：独立 `/api/hamster-switch` 命名空间，不复用 New API 付费套餐 `/subscription` 领域。
- `middleware`/渠道分发：只为内部 Hamster provider 凭证增加受限的指定渠道上下文；普通 Token 和管理员现有行为不变。

### 2.2 New API 前端

当前上游只有一套 `web/src`。新增配置订阅页面、模板管理、教程管理和渠道应用映射控件必须进入现有路由、权限、i18n、主题和组件体系，不创建 `web/default`/`web/classic` 分支实现。

### 2.3 发布仓库

`server-integrations` 负责固定上游指纹、确定性补丁、签名 manifest、Linux amd64 预构建镜像/二进制、版本绑定安装器、校验和、备份、健康检查和回滚。

## 3. 数据模型

表名使用独立 `hamster_` 前缀，避免与 New API 原有付费订阅冲突。最终字段名可按当前上游 GORM 约定校准，但以下身份和唯一性不可改变。

### 3.1 配置订阅

`hamster_config_subscriptions`

- 内部主键、不可枚举 `public_id`、用户 ID、名称、状态。
- 高熵 `download_token`，作为无登录下载 bearer secret。
- 创建/更新时间和软删除字段。
- `public_id`、`download_token` 全局唯一；`user_id`、状态和删除时间建索引。

`hamster_config_subscription_groups`

- 订阅 ID、稳定的分组绑定 ID、目标 New API 分组 key、底层计费 Token ID、排序位置。
- 同一订阅内目标分组唯一，计费 Token ID 唯一。
- 分组绑定 ID 在名称、模板展示字段和渠道变化时不变；用户删除再重新添加分组时视为新身份。

每个订阅分组只创建一个隐藏计费 Token，沿用 New API 的用户、目标分组、配额、限速、到期和用量扣减逻辑。不能为每个渠道复制计费 Token，否则会放大可用配额。

### 3.2 渠道绑定凭证

`hamster_provider_credentials`

- 分组绑定 ID、New API 渠道 ID、不可预测的 provider 访问 key、状态和时间戳。
- `(group_binding_id, channel_id)` 唯一，访问 key 全局唯一。
- 关联底层计费 Token 通过分组绑定获得，不重复存储配额。

该凭证只用于 YAML 的 `api_key`/`settings_config`，不是普通 Token。认证成功后写入：用户、底层计费 Token、目标分组、固定渠道 ID 和 `hamster_provider_credential=true` 上下文。分发层必须重新验证渠道启用、渠道属于目标分组、请求模型由该渠道在该分组下启用。任一条件失败即返回明确错误，不进入同组随机选择或跨渠道重试。

凭证不能使用可由用户修改的明文渠道后缀。下载时的并发调和使用唯一约束加冲突重读，保证同一分组×渠道始终返回同一 key。

### 3.3 渠道导入应用设置

`hamster_channel_exports`

- New API 渠道 ID 唯一。
- `apps` 以规范化 JSON 数组保存，只允许 `claude`、`claude-desktop`、`codex`、`gemini`，至少一项。
- 无记录时逻辑默认值为四项全选；恢复默认可删除覆盖记录。

渠道删除后记录通过应用层清理；读取时始终以现存渠道为准，孤儿记录不能生成 provider。

### 3.4 模板、展示覆盖与教程

沿用 sub2api 已验证的实体职责：

- YAML 模板草稿和不可变版本；同一时间只有一个启用版本。
- `presentation` JSON 保存订阅级覆盖和按“分组 key × 渠道 ID”定位的 provider 覆盖。
- 教程已签名远程快照、站点覆盖、候选冲突和图片缓存元数据。
- 站点设置包含“在普通 Token 列表显示 Hamster 专用 Token”，默认 `false`。

模板历史不保存渠道密钥、provider 访问 key 或下载 token。模板上下文使用闭合集合，敏感值只在最终下载渲染阶段注入。

## 4. Provider 发现与稳定身份

### 4.1 发现条件

对订阅每个所选分组读取当前渠道/能力状态。只有同时满足下列条件的渠道生成 provider：

1. 渠道存在且启用。
2. 渠道在目标分组下存在启用的 ability。
3. 至少有一个可请求模型。

模型集合以目标分组可实际请求的 ability 为准，去重并稳定排序；不能仅复制渠道原始 `models` 后暴露该分组不可用的模型。渠道不满足条件时普通用户只看到“当前暂无可用渠道”或 provider 缺失，管理员预览获得结构化跳过原因。

### 4.2 Provider ID

服务端 provider ID 使用不可变身份组合：

```text
new-api-hamster:<subscription-public-id>:<group-binding-id>:channel:<channel-id>
```

实际编码使用 URL/YAML 安全字符并限制长度。ID 不包含分组名称、渠道名称、模板名称、应用列表或倍率，因此这些显示字段变化不会导致 Hamster Switch 把 provider 误判为新增项。Hamster Switch 展开 `general` provider 时追加应用身份，仍保持可预测稳定。

### 4.3 动态调和

每次公共下载和管理员预览都读取当前启用模板、渠道、abilities、倍率和应用映射。公共下载在事务内为新增的分组×渠道补齐访问凭证，并对已失效绑定停用或清理；管理员纯预览使用脱敏占位 key，不创建真实凭证。

新增渠道会在下一次刷新出现；渠道禁用、删除或失去全部模型后会消失。失效访问 key 即使仍被旧客户端保存，也会因运行时校验而拒绝请求。

允许生成 `providers: []`。New API 页面负责空状态提示；Hamster Switch 当前拒绝空 provider 列表，该客户端行为不在本轮修改。

## 5. YAML 契约

每个服务端 provider 对应一个“订阅分组 × 渠道”，主要字段如下：

```yaml
id: <stable-id>
app_type: general
apps: [claude, claude-desktop, codex, gemini]
group_name: <target-group>
name: <channel-display-name>
models: [<enabled-models>]
base_url: <public-new-api-base-url>
api_key: <opaque-provider-credential>
pricing:
  multiplier: 1
settings_config:
  claude: {env: {}}
  claude-desktop: {env: {}}
  codex: {auth: {}, config: "..."}
  gemini: {env: {}}
endpoints:
  - url: <same-public-base-url>
```

- `apps` 始终显式输出，管理员渠道覆盖替换默认四项全选。
- `pricing.multiplier` 使用订阅所属用户访问目标分组时的有效倍率：特殊倍率优先，否则使用分组基础倍率；包括 `1` 在内始终输出。
- `general` provider 使用 Hamster Switch 已支持的 `claude`、`claude-desktop`、`codex`、`gemini` 子对象分别提供应用配置。服务端按 New API 的 Anthropic、Responses 和 Gemini 路由生成各自端点，不能假设一个 base URL 对四种客户端具有相同追加规则；未被管理员覆盖的字段仍可由顶层 `base_url`、`api_key` 和模型补全，不增加客户端协议。
- `settings_config` 在 API 和模板展示模型中始终是 JSON 对象；Codex `config` 是对象内部的合法 JSON 字符串值。YAML 通过结构化编码器生成，禁止手工拼接转义。
- 管理员覆盖 `base_url` 或 `settings_config` 中端点时双向同步 `endpoints[0].url`，并校验 HTTP(S)、TOML 和所需 token 占位符。
- 输入框第一次聚焦且尚无覆盖值时，将当前动态默认值写入可编辑状态；保存空值表示移除覆盖并恢复动态默认。

## 6. 生命周期与权限

### 6.1 用户状态机

- 创建：在一个事务中创建订阅、分组绑定和每组一个隐藏计费 Token。
- 编辑：按稳定分组 key 做 diff；保留未删除分组的绑定 ID 和 Token，新增分组新建，移除分组撤销 Token 与访问凭证。
- 禁用：订阅和所有底层 Token/访问凭证禁用，公共下载返回 `410`。
- 启用：恢复仍有效的分组 Token；渠道绑定继续由下一次下载动态调和。
- 删除：撤销 Token/访问凭证并软删除订阅，公共下载返回 `404`；不提供恢复。

### 6.2 权限

- 普通用户只能管理自己的订阅、查看结构化 provider 摘要、复制稳定 URL 和阅读教程。
- 登录态普通用户 API 不提供 raw YAML、底层计费 Token 或 provider 访问 key。稳定 URL 本身是 bearer secret，持有者可无登录下载 YAML，这是协议必需能力。
- 管理员可预览、复制和下载完整 YAML，管理模板、教程、渠道应用映射和 Token 可见性设置；敏感操作写审计日志并返回 `Cache-Control: no-store`。
- 普通 Token 列表查询默认过滤 Hamster 底层计费 Token。切换显示只改变列表可见性，不改变其生命周期所有权。

## 7. API 与前端

### 7.1 API 分区

- 用户：`/api/hamster-switch/subscriptions` 下的列表、详情、创建、更新、禁用、启用、删除和结构化摘要。
- 管理员：模板草稿/预览/发布/回滚、完整 YAML 预览/下载、教程更新/冲突、渠道应用映射和站点设置。
- 公开：高熵下载 token 驱动的稳定 YAML URL，不依赖登录会话。

所有 DTO 明确区分普通用户和管理员，不能先构造含密钥对象再依赖前端隐藏。

### 7.2 前端页面

- 新增独立“配置订阅”路由和侧边栏项，不占用 New API 原有付费“订阅管理”模块。
- 用户页支持搜索、创建、编辑、结构化 provider 摘要、复制 URL、教程、禁用/启用和不可恢复删除。
- 管理员区域支持模板和展示覆盖、严格 JSON 编辑、动态预览、版本历史/回滚、教程管理和 Token 隐藏开关。
- 渠道新增/编辑表单加入四应用多选控件，默认四项全选、至少一项、支持恢复默认。
- 所有新增文案进入当前 i18n 资源；中英文切换后侧边栏“配置订阅”和页面内容同步变化。控件只使用主题 token，并覆盖亮色/暗色和桌面/移动布局。

## 8. 模板与教程安全

- 模板 DSL 只暴露批准变量，必须包含 `format_version` 和 provider 循环；发布前渲染并重新解析 YAML。
- 硬错误阻止发布，警告要求管理员显式确认；版本发布和回滚均创建新的不可变活动版本。
- 当前启用模板每次下载读取，因此发布/回滚立即影响所有现有订阅的下一次刷新。
- 教程远程候选只接受独立签名信任根的正式 Release。图片下载执行协议、地址、重定向、大小、媒体类型和 SSRF 校验后再缓存。
- 站点覆盖不会被远程更新静默覆盖，冲突由管理员采纳或拒绝。

## 9. 发布、安装与回滚

独立通道从 `1.0.0` 开始：

- 源码补丁：`new-api-hamster-v1.0.0`。
- 预构建：`image-new-api-hamster-v1.0.0`。
- 镜像：`hamster-switch/new-api:hamster-v1.0.0`。

源码补丁 manifest 精确固定上游 commit、关键文件哈希和锚点。预构建 Release 包含 Linux amd64 镜像归档、嵌入前端的 Linux amd64 二进制、版本绑定安装器和 `SHA256SUMS`，资产不可覆盖。

安装器复用统一检测机制：优先现有 systemd，再检测 Docker Compose 标签；歧义时要求 `--compose-file` 和 `--service`。Compose 只替换一个服务镜像并使用 `--no-build`，systemd 只原子替换 `ExecStart` 二进制，不修改 unit/env。两种方式均备份、检查 `/api/status`，失败自动恢复。

README 提供无服务器编译的一条命令、支持矩阵、显式 Compose 参数示例、systemd 前置条件、升级/回滚、健康检查和故障诊断。

## 10. 兼容性与迁移

- 只支持新安装到固定当前上游，旧 `new-api-v1.0.0` 和旧表不迁移。
- GORM migration 只新增本通道表/字段，不修改已应用上游 migration 历史；SQLite、MySQL、PostgreSQL 均需验证。
- 上游关键文件哈希不匹配时安装硬失败，无 `--force`。
- 用户自定义模板、教程覆盖和渠道应用映射在同通道后续升级中保留；内置默认升级仅精确替换仍等于旧内置值的数据。

## 11. 关键取舍

- 采用“每组一个计费 Token + 每 provider 一个访问凭证”，同时满足共享配额与固定渠道；拒绝每渠道复制配额和可篡改渠道后缀。
- 动态生成而非创建时快照，使新增渠道可通过刷新出现；代价是下载路径需要并发安全的凭证调和和更严格的运行时验证。
- 单个 `general` provider 显式列出 apps，避免按应用复制服务端 provider 并丢失渠道/倍率颗粒度。
- 产品代码和预构建交付在同一个 `1.0.0` 验收，防止发布要求用户在服务器编译的半成品版本。

## 12. 风险与缓解

- 渠道绑定绕过：独立凭证类型、不可预测 key、每请求分组/渠道/模型复验和负向权限测试。
- 下载并发产生重复凭证：数据库唯一约束、事务、冲突后重读和并发测试。
- 动态渠道变化删除已导入 provider：稳定 ID、Hamster Switch 现有刷新同步语义、失效 key 立即拒绝。
- 多数据库差异：避免数据库专属 JSON 操作，应用层规范化 apps/presentation，并运行三数据库 migration/约束测试。
- 前端上游架构漂移：只针对固定 `web/src` 基线生成补丁，manifest 对关键路由/i18n/布局文件做哈希和锚点检查。
- 自动部署误识别：单目标选择、歧义拒绝、显式参数、健康失败自动回滚和安装器夹具测试。
