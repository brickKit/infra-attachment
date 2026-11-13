# internal/：业务模块

附件组件的业务，每个模块一个包。只给本仓库引用（Go 的 `internal`）：别的组件编译期就引用不了（决定 0101）。

## 模块

| 包 | 一句话 | 依赖（本组件内） |
| --- | --- | --- |
| `reconcile` | 每分钟与每天的清理与对账 | `core`、`records`、`uploads`、`lifecycle` |
| `uploads` | 上传会话与确认 | `core`、`catalog`、`records` |
| `downloads` | 下载 | `core`、`catalog`、`records` |
| `lifecycle` | 列出、解除引用、恢复、保留、擦除、保全、清除 | `core`、`catalog`、`records` |
| `catalog` | 类别目录的副本与属主核对 | `core` |
| `records` | 附件行、状态的唯一写入、状态事件 | `core` |
| `core`（共用） | 名字、状态、参数、类别快照、`Base`、凭证的随机值与摘要、校验 | — |

分层（每个包只引用比它低的层，没有环）：

| 层 | 包 |
| --- | --- |
| 4 | `reconcile` |
| 3 | `uploads`、`downloads`、`lifecycle` |
| 2 | `catalog`、`records` |
| 1 | `core` |

核对：`go list -f '{{.Imports}}' ./internal/<包>`。

## 共同的写法

- **每个模块的函数先收 `m *core.Base`**（运行时、存储端口、目录的来源、保留类别、扫描端口）。
- **状态只经 `records.SetState` 写**，它在同一个事务里发状态事件。
- **系统面的命令先过 `catalog.Check`**；被拒时返回 `*catalog.Denied`，入口的 `catalog.Settle` 在事务回滚后写安全审计。
- **对象存储的操作在事务之外**，先写记录再动对象。

## 测试

由 `itest/` 的流程测试覆盖（真实的对象存储）。
