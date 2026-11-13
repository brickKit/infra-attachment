# attachment/：入口与组装

组件对外的一个包：`main.go` 与外壳只引用它（`attachment.Definition()`、`attachment.New`）。这里只**组装**；业务一行也不写。

## 文件

| 文件 | 内容 |
| --- | --- |
| `definition.go` | `Version`、端口、迁移、`Keys` 与 `Own`（扫描端口、保留类别、浏览器可达的存储地址）；`Definition()`（流 `infra.attachment`、补发的操作） |
| `module.go` | `Module` 与 `New`（扫描端口只有 `off`、保留类别的 JSON、存储端口、授权成员的目录：格式不对成员不启动）；`UseCatalog`（集成测试换目录）、`Base`（集成测试直接调用作业）；`Routes`（确认、严格下载）、`Services`（`AttachmentService`：任何组件可调，属主核对在每个命令里）、`Jobs`（目录、每分钟与每天的对账、每小时的清除） |
| `services.go` | 系统面的九个方法转给模块，错误经 `catalog.Settle`（被拒的属主写安全审计） |
| `checks.go` | `RecoveryChecks`：首批没有自己的检查 |
| `contract_test.go` | 单元测试：清单的 `configSchema` 与 `events`、OpenAPI 与路由一一对应、合并安全 |

## 依赖与被依赖

- 依赖：`internal/` 下全部模块；契约模块 `infra-attachment/contract`。
- 被依赖：`main.go`、外壳、`itest/`。

## 测试

`go test ./attachment/`。
