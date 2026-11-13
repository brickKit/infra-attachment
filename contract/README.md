# infra/attachment 的契约

组件 `infra/attachment`（附件）对外发布的契约，单独的 Go 模块（标签 `contract/v3.…`），随组件的版本发布：

| 文件 | 内容 |
| --- | --- |
| `proto/be/attachment/v3/attachment.proto` | 系统面 `AttachmentService`（父记录的属主调用）：申请上传会话、申请下载、列出、解除引用、恢复、保留起算、擦除、保全与解除；生成代码在 `gen/go`、`python` |
| `openapi.yaml` | 用户面：凭确认凭证确认上传、凭一次性凭证下载严格类别 |
| `events/infra.attachment.changed.v1.schema.json` | 附件的完整状态（按版本覆盖） |

属主经系统面调用；本组件核对调用者是父资源类型的属主（类别目录来自授权成员），从不调用属主（D68）。

- Go：`go get github.com/brickKit/infra-attachment/contract/v3@v3.0.0-alpha.1`，导入 `github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3`。
- 修改：proto 改后 `make gen`，`make check` 通过，只做加法（`make breaking BASE=contract/v3.0.0-alpha.1`），发布新的 `contract/v…`。
