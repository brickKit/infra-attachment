# catalog：类别目录与属主核对

## 负责与不负责

- 负责：表 `category`、`catalog_state`；`GRPC`（授权成员的 `CatalogService.AttachmentCategories`）；`Sync`（作业 `JobSync`：按修订取，变了整份替换，读不到时用已有的、超过 15 分钟告警，AT01）；`Owner`、`Check`（AT02：不认识的资源类型答可重试的 `DEFINITION_PENDING`，不是属主返回 `*Denied`）；`Settle`（被拒的在事务回滚后写安全审计、答 `PERMISSION_DENIED`）。
- 不负责：附件本身。

## 文件

| 文件 | 内容 |
| --- | --- |
| `catalog.go` | `JobSync`、`GRPC`（`Fetch`）、`Sync`；`Owner`、`Check`、`Denied`、`Settle` |

## 依赖与被依赖

- 依赖：`core`；授权成员的契约 `contract-infra-authz` alpha.7。
- 被依赖：`uploads`、`downloads`、`lifecycle`、入口。

## 测试

`itest/lifecycle_integration_test.go` 的 `owners`。
