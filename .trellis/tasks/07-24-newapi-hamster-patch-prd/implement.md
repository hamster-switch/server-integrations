# New API Hamster 补丁实施计划

> 当前文件仅定义 Phase 2 的执行顺序。最终规划获批前不得运行 `task.py start`、修改 New API 产品代码或发布资产。

## 当前执行进度（2026-07-24）

- 固定上游产品实现、前后端测试、三数据库迁移验证、确定性补丁候选、Linux amd64 候选镜像、Compose/systemd 安装器和文档均已完成。
- 最终补丁包含 57 个产品文件；实际改动与 manifest 为 `57/57`，无 `create` 标记错配。连续两次构建 SHA-256 均为 `2d1ae3ee0e84766df062452541df1e0d978836b05bd067c49d827c6ff05fdb94`，候选 apply/rollback 通过。
- `go vet ./...` 只复现固定上游既有报告；`go test ./... -count=1` 只复现 `service` 共享状态测试的全量运行波动，目标测试单独运行通过。聚焦 Go、前端、安装器、发布仓库和真实 MySQL/PostgreSQL 验证均通过。
- 候选镜像 `hamster-switch/new-api:hamster-v1.0.0-candidate` 已验证为 `linux/amd64`，`/new-api` 为 x86-64，`/api/status` 返回 HTTP 200 JSON。
- 本会话内置浏览器不可用，真实桌面/移动端、亮/暗主题视觉检查仍未完成；临时检查容器已停止并删除。
- 正式标签、签名 GitHub Releases 和公开资产重下载验证仍受发布授权与签名凭证边界阻塞，不得以本地候选替代。
- `new-api-hamster-v1.0.0` 已发布但因 Windows CRLF `Dockerfile` 上游指纹无法应用于 Linux checkout，保持资产不可变并由 `1.0.1` 修正；不得覆盖或删除已发布资产。
- `1.0.1` 的校验错误使用了会应用 clean filter 的默认 `git hash-object`，仍未发现工作树 CRLF；最终修正版为使用 `--no-filters` 校验的 `1.0.2`。

## 0. 基线与工作区

- [x] 从 `QuantumNous/new-api@1721144221ec5c94dd87891a7ae1bee228e7bb63` 创建干净、可追踪的实现工作树：`D:/mini_project/Hamster switch/Proxy/new-api-hamster-1721144`，分支 `feat/new-api-hamster-v1.0.0`。
- [x] 记录上游 commit、Go/Bun 版本、当前 `web/src` 路由/i18n/构建命令和 Docker/systemd 健康端点。
  - 上游：`1721144221ec5c94dd87891a7ae1bee228e7bb63`；本机 Go：`go1.26.4 windows/amd64`；模块要求 Go `1.25.1`。
  - 前端为单一 `web/src`，脚本为 `bun run typecheck`、`bun run lint`、`bun run build`；本机启动时尚未安装 Bun。
  - 生产健康端点为 `/api/status`；Compose/systemd 交付仍由 `server-integrations` 现有安装器契约负责。
- [x] 加载 New API 目标层的 Trellis/spec/AGENTS 约束；确认旧 `new-api-main`、`new-api-patched-5a6` 只作历史参考，不作为修改基线。
- [x] 运行未修改上游的后端测试、前端检查和生产构建，记录已存在的基线失败。
  - `go test ./... -count=1`：除根包和 `service` 外通过。根包因未生成 `web/dist` 导致 `go:embed` setup failure；`service/TestObserveChannelAffinityUsageCacheByRelayFormat_MixedMode` 在全量运行中期望 `2`、实际 `3`。
  - 前端基线检查与生产构建受本机缺少 Bun 阻塞；安装项目本地 Bun 后补跑，不能把该环境缺口视为产品回归。

依赖：无。完成后才能执行其余阶段。

## 1. 数据模型与迁移

- [x] 新增配置订阅、订阅分组绑定、provider 访问凭证、渠道应用覆盖、模板草稿/版本、教程快照/覆盖/图片元数据实体。
- [x] 为 public ID、下载 token、provider 访问 key、订阅×分组、分组绑定×渠道和渠道应用覆盖建立唯一约束。
- [x] 将默认 Token 可见性设置为隐藏，并为普通 Token 列表增加可索引的内部 Token 过滤条件。
- [x] 接入当前上游 `AutoMigrate`，不得重写已应用上游 migration。
- [x] 为 SQLite、MySQL、PostgreSQL 验证建表、唯一冲突、软删除和事务回滚。

依赖：阶段 0。该阶段只建立持久化契约，不接入公开路由。

## 2. 订阅与计费 Token 生命周期

- [x] 实现创建输入校验：名称、至少一个目标分组、去重和稳定排序。
- [x] 在单事务中创建订阅、每组稳定绑定和每组一个隐藏计费 Token。
- [x] 实现列表/详情所有权查询、搜索和分页，普通 DTO 不含任何密钥。
- [x] 实现 group diff 编辑：保留未移除绑定的 ID/Token，新增时创建，移除时撤销 Token 和全部渠道凭证。
- [x] 实现禁用 `410`、启用恢复、不可恢复删除 `404` 和失败事务回滚。
- [x] 验证 quota、限速、到期、用量扣减仍落到底层分组 Token，不能按渠道复制额度。

