# new-api 配置订阅设计

## 数据模型

使用 GORM migration 新增独立实体：

- `HamsterConfigSubscription`：ID、public ID、user ID、name、status、稳定下载 token、时间戳和软删除字段。
- `HamsterConfigSubscriptionProvider`：subscription ID、group、token ID、排序位置；同一订阅内 group 唯一。

它与付费套餐 subscription 模块使用不同表、模型、路由和侧边栏 key，避免概念/权限交叉。开发期旧 `hs_<payload>.<signature>` URL 不迁移；`${name} - ${group}` Token 不可靠识别，升级不自动删除。

## 端点与状态机

在现有 `/api/hamster-switch/subscriptions` 命名空间扩展 list/detail/update/disable/enable/delete/yaml；download 只接受新持久化实体 token。管理端点使用 `UserAuth` 和 user ID 所有权约束。

创建与编辑在 GORM transaction 内同步实体、专用 Token 和 provider 关联。普通编辑保留 public ID/download token；禁用同时把关联 Token 设为 disabled，下载返回 `410`；启用恢复原 Token；删除关联 Token 并软删除实体，下载返回 `404`。

旧无状态 decoder、旧 API Keys 弹窗签发路径和旧格式测试删除。开发 Token 留给开发者手工清理。

## default/classic 功能矩阵

两套前端必须共享后端 DTO、状态语义和验收用例：

| 能力 | default | classic |
| --- | --- | --- |
| 配置订阅侧边栏/路由 | TanStack route + 原布局组件 | React Router + SiderBar/Semi |
| 列表/搜索/状态 | 必须 | 必须 |
| 创建和 provider 预览 | 必须 | 必须 |
| 复制 URL/完整 YAML | 必须 | 必须 |
| 导入到 Hamster | 必须 | 必须 |
| 配置订阅教程 | 必须 | 必须 |
| 编辑、禁用/启用、删除 | 必须 | 必须 |
| 管理员内容面板 | 对应管理体系 | 功能等价入口 |

允许视觉细节不同，不允许 classic 只有入口、default 才能管理。

## 旧入口移除

default 从 `api-keys-primary-buttons.tsx` 移除按钮，从 `api-keys-dialogs.tsx` 移除挂载和状态；API 类型/请求迁移到新 feature。无引用的 `hamster-switch-dialog.tsx` 删除。classic 不补兼容入口。API Keys 页面不跳转到配置订阅。

## 内容与安全上下文

模板/教程契约与 sub2api 共用 schema/夹具，但实现落在 new-api 代码体系。模板上下文只含订阅专用 Token、对外地址、group/provider、models、服务器生成安全配置和允许元数据；渠道 Key 永不进入上下文。

配置订阅页面按 sub2api 形式视觉脱敏，完整 Token 已在有权页面 DTO 内。该策略只用于 Hamster 配置订阅，不更改原 Token 页面按需取明文逻辑。

## 命名隔离

Hamster 功能统一显示“配置订阅”和“配置订阅教程”，使用独立 route/sidebar module key。现有付费套餐“订阅管理”“我的订阅”和 `/subscription`/`/subscriptions` 管理路由不改名、不复用。
