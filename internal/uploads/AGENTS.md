# uploads：上传会话与确认

## 负责与不负责

- 负责：`CreateUpload`（AT03：新附件存类别快照，替换只对可用的附件、一次一个；大小与类型在会话时就按快照查；预签名上传地址与确认凭证）；`Confirm`（AT04：先查暂存对象，没有答 `UPLOAD_INCOMPLETE` 不用掉凭证；原子作废凭证并转 `finalizing`）；`Finalize`（条件复制到最终键、读最终对象算摘要与大小、按内容检测类型）与 `record`（按快照判定，新附件可用或被拒，替换切换当前版本、旧版本 `superseded`）。
- 不负责：下载、清除。

## 文件

`uploads.go`

## 依赖与被依赖

- 依赖：`core`、`catalog`、`records`；SDK 的 `storage`。
- 被依赖：入口、`reconcile`。

## 测试

`itest/uploads_integration_test.go`。
