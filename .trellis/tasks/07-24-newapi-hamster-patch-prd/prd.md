# New API Hamster 更新补丁 PRD

## Goal

基于当前 New API 上游和 sub2api 已验证的配置订阅体验，交付一套可安装、可升级、可回滚的 New API Hamster 补丁，使用户通过稳定订阅 URL 在 Hamster Switch 中获取按“订阅分组 × 底层渠道”拆分、倍率准确且真实固定渠道的 provider。

正式版本必须同时包含完整产品功能与 Linux amd64 预构建安装能力，不允许要求最终用户在服务器编译前后端。

本任务当前只完成规划，不修改 New API 产品代码、不运行 `task.py start`、不发布 Release。

## User Value And Roles

- 普通用户：创建和管理自己的配置订阅，选择 New API 分组，查看结构化 provider 摘要，复制稳定 URL，在 Hamster Switch 刷新渠道，并查看使用教程。
- 管理员：管理订阅模板和教程，查看完整 YAML，编辑 provider 展示/倍率/图标/端点配置，为渠道指定默认导入应用，控制专用 Token 是否显示，并诊断渠道跳过原因。
- 运维人员：使用一条命令将预构建版本安装到现有 Docker Compose 或 systemd New API，完成校验、备份、健康检查和失败回滚。

## Confirmed Baseline

### New API

- 唯一实现基线为 `QuantumNous/new-api@1721144221ec5c94dd87891a7ae1bee228e7bb63`。
- 已发布旧版 `new-api-v1.0.0` 固定在 `5a6c53d4966b2e34690ab49f3dd19be01c88fdbe`，补丁覆盖 24 个文件（`releases/new-api/1.0.0/manifest.template.json`）。从旧基线到当前上游约 1536 个文件发生变化，不能原地复用。
- 当前前端已合并为单一 `web/src`，旧 `web/default` 和 `web/classic` 路径不存在；当前栈为 React 19、TanStack Router/Query、i18next、Rsbuild 和 Bun。
- 后端继续使用 Go、Gin、GORM；迁移由 `model/main.go` 的 `AutoMigrate` 集中执行，必须兼容 SQLite、MySQL 和 PostgreSQL。
- New API 已有付费套餐“订阅”领域，Hamster 功能必须使用独立表、路由、侧边栏 key 和文案，不能占用 `/subscription` 命名空间。
- 普通 Token 按分组和模型选择渠道。历史实现中指定渠道只允许管理员 Token（`middleware/auth.go:458`），分发入口从 Token specific-channel context 读取渠道（`middleware/distributor.go:35`）。本补丁不能把该能力开放给普通 Token。

### 历史 New API Hamster 实现

- 本地 `Proxy/new-api-main` 和 `Proxy/new-api-patched-5a6` 均基于旧 `5a6c53d`，仅作为历史实现参考。
- 早期实现用 `GET /api/hamster-switch/subscription.yaml` 和 `hs_` 签名载荷生成临时订阅，并通过 `service.GetUserGroupRatio` 返回用户到目标分组的有效倍率（`controller/hamster_switch_subscription.go:57-77`）。
- 旧 1.0.0 已实现持久化订阅、分组专用 Token、稳定 URL、模板版本、教程和两套旧前端（`model/hamster_config_subscription.go`、`service/hamsterswitch/`），但没有当前所需的结构化展示字段、严格 JSON、动态默认编辑、渠道应用映射和真实渠道绑定。

### sub2api 产品基线

- 参考源码为 `D:/sub2api_private@31d667fb72bde5e13039b055d66802c5256aec4b`。
- 管理员可编辑订阅 `name`、`description`、`icon`、`home_url`，以及 provider 的 `name`、`description`、`icon`、`base_url`、`pricing`、`settings_config`；空覆盖输入框聚焦后填入动态默认值（`frontend/src/views/user/SubscriptionManagementView.vue:419`、`:1276`）。
- 默认倍率读取实际分组倍率，倍率 `1` 也显示；管理员覆盖与默认 `pricing` 按字段合并（`backend/internal/hamsteryaml/presentation_test.go:50`）。
- `settings_config` 在 API 边界是 JSON 对象，Codex TOML 是对象内部的字符串；端点需要双向同步并校验（`backend/internal/hamsteryaml/settings_config.go`、`settings_config_test.go:44`）。Hamster Switch 的 general 展开还支持按 `claude`、`claude-desktop`、`codex`、`gemini` 子对象读取应用专用配置（`src-tauri/src/subscription/general.rs:180`）。
- 公共下载每次读取当前启用模板并动态生成 YAML（`backend/internal/handler/hamster_switch_subscription.go:548`），因此模板和数据变化影响现有订阅的下一次刷新。

### Hamster Switch 消费契约