依赖：阶段 1。

## 3. 渠道绑定认证与分发

- [x] 定义独立 provider 访问 key 格式和只存储/返回必要明文的策略，确保无法伪造渠道 ID。
- [x] 实现按“分组绑定 × 渠道”并发安全 `get-or-create`，唯一冲突后重读同一凭证。
- [x] 在认证中解析 provider 凭证，加载订阅、分组绑定和底层计费 Token，写入专用上下文标记。
- [x] 在渠道分发前验证订阅/Token/凭证状态、渠道启用、目标分组 membership 和请求模型 ability。
- [x] 绑定渠道失败时禁止进入普通随机选择、渠道亲和或跨渠道 retry；普通 Token 和管理员现有指定渠道行为保持原样。
- [x] 增加伪造 key、篡改渠道、跨用户、跨分组、禁用渠道、无模型和普通 Token 冒充的负向测试。

依赖：阶段 1、2。完成后才能生成携带真实 key 的 YAML。

## 4. 动态 Provider 生成

- [x] 从订阅所选分组和当前 enabled abilities 查询分组×渠道集合，不按应用聚合。
- [x] 只输出启用、属于目标分组且至少有一个可请求模型的渠道；模型去重并稳定排序。
- [x] provider ID 由 subscription public ID、稳定分组绑定 ID 和 channel ID 构成，不依赖显示字段。
- [x] 输出 `app_type: general` 和显式四应用默认 `apps`；应用覆盖只接受四个枚举且至少一个。
- [x] 使用订阅用户到目标分组的有效倍率，特殊倍率优先，`pricing.multiplier: 1` 也显式输出。
- [x] 生成 base URL、opaque api_key、models、default model、endpoints 和 meta；settings_config 按 Claude Code、Claude Desktop、Codex、Gemini 子对象输出各协议正确端点。
- [x] 普通摘要使用脱敏数据；管理员预览使用占位 key；公共下载才调和真实 provider 凭证。
- [x] 支持合法 `providers: []`，同时生成普通用户空状态和管理员结构化跳过诊断。
- [x] 验证新增/禁用/删除渠道、模型和应用映射变化在同一稳定 URL 下一次生成中出现，已有 ID 保持不变。

依赖：阶段 2、3。

## 5. 模板、展示配置与严格 JSON

- [x] 移植并适配 sub2api 的闭合模板上下文、草稿、诊断、发布、不可变历史和回滚。
- [x] presentation 支持订阅级覆盖和按“分组 key × 渠道 ID”的 provider 覆盖；新增渠道无覆盖时自动显示动态默认。
- [x] 默认展示 description、icon、base_url、pricing 和 settings_config；倍率 `1` 可见。
- [x] API 中 `settings_config` 始终为对象；Codex TOML 仅作为对象内字符串值，YAML 使用结构化编码。
- [x] 实现 base_url、四应用 settings_config 内地址和 endpoints 的同步，以及 HTTP(S)、TOML、token 占位符校验；不能把不同协议需要的路径错误合并。
- [x] 只对仍等于旧内置默认值的数据执行后续精确升级，自定义模板/覆盖保持不变。
- [x] 测试模板不接触渠道密钥、下载 token、真实 provider key，硬错误不能绕过，警告必须确认。

依赖：阶段 4。

## 6. API、权限与审计

- [x] 在独立 `/api/hamster-switch` 命名空间注册用户、管理员和公开下载路由。
- [x] 用户 API 提供生命周期、结构化摘要、稳定 URL 和教程，不提供 raw YAML 或密钥。
- [x] 管理员 API 提供完整 YAML 预览/复制/下载、模板、教程、渠道应用映射和 Token 显示设置。
- [x] 下载 token 采用恒定时间或等价安全查找策略，敏感响应设置 `no-store`，日志不记录 URL token、provider key 或 YAML 正文。
- [x] 管理员敏感读取、模板发布/回滚、教程更新和设置变更写审计事件。
- [x] 验证普通用户跨所有权访问、普通用户命中管理员 API、禁用和删除状态码。

依赖：阶段 2、4、5。

## 7. 教程内容

- [x] 移植签名教程 manifest、候选版本、站点覆盖、冲突采纳/拒绝和图片缓存流程。
- [x] 使用独立签名信任根，只接受非草稿、非预发布且校验通过的内容 Release。
- [x] 图片抓取覆盖 SSRF、重定向、协议、DNS/IP、大小、类型和尺寸限制。
- [x] 普通用户只读已发布内容；管理员可管理覆盖和远程候选。

依赖：阶段 1、6。可与阶段 5 并行编码，但必须在同一正式版本验收。

## 8. 单一前端实现

