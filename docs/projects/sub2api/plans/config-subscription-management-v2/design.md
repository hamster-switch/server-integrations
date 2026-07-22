# sub2api 配置订阅权限、编辑与内容管理改进：技术设计

## 1. 范围与仓库边界

本任务涉及三个代码库，但各自职责固定：

- `Proxy/sub2api-main`：sub2api 前后端、数据库迁移及自动化测试的开发工作区。
- `server-integrations`：补丁发布物和项目开发文档的正式维护仓库。历史资料迁移后，以该仓库内容为准。
- `Hamster Switch`：仅维护订阅 YAML 消费端契约测试和必要的跨项目链接，不继续存放 sub2api/new-api 补丁项目文档。

`server-integrations` 的文档布局采用：

```text
docs/projects/
├── sub2api/
│   ├── README.md
│   ├── history/2026-07-patch-upgrade/
│   └── plans/config-subscription-management-v2/
└── new-api/
    ├── README.md
    └── history/2026-07-patch-upgrade/
```

历史目录保留原任务的 `prd.md`、`design.md`、`implement.md`、`task.json`、`implement.jsonl` 和 `check.jsonl`，并在 README 标明来源任务、迁移日期、对应 Release/提交和父任务关系。新计划目录保存本任务收敛后的 `prd.md`、`design.md`、`implement.md`。迁移采用“先复制并校验文件清单/内容，再删除 Hamster Switch 中重复副本”的顺序。

## 2. 权限与订阅使用流程

### 2.1 两类 YAML 访问路径

必须区分管理面和数据面：

- 管理面：`GET /api/v1/keys/hamster-switch/subscriptions/:id/yaml` 用于后台预览，只允许管理员访问。路由中间件和 handler 均校验管理员身份，避免仅靠前端隐藏。
- 数据面：`GET /api/hamster-switch/subscription.yaml?token=hsc_...` 是 Hamster Switch 的稳定下载地址，继续按下载令牌鉴权并返回 YAML，不要求管理员登录。

普通用户列表把“查看 YAML”替换为“使用订阅”。弹窗复用已发布教程的渲染组件与数据源，并显示稳定订阅 URL 的复制操作，不加载管理面 YAML。管理员仍可从管理操作进入 YAML 预览、复制和下载。

公共 URL 本质是 bearer secret；文案提示用户妥善保管。权限目标是不在普通用户页面和登录态管理 API 主动暴露原始 YAML，而不是阻断 URL 持有人下载。

### 2.2 API 契约

- 普通用户请求管理面 YAML 返回 `403`，不存在资源与无权限的响应不得泄露其他用户订阅信息。
- 管理员预览支持按订阅 ID 获取渲染结果，但需要记录操作审计；响应不写入普通日志。
- 教程用户接口继续只返回已发布、合并后的教程内容；管理员接口返回可编辑状态。

## 3. 配置订阅编辑与密钥生命周期

### 3.1 前端表单状态

创建和编辑使用同一份表单模型：名称、分组、IP 白名单、IP 黑名单、额度、有效期、限速以及每模型限速。编辑时从详情接口获取完整状态，不再依赖列表行数据拼装。

分组密钥采用三态展示：

- 已存在分组：只显示“沿用现有密钥”，不返回、不回填、不允许修改密钥字符串。
- 新增分组：显示自定义密钥输入；空值表示后端自动生成。
- 已移除分组：提交后删除 provider 关联及其专用 API 密钥。

有效期沿用原生 API 密钥编辑的绝对时间语义；关闭额度、有效期或限速时，请求必须携带明确的清除语义，不能因为字段省略而保留旧值。

### 3.2 后端更新算法

更新服务先锁定并读取当前订阅/provider 集合，再按稳定 `group_id` 计算 `kept`、`added`、`removed`：

1. 校验订阅归属、目标分组可用性和表单高级设置。
2. `kept` 只更新 API 密钥元数据，不检查或替换其 key 字符串。
3. `added` 才校验自定义 key 的格式与唯一性；空值则生成新 key。
4. `removed` 删除 provider 关联和该关联指向的专用 key。
5. 更新订阅名称、provider 顺序和时间戳后一次性提交事务。

任何一步失败都回滚，下载 token/public ID 不变。删除专用 key 前以 provider 的 `api_key_id` 精确定位，不使用名称前缀；数据库唯一约束冲突转换为可理解的领域错误。

为避免并发编辑静默覆盖，详情返回 `updated_at`，更新请求携带该版本值；版本不匹配返回冲突，前端保留输入并提示重新载入。

## 4. 订阅专用 API 密钥隐藏设置

新增站点级布尔设置键（建议命名 `hide_hamster_subscription_api_keys`），默认 `false`，复用现有 settings 持久化与缓存失效机制。管理员专用 GET/PUT 契约返回和修改该值，普通用户只接收最终列表结果，不读取设置详情。

开启时，普通用户 API 密钥列表、分页总数和搜索查询统一增加：排除所有被 `hamster_config_subscription_providers.api_key_id` 引用的 key。判断依据是数据库关联，不使用 key 名称或前缀。管理员用户详情、审计、排障查询保持完整结果。

