# records：附件行与它的状态

## 负责与不负责

- 负责：`Attachment`（带当前版本的大小、类型、未扫描）；`Load`、`ByID`、`Of`（父记录的、不含已清除的）；`SetState`（状态唯一的写入，同事务发事件）、`Publish`（`infra.attachment.changed.v1`，AT13）。
- 不负责：命令的业务判断。

## 文件

`records.go`

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：全部业务模块。

## 测试

全部流程测试。
