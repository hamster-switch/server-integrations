# new-api 集成项目文档

本目录是 Hamster Switch `new-api` 服务端集成补丁的开发文档归档位置。补丁发布物位于 [`releases/new-api/`](../../../releases/new-api/)，正式版本以 GitHub Release 和签名 manifest 为准。

## 文档索引

- [`history/2026-07-patch-upgrade/`](history/2026-07-patch-upgrade/)：从 Hamster Switch Trellis 任务 `07-19-new-api-patch-upgrade` 迁移的历史 PRD、设计、实施与验证记录；原父任务为 `07-19-719-update-planning`。
- 对应正式版本：[`new-api-v1.0.0`](https://github.com/hamster-switch/server-integrations/releases/tag/new-api-v1.0.0)，任务记录提交 `ac071039decf7cef2c97dc3dd902b84de4369df8`。

## 维护边界

- new-api 功能修改先在固定上游源码树中完成并通过测试，再生成确定性补丁资产。
- 本仓库维护补丁文档、签名 manifest 和 Release 资产；Hamster Switch 主仓库只维护消费端协议及跨项目链接。
- 历史 Trellis 文件作为决策与验收证据保留，不作为本仓库的活动任务状态。
