# sub2api 配置订阅权限、编辑与内容管理改进：实施计划

## 执行原则

- 本任务按下列阶段顺序实施，每一阶段单独提交、单独验证；后一阶段不得把前一阶段的失败留到发布时处理。
- 开发前在目标仓库运行 `trellis-before-dev`，读取对应 package/layer 规范。
- 不覆盖 Hamster Switch 当前工作树中与本任务无关的 `subscription/parser.rs`、`subscription/tests.rs` 和 `.tmp-sub2api-v1.0.1/` 变更。
- 业务代码完成前不生成或发布补丁；Release 是所有功能和契约验证通过后的独立门禁。

## 阶段与依赖

| 阶段 | 可独立验收的交付物 | 显式依赖 |
| --- | --- | --- |
| A | 两个补丁项目文档迁移和索引 | 无 |
| B | 数据迁移、站点设置和后端基础契约 | 无；但模板代码依赖其 schema |
| C | 权限、订阅编辑和密钥隐藏后端 | B |
| D | 半开放模板和教程空状态后端 | B |
| E | 普通用户/管理员前端交互 | C、D 的 API 契约 |
| F | 跨仓库测试、补丁打包和发布候选 | A–E |

## A. 开发文档迁移

- [ ] 在 `server-integrations/docs/projects/sub2api/` 和 `docs/projects/new-api/` 创建 README 索引，说明项目范围、历史来源、对应 releases 和维护方式。
- [ ] 将 Hamster Switch 归档任务 `07-19-sub2api-patch-upgrade` 的六个任务文件迁移到 `sub2api/history/2026-07-patch-upgrade/`。
- [ ] 将 `07-19-new-api-patch-upgrade` 的六个任务文件迁移到 `new-api/history/2026-07-patch-upgrade/`。
- [ ] 将本任务最终 `prd.md`、`design.md`、`implement.md` 复制到 `sub2api/plans/config-subscription-management-v2/`，并注明 Trellis 源任务 ID。
- [ ] 比对源/目标文件清单和 SHA-256 后，删除 Hamster Switch 中两套重复历史任务；如主项目需要追溯，仅保留指向新目录的简短索引。
- [ ] 检查所有 Release、提交、父任务和相对链接，确保迁移没有丢失证据。

验证：

```powershell
Get-ChildItem -Recurse docs/projects/sub2api,docs/projects/new-api
git diff --check
```

## B. 数据与后端基础契约

- [ ] 为模板草稿/版本增加结构化 `presentation` JSON，并提供旧 `body`/新 `layout` 的兼容读取迁移；迁移可重复执行且可回滚。
- [ ] 增加 `hide_hamster_subscription_api_keys` 站点设置常量、默认 `false`、读取/更新服务及缓存失效。
- [ ] 定义模板 DTO、覆盖字段的“未设置/显式空值”表达、受限 token 注册表和统一错误码。
- [ ] 定义订阅编辑详情/更新 DTO：绝对有效期、清除标记和 `updated_at` 乐观锁版本。
- [ ] 为 provider 关联过滤所需查询建立或确认索引，避免普通 API 密钥分页查询退化。
- [ ] 更新 Ent schema、SQL migration、测试 schema/fixture，并验证旧数据升级后行为不变。

重点文件：

- `backend/ent/schema/hamster_config.go`
- `backend/internal/service/domain_constants.go`
- `backend/internal/handler/dto/`
- `backend/migrations/`

## C. 权限、编辑和密钥可见性后端

- [ ] 把登录态 YAML 预览路由移入 `AdminOnly` 管理路由，handler 再做管理员防御性校验；公共 `hsc_` 下载路由保持不变。
- [ ] 为管理员 YAML 预览补充审计记录，并保证日志不输出下载 token、API key 或完整 YAML。
- [ ] 重构配置订阅更新：在事务内按 `group_id` 计算保留/新增/移除集合。
- [ ] 保留组只更新原生 API key 编辑允许的元数据；不验证、不替换 key 字符串。
- [ ] 新增组支持自定义 key 或自动生成；移除组删除 provider 和其 `api_key_id` 指向的专用 key。
- [ ] 对额度、有效期和限速实现显式清除；保持 public ID/download token 稳定；并发版本冲突返回 `409`。
- [ ] 普通用户 API key 列表、搜索和分页总数按 provider 关联排除订阅专用 key；管理员查询保持完整。
- [ ] 增加设置开关的管理员 GET/PUT API 和权限测试。

重点文件：

- `backend/internal/server/routes/user.go`
- `backend/internal/server/router.go`
- `backend/internal/handler/hamster_switch_subscription.go`
- `backend/internal/service/config_subscription_service.go`
- `backend/internal/service/api_key_service.go`
- `backend/internal/repository/api_key_repo.go`

## D. 模板和教程后端

- [ ] 将模板领域命名从 YAML template 调整为 subscription template；必要时保留旧 API 路由别名一个兼容周期。
- [ ] 实现站点级默认值、`platform -> Hamster icon` 映射及未知平台后备值。
- [ ] 读取所有当前可订阅分组并生成 provider 默认配置；覆盖按稳定 `group_id` 保存。
- [ ] 处理分组生命周期：新增自动出现，改名/平台变化仅更新未覆盖默认值，删除同步清除覆盖。
- [ ] 扩展安全展示字段和 token 白名单；禁止任意模板函数、环境变量、文件及秘密数据访问。
- [ ] 草稿保存、完整预览、发布和回滚都同时处理 `presentation + layout`，预览 API 只使用占位 API key。
- [ ] 发布前以全部当前分组渲染并校验 YAML、provider 唯一性、endpoint 必填项和 Hamster schema。
- [ ] 统一教程管理员响应规范：无数据时提供空 `steps`、空 `overrides`、可空 `candidate`；所有操作返回规范状态。

