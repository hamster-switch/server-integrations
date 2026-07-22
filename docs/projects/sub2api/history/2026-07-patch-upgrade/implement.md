# sub2api 配置订阅实施清单

## 数据与服务

- [x] 增加 Ent schema、SQL migration、唯一约束和所有权索引。
- [x] 增加 repository/service DTO 和事务创建、编辑、禁用/启用、删除方法。
- [x] 用持久化实体替换 API Key 名称聚合逻辑，删除旧 URL 解码兼容。
- [x] 保持旧开发 Key 不自动删除，并在升级说明中给出手工清理方式。

## API 与模板

- [x] 调整 options/preview/create/list/detail/update/status/delete/yaml/download 路由。
- [x] 统一 400/401/404/410/422/500 错误语义及所有权隐藏。
- [x] 实现闭合模板上下文、受限 DSL、硬错误/警告和版本发布/回滚。
- [x] 实现教程通用层、站点覆盖、图片上传/外链缓存和远程候选差异。

## 前端

- [x] 页面和导航统一改为“配置订阅”，教程改为“配置订阅教程”。
- [x] 补齐创建/预览、复制、深链导入、编辑、禁用/启用和删除。
- [x] 复用原 API Key 脱敏、ConfirmDialog、成功/失败提示和刷新逻辑。
- [x] 删除旧文案、旧 URL 假兼容和无引用逻辑。

## 测试

- [x] 创建多分组成功及任一 Key 创建失败整体回滚。
- [ ] 编辑新增/保留/移除分组，URL 不变；失败仍返回旧 YAML。
- [ ] 禁用返回 410、启用恢复原 URL/Key；删除返回 404 且无恢复。
- [x] 旧 URL 格式拒绝。
- [ ] 跨用户访问由 PostgreSQL 集成测试覆盖。
- [x] 模板闭合 DSL、硬错误/警告和教程签名/图片安全单元测试。
- [ ] 远程冲突不自动覆盖由 PostgreSQL 集成测试覆盖。
- [x] 前端类型检查与生产构建。
- [ ] 暗色、移动布局及删除文案人工视觉验收。

## 本轮验证记录

- 后端除上游已确认的 Windows `internal/config` 基线失败外，全包测试通过，包含 `cmd/server` 装配编译。
- 前端 `pnpm typecheck` 与 `pnpm build` 通过。
- 更新器 `go test ./...`、`go vet ./...` 和 Linux/amd64 交叉构建通过。
- 补丁包连续构建两次 SHA-256 一致；安装器 7 个修改文件的应用步骤连续执行两次且逐文件哈希不变。
- 本机没有可用 Linux/WSL、GCC/CGO 和 systemd，Linux race、manual/systemd 部署及 PostgreSQL 生命周期集成测试交由 Ubuntu CI/测试服务器完成。

## 建议验证命令

按目标仓库现有脚本执行，至少包含：

```text
cd backend
go test ./internal/hamsteryaml ./internal/handler ./internal/server/routes

cd frontend
pnpm lint
pnpm type-check
pnpm build
```

另运行补丁仓库锚点/安装检查，并在固定上游版本完成一次 Linux manual 与自动部署演练。