- `format_version: 1` 支持稳定 provider ID、`app_type`、`apps`、`group_name`、`pricing.multiplier`、`settings_config`、`endpoints` 和图标（`src-tauri/src/subscription/models.rs:116`）。
- `app_type: general` 可按显式 `apps` 展开 Claude Code、Claude Desktop、Codex、Gemini；省略 apps 才隐式全选（`src-tauri/src/subscription/general.rs:14`）。服务端必须显式输出 apps。
- 刷新按稳定 ID 替换远程 provider 集合并同步已导入状态（`src-tauri/src/database/dao/subscriptions.rs:229`、`src-tauri/src/commands/subscription.rs:121`）。
- 客户端当前主动拒绝空 provider 列表（`src-tauri/src/subscription/parser.rs:22`、`fetcher.rs:126`）。本轮不修改客户端；New API 允许保留空订阅，但 Hamster Switch 初次导入或空列表刷新沿用现有失败行为。

## Requirements

### R1. 独立领域与兼容边界

- 只针对固定的当前 New API 上游开发，不迁移旧 `new-api-v1.0.0` 表、数据、`hs_` URL 或旧双前端入口。
- Hamster 配置订阅使用独立 `hamster_` 模型、`/api/hamster-switch` 路由、权限和前端模块；New API 原有付费订阅和普通 Token 行为保持不变。
- GORM 只能新增本功能结构，不能重写上游已应用 migration 历史。

### R2. 配置订阅生命周期

- 用户可列表、搜索、创建、查看、编辑、禁用、启用和不可恢复删除自己的订阅。
- 每个所选分组创建一个隐藏的底层计费 Token；编辑分组使用 diff，未移除分组保留稳定身份和 Token。
- 稳定下载 URL 在普通编辑、禁用和重新启用后不变；禁用返回 `410`，删除返回 `404`。
- 订阅、分组绑定、计费 Token 和 provider 访问凭证的创建、更新、禁用、恢复、删除必须保持事务一致。
- 专用 Token 默认从普通 API Key 列表隐藏；管理员可切换显示。显示设置不改变认证、计费或生命周期所有权。

### R3. 最细粒度 Provider 与真实渠道绑定

- 每个 provider 对应一个“订阅所选分组 × 当前启用底层渠道”，不能按应用类型、相同模型或相同倍率合并。
- 渠道只有在启用、属于目标分组且至少配置一个该分组可请求模型时才能生成 provider。
- provider ID 必须包含稳定分组绑定身份与 New API 渠道 ID，不得依赖分组名、渠道名、图标、倍率或应用列表。
- 每个 provider 使用不可伪造的内部访问凭证绑定对应渠道；请求时重新验证订阅/Token/凭证状态、渠道、分组和模型。
- 绑定渠道不可用时请求明确失败，不允许静默改投同组其他渠道；普通 API Key 不获得指定渠道权限。
- 每个分组仍只使用一个底层计费 Token，所有该分组渠道共享其真实 quota、限速、到期和扣费，不能按渠道复制额度。

### R4. 动态刷新、应用和倍率

- 公共 YAML 每次下载都读取当前启用模板、渠道、abilities、应用映射和倍率，不保存创建时 provider 快照。
- 新增渠道在现有订阅下一次刷新出现；禁用、删除或失去全部模型的渠道消失；未变化 provider 的 ID 保持不变。
- 默认输出 `app_type: general`，并显式输出 `apps: [claude, claude-desktop, codex, gemini]`。
- 管理员可在渠道配置中多选默认导入应用，至少选择一项；恢复默认回到四项全选。相同渠道在不同分组下继承同一渠道级应用映射。
- `pricing.multiplier` 使用订阅用户访问目标分组的有效倍率，特殊倍率优先，否则为目标分组基础倍率；包括 `1` 在内始终显式输出。
- 允许创建和保存暂时没有可用 provider 的订阅。New API 用户页显示空状态，管理员预览显示跳过原因，公共 YAML 返回合法 `providers: []`。

### R5. 模板与结构化展示配置

- 管理员可管理模板草稿、动态预览、硬错误/警告、发布、不可变历史和回滚；发布或回滚影响全部现有订阅下一次刷新。
- 模板提供订阅级和“分组 × 渠道”provider 级展示覆盖，至少覆盖名称、描述、图标、base URL、pricing 和 settings_config。
- 新增渠道和分组无需手工刷新模板状态即可显示动态默认；删除后不再生成 provider。
- 倍率 `1` 在模板界面可见并可编辑。输入框首次聚焦且无覆盖时填入当前动态默认；清空并保存表示恢复动态默认。
- `settings_config` 必须按严格 JSON 对象编辑、校验和返回，不能出现多余反斜杠或双重编码。
- `base_url`、四应用 `settings_config` 内端点和 `endpoints` 保持同一站点语义，同时保留各协议所需路径，并校验 HTTP(S)、Codex TOML 和 token 占位符。
- 模板上下文不得包含渠道上游密钥；真实下载 token 和 provider 访问 key 只在最终授权下载渲染阶段注入。