重点文件：

- `backend/internal/handler/hamster_switch_template.go`
- `backend/internal/handler/hamster_switch_tutorial.go`
- `backend/internal/hamsteryaml/template.go`
- `backend/internal/handler/hamstertutorial/`

## E. 前端交互

- [ ] 在 `SubscriptionManagementView.vue` 中按用户角色渲染“使用订阅”或管理员 YAML 管理操作。
- [ ] 提取/复用教程展示组件；普通用户弹窗显示已发布教程和订阅 URL，不调用 YAML 预览 API。
- [ ] 编辑时调用详情接口并完整回填表单；已有组展示“沿用现有密钥”，新增组才显示自定义密钥输入。
- [ ] 支持明确关闭额度/有效期/限速、冲突重载提示、事务错误保留输入和成功后刷新。
- [ ] 在管理员页面按指定位置加入“隐藏 API 密钥显示”开关，显示保存中/失败/当前持久值状态。
- [ ] API key 页面无需自行猜测过滤，严格显示后端结果和正确分页总数。
- [ ] 管理入口及文案统一改为“订阅模板管理”。实现站点字段、全部 provider 字段、覆盖状态、高级布局、token 清单、实时预览、诊断、发布历史与整版本回滚。
- [ ] 复用现有 `site_logo` 上传/预览；不添加 group Logo 表单。
- [ ] 在 API 层实现教程管理员状态规范化，修复所有 `.find`/`.some`/`.steps` 空值访问。

重点文件：

- `frontend/src/views/user/SubscriptionManagementView.vue`
- `frontend/src/api/hamsterSwitchSubscriptions.ts`
- `frontend/src/views/user/KeysView.vue`
- 新增或复用的教程、模板子组件及对应测试

## F. 自动化验证与发布门禁

### 后端测试

- [ ] 权限矩阵：普通用户管理面 YAML `403`、管理员成功、合法公共 token 成功、非法 token 失败。
- [ ] 编辑矩阵：仅改元数据、添加自动 key、添加自定义 key、重复 key、移除组、全部清除、并发冲突、事务回滚。
- [ ] 隐藏矩阵：设置开/关、普通列表/搜索/总数、管理员查询、订阅鉴权和下载不受影响。
- [ ] 模板矩阵：默认值、显式空、platform icon、分组新增/修改/删除、token 拒绝、旧版本兼容、整版本回滚、预览不含真实秘密。
- [ ] 教程矩阵：`base=null`、`overrides=null`、`candidate=null`、空数组及首次创建。

```powershell
Set-Location 'D:\mini_project\Hamster switch\Proxy\sub2api-main\backend'
go test ./internal/handler/... ./internal/service/... ./internal/repository/... ./internal/hamsteryaml/...
go test ./...
```

### 前端测试

- [ ] 新增组件/API 单元测试覆盖普通与管理员角色、表单回填、密钥三态、隐藏开关、模板默认/覆盖、教程 null 规范化。

```powershell
Set-Location 'D:\mini_project\Hamster switch\Proxy\sub2api-main\frontend'
pnpm test:run
pnpm typecheck
pnpm lint:check
pnpm build
```

### Hamster 契约和端到端冒烟

- [ ] 用生成的 YAML 运行 Hamster Switch 订阅 parser 测试，至少覆盖顶层 icon/description/home_url、provider icon/description、endpoints.url 和唯一 provider ID。
- [ ] 本地启动升级后的 sub2api，创建订阅 → 编辑增删分组 → Hamster 导入 → 刷新 → 切换隐藏设置 → 管理员回滚模板。
- [ ] 检查容器日志和浏览器网络响应，不得出现 `internal error`、唯一约束错误、秘密泄漏或教程空值异常。

```powershell
Set-Location 'D:\mini_project\Hamster switch\Hamster Switch\src-tauri'
cargo test subscription
```

### 补丁发布候选

- [ ] 仅在上述门禁全部通过后，重新生成可直接安装的预编译补丁资产、manifest、校验和及通用一行安装脚本。
- [ ] 在临时全新安装和已有版本覆盖升级两种环境验证，不要求用户本机构建前端。
- [ ] 先创建候选 tag/release 并记录回滚版本；未经用户明确要求不推送、不建立 PR、不发布正式 Release。

## 风险文件与回滚点

- 数据迁移：模板草稿/版本表；回滚前必须确认新版本是否已发布，保留旧 `body` 数据直至兼容期结束。
- 凭据生命周期：`config_subscription_service.go`；删除只允许命中 provider 关联的 `api_key_id`，禁止按名称批量删除。
- 列表过滤：`api_key_repo.go`；总数和结果必须使用同一 predicate，发现回归可立即关闭隐藏设置。
- 权限路由：不能误伤公共下载接口；路由测试是合并前硬门禁。
- 文档迁移：在目标仓库校验完成前不删除源文档；删除后可从 Git 历史恢复。

## 启动前检查

- [ ] 用户已审阅并批准 `prd.md`、`design.md`、`implement.md`。
- [ ] 目标三个仓库的分支、remote、dirty 状态已记录，且已隔离无关改动。
- [ ] 已为 sub2api 数据库迁移和补丁安装准备可恢复备份方案。
- [ ] 已确认本轮实施范围是否包含正式 Release；默认仅做到可验证的发布候选。
