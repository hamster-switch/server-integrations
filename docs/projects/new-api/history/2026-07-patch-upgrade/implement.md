# new-api 配置订阅实施清单

## 后端

- [x] 新增 GORM 模型、migration、唯一约束和所有权索引。
- [x] 实现事务创建、group diff 编辑、禁用/启用和不可恢复删除。
- [x] 扩展 list/detail/yaml 管理端点并切换 download 到持久化实体。
- [x] 删除旧无状态 URL decoder/签发流程和对应兼容测试。
- [x] 不按名称清理旧 Token；在开发升级说明记录手工清理方式。
- [x] 接入完整 YAML 模板、DSL、教程/覆盖层、图片和内容版本服务。

## default 前端

- [x] 新增独立“配置订阅” route、sidebar module 和 feature。
- [x] 实现完整功能矩阵及“配置订阅教程”。
- [x] 删除 API Keys 旧按钮、dialog mount/state 和无引用旧 dialog。
- [x] 使用 AlertDialog destructive 删除确认、提交态、toast 和刷新。

## classic 前端

- [x] 新增独立 route、SiderBar item/module config 和页面。
- [x] 使用 Semi 组件完成与 default 等价的完整功能矩阵。
- [x] 使用 `Modal.confirm`/Modal warning/danger 体系及同一删除正文。
- [x] 不在 Token/API Keys 页面增加兼容按钮。

## 测试

- [x] GORM 事务、所有权、group diff、URL 稳定性和失败回滚。
- [x] 禁用 410、启用恢复、删除 404、旧 `hs_` URL 拒绝。
- [x] 模板敏感上下文证明不含渠道 Key，硬错误/警告行为正确。
- [x] default/classic 按功能矩阵逐项运行共享 API contract tests/E2E。
- [x] 验证付费套餐订阅路由、文案和权限完全不受影响。

## 建议验证命令

按仓库实际 package scripts 校准，至少执行：

```text
go test ./controller ./model ./router

cd web/default
pnpm lint
pnpm type-check
pnpm build

cd web/classic
pnpm lint
pnpm build
```

补充补丁锚点/安装测试，并在固定上游版本完成 Linux 部署和回滚演练。

## 统一验收记录（2026-07-19）

- `go test ./controller ./model ./router ./service/hamsterswitch -count=1`、`go vet` 和根入口 `go build` 通过。
- default 新增文件 oxlint/oxfmt、全量 `tsgo -b` 与生产构建通过；classic 变更文件 Prettier 与生产构建通过。
- 全仓 default oxlint 存在上游基线错误，新增功能文件单独检查为零错误，未越界修改上游无关代码。
- 两套前端共同调用持久化订阅、模板和教程管理 API；源码契约检查确认功能入口、深链、删除文案及管理员更新操作等价。
- `new-api-v1.0.0` 精确绑定上游 `5a6c53d4966b2e34690ab49f3dd19be01c88fdbe`，24 个文件进入确定性补丁包。
- 补丁连续构建 SHA-256 一致，并通过真实 manual 应用/结果哈希检查/回滚/原始哈希恢复烟测。
- `server-integrations` 提交 `ac071039decf7cef2c97dc3dd902b84de4369df8` 已推送到
  `feat/new-api-config-subscriptions-v1`；注解标签 `new-api-v1.0.0` 已通过纯 Git 推送并触发签名工作流。
- 正式公开 Release 已发布：`https://github.com/hamster-switch/server-integrations/releases/tag/new-api-v1.0.0`。
  远端重新下载验真通过：非草稿、非预发布，Ed25519 签名有效，三个声明资产齐全，固定上游 24 个文件全部兼容。
- `server-integrations/main` 受保护，拒绝直接快进并要求 PR + `test` 检查；这不影响标签绑定提交或已发布的不可变正式 Release。