### R6. 权限与敏感数据

- 普通用户管理 API 只返回结构化摘要和稳定 URL，不返回、预览、复制或下载 raw YAML，不返回底层计费 Token 或 provider 访问 key。
- 稳定 URL 是 bearer secret；持有者可无登录下载完整 YAML，这是 Hamster Switch 消费所必需的公开接口。
- 完整 YAML 的登录态预览、复制和下载仅管理员可用，并要求审计日志与 `Cache-Control: no-store`。
- DTO 在服务端按角色构造，不能依赖前端隐藏敏感字段；日志和错误不得包含 URL token、provider key 或 YAML 正文。

### R7. 教程、国际化与主题

- 普通用户可读取已发布教程；管理员可管理站点覆盖、签名远程候选、图片、冲突采纳/拒绝和版本状态。
- 远程教程只接受独立信任根签名的正式 Release；图片缓存执行 SSRF、重定向、地址、大小、类型和尺寸校验。
- 所有新增可见文案进入当前 New API i18n。切换英文后侧边栏“配置订阅”、页面标题和操作文案全部切换。
- 页面使用现有组件和主题 token，支持亮色/暗色、桌面/移动、加载、空、错误和禁用状态。

### R8. 发布与无编译安装

- 建立独立 `new-api-hamster` 通道，从 `1.0.0` 开始；不覆盖旧 `new-api-v1.0.0`。
- 源码补丁标签为 `new-api-hamster-v1.0.0`，预构建标签为 `image-new-api-hamster-v1.0.0`，镜像为 `hamster-switch/new-api:hamster-v1.0.0`。
- 首版只支持 Linux amd64，同时支持现有 Docker Compose 和 systemd；Linux arm64 延后。
- 预构建 Release 必须包含镜像归档、嵌入前端的 Linux amd64 二进制、版本绑定安装器和 `SHA256SUMS`。
- Compose 安装只替换一个现有服务镜像并使用 `--no-build`；systemd 只原子替换 `ExecStart` 二进制，不改 unit/env。
- 安装器必须校验、备份、检查 `/api/status` 并在失败时自动恢复；自动检测歧义时支持显式 `--compose-file` 和 `--service`。
- README 提供可直接公开的一条命令安装、支持矩阵、显式参数、升级、回滚、健康检查和故障排查，不要求服务器执行 Bun/Go 构建。

## Key Decisions

| ID | Decision | Requirement |
| --- | --- | --- |
| D1 | 仅支持固定当前上游，不兼容旧 1.0.0 | R1 |
| D2 | 首个正式版本完整覆盖 sub2api 产品能力，不发布缺模板/教程/预构建的半成品 | R2-R8 |
| D3 | provider 使用“分组 × 渠道”最细颗粒度 | R3 |
| D4 | 渠道默认作为 general provider 支持全部应用，管理员可覆盖 | R4 |
| D5 | 应用为四项多选，默认显式全选且禁止空选 | R4 |
| D6 | 普通登录态用户不能读取 raw YAML，管理员可以 | R6 |
| D7 | 配置订阅专用 Token 默认隐藏，管理员可切换显示 | R2 |
| D8 | 首版支持 Linux amd64 的 Compose 与 systemd，arm64 延后 | R8 |
| D9 | 使用独立 `new-api-hamster` 通道并从 1.0.0 开始 | R8 |
| D10 | New API 允许空订阅；Hamster Switch 空列表支持不在本轮 | R4 |
| D11 | provider 必须真实固定渠道，不允许仅展示拆分 | R3 |
| D12 | YAML 和 provider 集合动态生成，现有订阅刷新获取当前配置 | R4-R5 |

## Acceptance Criteria

