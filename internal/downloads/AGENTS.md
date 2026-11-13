# downloads：下载

## 负责与不负责

- 负责：`IssueDownload`（AT06：只给可用的；先查最终对象，缺失转 `missing` 答依赖不可用；普通类别 5 分钟的预签名地址，严格类别 1 分钟的一次性凭证；签发写审计）；`Download`（严格类别：原子作废凭证、是本人的、版本还是当前的，经 SDK 的 `Download` 转发字节，保存不执行）。
- 不负责：上传、清除。

## 文件

`downloads.go`

## 依赖与被依赖

- 依赖：`core`、`catalog`、`records`；SDK 的 `storage`、`httpx`。
- 被依赖：入口。

## 测试

`itest/uploads_integration_test.go` 的 `strict`、`itest/lifecycle_integration_test.go` 的 `missing`。