- [x] 在当前 `web/src` 新增“配置订阅”路由、侧边栏模块和权限保护，不修改付费订阅入口。
- [x] 实现用户列表、搜索、创建/编辑、provider 摘要、空状态、复制 URL、教程、禁用/启用和危险删除确认。
- [x] 实现管理员模板编辑、结构化展示字段、严格 JSON 编辑、动态预览、历史/回滚、教程管理和 Token 隐藏设置。
- [x] 在渠道新增/编辑表单加入四应用多选，默认全选、禁止空选、支持恢复默认。
- [x] 输入控件聚焦时若尚无覆盖，填入当前动态默认；清空并保存表示恢复动态默认。
- [x] 所有可见文案接入当前 i18n；测试中英文切换后侧边栏“配置订阅”和页面标题同步变化。
- [ ] 使用当前主题 token 和现有组件，验证亮/暗主题、桌面/移动、长文本和加载/空/错误/禁用状态。

依赖：阶段 4、5、6 的 DTO 和权限契约稳定后。

## 9. 自动化验证

后端最低验证：

```powershell
go test ./model ./middleware ./service/hamsterswitch ./controller ./router -count=1
go test ./... -count=1
go vet ./...
go build -trimpath -o new-api .
```

前端命令以固定上游脚本为准，预期为：

```powershell
Set-Location web
bun install --frozen-lockfile
bun run lint
bun run type-check
bun run test
bun run build
```

必须新增并通过：

- [x] 生命周期、事务失败回滚、稳定 URL 和所有权测试。
- [x] provider ID 稳定、渠道动态增删、倍率特殊值和 `1`、apps 覆盖测试。
- [x] provider 凭证并发、篡改、越权、禁用和固定渠道无回退测试。
- [x] `settings_config` JSON round-trip、YAML reparse、Codex TOML 和端点同步测试。
- [x] 用户 DTO 不含 YAML/密钥、管理员权限/审计/no-store 测试。
- [x] 中英文、主题、输入框聚焦填充和新增动态配置项的前端交互测试。
- [x] 空 providers 服务端行为测试，并注明 Hamster Switch 当前拒绝行为属于已知边界。
- [x] SQLite/MySQL/PostgreSQL migration/constraint 集成测试。

依赖：阶段 1 至 8。

## 10. 补丁与确定性资产

- [x] 对最终修改文件记录固定上游 SHA-256、结果 SHA-256、必要锚点和新增文件声明。
- [x] 生成 `releases/new-api/hamster/1.0.0/manifest.template.json` 和确定性 `new-api-hamster-patch-1.0.0.tar.gz`。
- [x] 连续构建两次并比较补丁哈希；在干净固定上游执行 apply/check/rollback，确认原始哈希完全恢复。
- [x] 文档明确旧 `new-api-v1.0.0` 不兼容且不迁移。

依赖：阶段 9 全部通过。

## 11. Linux amd64 预构建与安装器

- [x] 扩展预构建工作流，为 New API 构建嵌入前端的 Linux amd64 二进制和 amd64 镜像。
- [ ] 生成 `image-new-api-hamster-v1.0.2` Release、版本绑定安装器和 `SHA256SUMS`。
- [x] Compose 自动检测支持标准/自定义镜像和重命名服务；歧义拒绝并支持显式 `--compose-file`、`--service`。
- [x] systemd 从实际 `ExecStart` 解析二进制，保留 owner/mode，原子替换且不改 unit/env。
- [x] 两种模式都备份、等待 `/api/status`、失败恢复；部署命令禁止服务器端 build。
- [x] 使用夹具覆盖自动检测、显式选择、路径含空格、非 x86_64 拒绝、校验失败、健康失败和回滚。

依赖：阶段 10 的补丁版本和结果固定。

## 12. README 与正式发布

- [x] 新增公开 README：用途、支持版本、Linux amd64/Compose/systemd 矩阵、风险和不兼容说明。
- [x] 提供默认一条命令安装，以及无法自动识别时带绝对 Compose 文件和服务名的完整命令。
- [x] 说明升级、回滚、健康检查、备份位置、常见错误和如何确认实际运行镜像/二进制版本。
- [ ] 在发布前从 README 原样执行安装命令完成全新 Compose、现有 Compose 和 systemd 演练。
- [ ] 创建修正版 `new-api-hamster-v1.0.2`，等待签名补丁资产完成；再创建 `image-new-api-hamster-v1.0.2`。
- [ ] 从公开 Release 重新下载全部资产，复验 SHA-256、签名、安装、健康检查和回滚。
- [x] 不覆盖任何已发布资产；修正必须发布新 patch 版本。

依赖：阶段 9、10、11。全部通过才满足正式 `1.0.0` 验收。

## 13. 高风险文件与回滚点

- 认证/分发中间件：必须以独立上下文标记隔离，保留普通 Token 回归测试。
- Token 查询：隐藏过滤必须只影响列表展示，不能影响认证、扣费和管理员显式诊断。
- GORM `AutoMigrate`：先在临时三数据库验证，禁止修改上游历史 migration。
- 路由与侧边栏：固定上游哈希和中英文测试防止主题/语言入口回归。
- 统一安装器和 release workflow：必须先通过现有 sub2api 安装器回归，避免破坏其他 component/channel。

任何阶段出现不可逆 schema、权限绕过、配额放大、provider 跨渠道或安装回滚失败，都应停止发布并回到最近通过的阶段，而不是通过 manifest `--force` 或跳过健康检查继续。