- [ ] AC1：在固定上游全新数据库启动后，SQLite、MySQL、PostgreSQL 均成功创建独立 Hamster 结构，原付费订阅和普通 Token 回归测试通过。（R1）
- [ ] AC2：创建含两个分组的订阅只产生两个底层计费 Token；同组多个渠道不会增加计费 Token，事务任一步失败不残留记录。（R2、R3）
- [ ] AC3：编辑名称、排序或展示字段保持 public ID、下载 URL、未移除分组绑定 ID、Token 和 provider ID 不变；分组新增/移除只影响对应资源。（R2、R3）
- [ ] AC4：禁用后下载返回 `410` 且所有凭证拒绝请求；启用后同一 URL 恢复；删除后下载返回 `404` 且凭证不可再用。（R2）
- [ ] AC5：同一分组含三个合格渠道时生成三个 provider；同一渠道属于两个所选分组时生成两个 provider，模型、倍率和 ID 独立。（R3、R4）
- [ ] AC6：每个 provider 的请求只命中其绑定渠道。篡改 key、跨用户、跨分组、禁用渠道、无能力模型均失败且不会回退；普通 Token 不能指定渠道。（R3）
- [ ] AC7：管理员新增渠道后，同一稳定 URL 下一次生成包含新 ID；渠道禁用/删除后移除；其他 provider ID 不变。（R4）
- [ ] AC8：默认 provider 显式输出 general、四项 apps 和实际有效倍率；倍率为 `1` 时 YAML、管理员界面和结构化摘要均显示 `1`。（R4）
- [ ] AC9：管理员将渠道应用改为任意非空子集后，全部相关 provider 下一次生成同步变化；空选择被拒绝，恢复默认回到四项全选。（R4）
- [ ] AC10：无合格渠道时 New API 可创建/编辑订阅，用户页显示空状态，管理员看到跳过原因，公共下载可被 YAML 解析器解析为 `providers: []`。（R4）
- [ ] AC11：模板输入框聚焦后出现动态默认内容；icon、pricing、base URL 和 settings_config 可编辑。严格 JSON 可 round-trip，YAML 重新解析后为对象且无双重转义。（R5）
- [ ] AC12：任一方向修改端点都会同步 base URL、settings_config 和 endpoints；非法 URL、TOML 或缺失 token 占位符不能发布。（R5）
- [ ] AC13：模板发布/回滚后现有订阅下一次下载使用新活动版本；自定义历史、教程覆盖和渠道映射不被默认升级覆盖。（R5、R7）
- [ ] AC14：普通用户所有登录态 DTO 均不含 raw YAML/Token/provider key；管理员完整 YAML 操作有权限、审计和 no-store；敏感值不进入日志。（R6）
- [ ] AC15：中英文切换、亮暗主题、桌面和移动端测试通过；英文状态下不残留中文“配置订阅/订阅管理”入口文案。（R7）
- [ ] AC16：签名教程更新、覆盖冲突和图片 SSRF/类型/大小校验测试通过，未签名或预发布内容不能激活。（R7）
- [ ] AC17：补丁两次构建哈希一致，在干净固定上游完成 apply/check/rollback 且回滚恢复全部原始哈希。（R8）
- [ ] AC18：公开 README 的一条命令可在 Linux amd64 Compose 和 systemd 环境安装，无服务器编译；健康失败自动恢复原镜像/二进制。（R8）
- [ ] AC19：安装器自动检测标准和自定义服务，歧义时明确失败；提供绝对 Compose 文件和服务名后可安装。非 x86_64 明确拒绝。（R8）
- [ ] AC20：从公开 `new-api-hamster-v1.0.0` 和 `image-new-api-hamster-v1.0.0` 重新下载后，签名、SHA-256、资产完整性、安装、健康和回滚复验通过。（R8）

## Out Of Scope

- 迁移、兼容或原地升级旧 `new-api-v1.0.0`、`5a6c53d` Hamster 表、旧 `hs_` URL 和旧双前端实现。
- 修改 Hamster Switch 以接受空 provider 列表或显示专门空订阅提示。
- 在 Hamster Switch 中新增 Claude Code 与 Claude Desktop 的额外默认双映射规则。
- Linux arm64、Windows、macOS 服务器预构建产物。
- 长期兼容任意 New API fork、漂移 commit 或被修改的上游 migration；哈希不匹配不提供 `--force`。
- 本规划阶段的产品代码修改、任务启动、提交、标签或发布。

## Risks And Deferred Items

- 渠道绑定认证属于高风险核心路径，必须通过独立凭证类型和普通 Token 负向回归测试隔离。
- 动态下载会调和 provider 凭证，需要唯一约束、事务和并发测试；不能因竞态生成多个 key。
- Hamster Switch 遇到 `providers: []` 会拒绝导入/刷新，这是已接受的客户端边界；未来可在 Hamster Switch 独立任务改善。
- GitHub Actions 分钟不足会影响云端预构建，但不降低正式发布验收标准；可等待额度恢复或使用等价受控 amd64 构建环境，不要求最终用户编译。
- 上游继续演进时必须为新 commit 发布新补丁版本并重新完成兼容和安装验证，不能覆盖 1.0.0 资产。

## Planning Status

所有用户所有的产品、范围、UX、兼容和风险决策已确认，无阻塞待定项。技术设计见 `design.md`，执行与验证顺序见 `implement.md`。进入实现前仍需用户审核本次最终规划摘要，并在后续消息中明确批准启动实现。
