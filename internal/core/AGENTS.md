# core：全组件共用的名字与写法（共用）

## 负责与不负责

- 负责：组件 ID、流与事件类型、配置键（扫描、保留）、资源与操作、附件与版本的状态、凭证的操作、参数（协议 12 的起始值）、类别快照的类型、资源定义 `Definitions()`（`names.go`）；`Base`、`Retention`、目录来源接口 `CatalogSource`、凭证的随机值与摘要 `NewSecret`、`Digest`（`base.go`）；授权（`Check`）与校验（`Invalid`、`NotFound`）（`rules.go`）。
- 不负责：任何业务规则。

## 文件

`names.go`、`base.go`、`rules.go`

## 依赖与被依赖

- 依赖：只有 SDK。
- 被依赖：全部模块与入口。

## 测试

入口的单元测试与全部流程测试间接覆盖。
