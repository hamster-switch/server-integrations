# new-api 集成项目文档

本目录是 Hamster Switch `new-api` 服务端集成补丁的开发文档归档位置。补丁发布物位于 [`releases/new-api/`](../../../releases/new-api/)，正式版本以 GitHub Release 和签名 manifest 为准。

## 文档索引

- [`history/2026-07-patch-upgrade/`](history/2026-07-patch-upgrade/)：从 Hamster Switch Trellis 任务 `07-19-new-api-patch-upgrade` 迁移的历史 PRD、设计、实施与验证记录；原父任务为 `07-19-719-update-planning`。
- 对应正式版本：[`new-api-v1.0.0`](https://github.com/hamster-switch/server-integrations/releases/tag/new-api-v1.0.0)，任务记录提交 `ac071039decf7cef2c97dc3dd902b84de4369df8`。

## New API Hamster 通道

当前候选 `new-api-hamster-v1.0.0` 固定在
`QuantumNous/new-api@1721144221ec5c94dd87891a7ae1bee228e7bb63`，候选 manifest
和确定性补丁位于
[`releases/new-api/hamster/1.0.0/`](../../../releases/new-api/hamster/1.0.0/)。
该通道不迁移旧 `new-api-v1.0.0`、`5a6c53d` 表结构或 `hs_` URL。

正式发布使用两个不可变标签：源码补丁
`new-api-hamster-v1.0.0`，预构建资产
`image-new-api-hamster-v1.0.0`。预构建版本只支持 Linux amd64，包含
`hamster-switch/new-api:hamster-v1.0.0` 镜像、嵌入前端的 systemd 二进制、
版本绑定安装器和 `SHA256SUMS`。Compose 与 systemd 均检查 `/api/status`，失败
自动恢复原镜像或二进制；服务器不执行 Bun、Go 或 Docker build。

安装、显式 Compose 参数、备份恢复和故障排查命令以仓库根目录
[`README.md`](../../../README.md) 为准。

## 维护边界

- new-api 功能修改先在固定上游源码树中完成并通过测试，再生成确定性补丁资产。
- 本仓库维护补丁文档、签名 manifest 和 Release 资产；Hamster Switch 主仓库只维护消费端协议及跨项目链接。
- 历史 Trellis 文件作为决策与验收证据保留，不作为本仓库的活动任务状态。