该过滤只在展示查询边界生效，认证、额度、统计、禁用/启用、订阅渲染及级联删除仍按原 API key 记录运行。设置切换不迁移、不改写任何 key，因此可立即回退。

## 5. 半开放式订阅模板

### 5.1 配置模型

模板版本由两部分组成，并作为一个发布单元保存：

- `presentation`：结构化 JSON，包含订阅级覆盖和按稳定 `group_id` 索引的 provider 覆盖。
- `layout`：高级 YAML 布局，控制字段顺序、可选字段和受限变量位置。

草稿表和版本表增加结构化 JSON 字段；现有 `body` 迁移为 `layout` 的兼容来源。旧记录读取时使用空 `presentation`，写入或发布后落为新结构。版本发布和回滚必须同时切换两部分，继续保留诊断、警告确认、创建人和活动版本信息。

结构化覆盖需要区分“未覆盖”和“显式空值”，因此字段使用可选值/覆盖标记，不能用空字符串直接代表两种含义。

### 5.2 默认值合并

渲染顺序为：系统安全默认值 → 当前站点/分组动态默认值 → 管理员结构化覆盖 → 高级布局。

- 订阅级：`name`、`icon`、`description` 分别优先来自现有 `site_name`、`site_logo`、`site_subtitle`；`home_url` 使用规范化后的 `api_base_url` 站点源地址，并在未配置时回退到服务的公开访问源地址。
- provider 级：`name=group.name`，`icon=platformIcon(group.platform)`，`description=""`。
- 不新增 group Logo 字段。平台图标映射集中维护并覆盖未知平台的稳定后备图标。
- 新分组按默认值自动出现；分组改名/平台变化时，未覆盖字段跟随变化，显式覆盖保持不变；删除分组时删除对应覆盖。

管理员可配置 Hamster schema 已支持且不含秘密的展示字段，例如 `website_url`、`category`、`notes`、`description`、`pricing`、`models`、`features`、`icon_color`、`sort_index` 和 `in_failover_queue`。运行必需的 ID、base URL、API key、端点等仍由受控数据生成，不能被任意模板代码替代。

### 5.3 变量与安全

建立显式 token 注册表，按订阅级/provider 级标注类型和允许位置。用户相关值使用 `{{provider.api_key}}` 等占位符；管理预览使用固定脱敏样例或原样占位符，绝不查询真实用户密钥。模板引擎禁止任意函数调用、文件/环境变量访问、动态 include 和未注册 token。

保存草稿时完成语法与 token 校验；发布时使用全部当前分组生成完整 YAML，经过结构校验及 Hamster Switch 解析契约测试。未知字段可以产生警告，但缺少必需字段、重复 provider ID、无效 endpoint 或未注册变量必须阻止发布。

### 5.4 管理界面

入口统一命名为“订阅模板管理”。界面包含：

1. 站点级展示字段。
2. 当前所有分组/provider 的可折叠字段编辑器。
3. 高级 YAML 布局编辑器和受支持变量清单。
4. 实时完整预览、诊断和警告。
5. 草稿保存、发布、历史与整版本回滚。

站点 Logo 继续复用设置页现有上传能力；模板页选择/预览该值，不另建上传存储。

## 6. 教程空状态修复

后端管理员接口始终返回规范对象：`base` 为带空 `steps` 的教程对象，`overrides` 为数组，`candidate` 可为 `null`，候选冲突为数组。数据库无记录和 JSON `null` 都在 DTO 边界规范化。

前端 API 层增加唯一的 `normalizeTutorialAdminState`，所有组件只消费规范状态；模板和 computed 中对步骤查找继续使用安全默认值。站点尚无基础教程时允许创建覆盖步骤，更新检查、采纳、拒绝和删除覆盖均返回可渲染状态。

## 7. 兼容性、迁移与回滚

- 数据库迁移只新增可空/带默认值字段和必要索引，不修改现有下载 token、subscription/provider 主键或 API key 内容。
- 旧模板版本仍可读取和回滚；若回滚到旧记录，结构化覆盖按空对象处理。
- 隐藏设置默认关闭，因此升级后用户 API 密钥列表不变。
- 公共 `hsc_` URL、YAML Content-Type 和缓存语义保持兼容。
- 若新模板发布后出现消费端问题，管理员可回滚上一完整版本；若隐藏查询有问题，可关闭设置；订阅编辑事务失败不会产生半套密钥。
- 发布补丁前备份数据库，并在安装脚本中保留现有迁移/容器健康检查与失败回滚机制。

## 8. 验证策略

- 后端：权限矩阵、事务增删分组、existing key 不变、custom key 唯一性、明确清空高级设置、并发冲突、普通/管理员密钥查询差异、模板版本迁移与回滚、教程 null 规范化。
- 前端：普通/管理员按钮差异、教程弹窗复用、编辑状态回填、新分组 key 输入、错误保留、隐藏开关、模板覆盖/default 行为、教程 null/empty 状态。
- 契约：使用 Hamster Switch 当前订阅模型解析包含 icon/description/endpoints 的生成 YAML，并验证 public URL 下载仍可导入。
- 文档：迁移前后文件清单和内容哈希一致，README 链接有效，Hamster Switch 中不存在重复项目文档。
