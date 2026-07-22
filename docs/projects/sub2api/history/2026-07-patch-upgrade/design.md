# sub2api 配置订阅设计

## 数据模型

新增配置订阅实体和关联表（实际命名遵循 sub2api Ent/迁移规范）：

- `config_subscription`：内部 ID、公开 ID、user_id、名称、status、稳定下载 token、创建/更新时间、删除时间。
- `config_subscription_provider`：subscription_id、group_id、api_key_id、排序位置和必要展示快照；`subscription_id + group_id` 唯一。

下载 token 可恢复保存，以便管理页直接拼装/复制完整 URL，安全基线对齐现有 sub2api API Key。旧版按 API Key 名称聚合的状态不迁移，新实体不再依赖名称编码作为 SSOT。

## 后端端点

所有管理端点沿用用户认证和所有权校验：

| 方法 | 路径语义 | 行为 |
| --- | --- | --- |
| GET | `/api/v1/keys/hamster-switch/options` | 可选分组/预览输入 |
| GET | `/api/v1/keys/hamster-switch/subscriptions` | 分页搜索当前用户配置订阅 |
| POST | `/api/v1/keys/hamster-switch/subscriptions/preview` | 不落库预览 |
| POST | `/api/v1/keys/hamster-switch/subscriptions` | 事务创建实体和专用 API Key |
| GET | `/.../subscriptions/:id` | 返回详情、完整 URL/凭据及视觉脱敏所需数据 |
| PUT | `/.../subscriptions/:id` | 事务化名称/分组/限制编辑，URL 不变 |
| POST | `/.../:id/disable`、`enable` | 可恢复状态切换 |
| DELETE | `/.../subscriptions/:id` | 不可恢复删除 |
| GET | `/.../subscriptions/:id/yaml` | 管理页完整 YAML |
| GET | 公共下载 URL | 按 token 解析实体并生成当前 YAML |

具体路径可为保持补丁锚点而调整，但语义和状态码不得变化。禁用返回 `410`，不存在/删除/旧格式返回 `404`。

## 写入事务

创建先校验分组权限、数量限制和预览硬错误，再在同一事务创建实体、每组专用 API Key 和关联。编辑计算 group diff：未变项保留原 Key；新增项创建 Key；移除项撤销 Key。任一步失败回滚，原 URL 必须继续生成旧完整 YAML。

禁用/启用同时修改实体和全部关联 Key 状态。删除先校验所有权，再删除关联 Key 并软删除实体；用户侧无恢复端点。

## YAML 与内容管理

渲染输入由服务层构造闭合 DTO，不把 repository/Ent 实体直接暴露给模板。模板管理提供草稿、诊断、发布历史、回滚和候选远程内容差异。发布硬校验包含 DSL、渲染、YAML、Hamster 必需字段和禁用变量；警告确认写入版本元数据。

教程使用稳定步骤 ID、通用版本、站点覆盖和站点新增步骤。页面标题与入口均为“配置订阅教程”。

## 前端

在现有 `SubscriptionManagementView.vue` 基础上重构为“配置订阅”，继续使用 sub2api 的 BaseDialog、ConfirmDialog、DataTable、按钮、toast、暗色与响应式体系。列表提供创建、复制 URL、导入 Hamster、配置订阅教程、编辑、禁用/启用、删除和 YAML 查看/复制/下载。

密钥默认调用 `maskApiKey` 视觉脱敏；页面 DTO 已含完整值，复制和下载直接使用完整内容。删除正文严格为 `确定要删除'{name}'吗？此操作无法撤销。`。

## 兼容与边界

- 旧签名 URL 和名称聚合列表升级后失效，不实现迁移适配器。
- 升级不静默删除旧开发 API Key。
- 不改变 sub2api 其他 API Key 页面、安全模型或删除审计实现。
- 新功能只面向 Linux 生产补丁交付，现有 PowerShell 脚本不扩展等价能力。
