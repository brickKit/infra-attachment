# itest/：集成测试与测试索引

附件组件对着真实的 PostgreSQL、消息系统与对象存储（RustFS）、经它真实的接口的测试（L2）。构建标签 `integration`。

## 怎样跑

- `make test-integration`（`go test -count=1 -tags integration ./itest/`）；环境变量 `BE_SDK_TEST_PG_ADMIN`、`BE_SDK_TEST_NATS_URL`、`BE_SDK_TEST_S3_URL`、`BE_SDK_TEST_S3_ACCESS_KEY`、`BE_SDK_TEST_S3_SECRET_KEY`（装配项目的 `itenv.sh`）。
- 测试每次删掉再建 schema `infra_attachment_it`；对象放在 SDK 测试的桶 `be-sdk-test` 里，每次一个新前缀；端口 28204、9204 要空着。
- 属主由测试经 mTLS 扮演：`demo/orders`（`demo.order` 的属主）与 `demo/other`；类别目录是测试的替身（`demo.order_contract` 普通、`demo.order_invoice` 严格）。用户经预签名地址上传与下载，与浏览器相同。
- **只有一个测试** `TestAttachmentEndToEnd`，各步的顺序是 `upload`、`credentials`、`content`、`owners`、`strict`、`replace`、`lifecycle`、`missing`、`purge`。

## 文件

| 文件 | 步 | 测的流程 | 规则 |
| --- | --- | --- | --- |
| `uploads_integration_test.go` | `upload`、`credentials`、`content`、`strict`、`replace` | 会话、直传、确认、列出、普通下载的预签名地址保存为文件；上传没到时答 `UPLOAD_INCOMPLETE` 不用掉凭证、别人的凭证、篡改、用第二次；按内容检测出 HTML 被拒、确认之后再写暂存键不影响最终对象（确认**期间**的覆盖由 SDK 的条件复制保证，测在 be-sdk-go 的 `TestCopyOnlyWhenAbsent`）、超过上限与不收的类型在会话时被拒；严格类别的一次性凭证、响应头、别人的与第二次；替换：确认前下载旧版、一次一个、确认后切换、旧版本 `superseded` | AT03、AT04、AT06、AT07；FA02、FA03、FA05、FA06、FA09、FA23（篡改；过期没测）、FA25、FA30。FA22（甲的凭证用于乙）在结构上不会发生：凭证本身指定附件 |
| `purge_integration_test.go` | `purge` | 确认后暂存对象删除；替换内容与保留起算让事件的版本号加一；起算只往后移；最长保留已过时每天的对账告警；清除作业直接调用：没有起算、类别没给年限、最短保留未到、父记录保全、宽限期内都不清除，条件全满足才清除并删对象；已清除的不能保全 | AT09、AT11、AT12、AT13；FA11、FA31、FA35 |
| `lifecycle_integration_test.go` | `owners`、`lifecycle`、`missing` | 不是属主被拒并留下安全审计、目录未知答可重试且不审计；解除全部（进行中的上传一并作废）、恢复、父记录的保全挡住擦除、解除保全后擦除、对象删掉、记录留着；对象缺失时下载答依赖不可用、转 `missing` | AT02、AT08、AT10、AT11、AT12；FA10、FA11、FA13、FA20、FA24 |

共用的：

| 文件 | 内容 |
| --- | --- |
| `endtoend_integration_test.go` | `TestAttachmentEndToEnd`；`fixture`（运行中的成员与它的 `Module`、成员的日志、测试自己的存储视图、两个属主的系统面客户端、Bob 与 Carol 有确认与下载、Dave 没有授予）；`catalog` 替身；`start`；`dial`、`parent`、`put`、`get`、`pdf`；`client.do`、`portFree` |

没有自动测试的：每分钟的对账（会话到期、卡住的确认重做），每天对账里的孤儿删除与对象回来（要把对象的修改时刻拨到 7 天前，RustFS 不支持）；对象缺失的转换经下载触发测到了。清除与每天的告警由 `purge` 经入口的 `Module.Base()` 直接调用作业函数。

## 加测试

新流程加一个文件，在上表加一行；测试不为了通过而改。
