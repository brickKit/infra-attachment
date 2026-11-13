# lifecycle：属主命令的生命周期

## 负责与不负责

- 负责：`List`（AT07）；`Detach`（AT08：任何未清除的状态，进行中的上传作废、凭证用掉，可带保留起算）、`Restore`（宽限期内回到原状态）、`SetRetentionStart`（AT09）、`Erase`（AT10：保全中的保留并报出）、`Hold`、`ReleaseHold`（AT11）；`Purge`（作业 `JobPurge`：宽限期、起算、最短保留、保全都满足才清除，保留类别没配的从不清除）；`DeleteObjects`（提交后删全部版本的对象）。
- 不负责：上传与下载。

## 文件

`lifecycle.go`

## 依赖与被依赖

- 依赖：`core`、`catalog`、`records`；SDK 的 `storage`。
- 被依赖：入口、`reconcile`。

## 测试

`itest/lifecycle_integration_test.go`。
