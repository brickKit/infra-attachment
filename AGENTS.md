# infra/attachment

附件组件的代码。用法与边界在 `BRICKKIT.md`，为什么这样设计（术语、数据、状态、规则、未决问题）在 [docs/design.md](docs/design.md)；跨组件的规则在装配项目 be-assembly-standard 的协议 12（附件与对象存储）、协议 11（保留类别）、协议 01（后台工作）、安全 09a（附件类别）与决定 D53、D68；对象的读写经线协议第 19 章的存储端口。

## 代码地图

代码按业务模块分层。**先读这一节，再进目录**：每个代码目录都有自己的 `AGENTS.md`（同目录的 `CLAUDE.md` 让 Claude Code 读这个目录的文件时自动带上它）。

| 目录 | 是什么 |
| --- | --- |
| [`attachment/`](attachment/AGENTS.md) | 入口：组件定义与组装（两条用户面路由、系统面 `AttachmentService`、作业、恢复核对）；不写业务 |
| [`internal/`](internal/AGENTS.md) | 业务：下面的模块各一个包；模块之间的依赖表在它的 `AGENTS.md` |
| [`itest/`](itest/AGENTS.md) | 集成测试（真实的 PostgreSQL、消息系统与 RustFS）；它的 `AGENTS.md` 是测试索引 |
| `attachment/migrations/` | 表：类别目录的副本、附件、内容版本、凭证、保全 |
| `cmd/seed/` | 种子（`make seed`）：角色与授予（确认上传与严格下载给要用附件的种子用户） |
| `contract/` | 本组件发布的契约，单独的模块：`contract/proto/`（系统面）、`contract/openapi.yaml`（用户面）、`contract/events/` |
| `docs/design.md` | 设计（状态表、AT01–AT14） |

业务模块（`internal/` 下）：

| 模块 | 负责 | 拥有的表 |
| --- | --- | --- |
| [`catalog`](internal/catalog/AGENTS.md) | 类别目录的副本（每分钟按修订同步）；属主核对（被拒的写安全审计） | `category`、`catalog_state` |
| [`records`](internal/records/AGENTS.md) | 附件行的读取、状态唯一的写入（`SetState`）、状态事件 | `attachment`（状态） |
| [`uploads`](internal/uploads/AGENTS.md) | 上传会话（新附件或替换）、确认（先查暂存、原子作废凭证、条件复制到最终键、读最终对象检查） | `attachment`（新建）、`version`、`credential`（确认） |
| [`downloads`](internal/downloads/AGENTS.md) | 下载：普通类别的预签名地址、严格类别的一次性凭证与转发字节；对象缺失转 `missing` | `credential`（下载） |
| [`lifecycle`](internal/lifecycle/AGENTS.md) | 列出、解除引用、恢复、保留起算、擦除、保全；按宽限期与保留清除 | `hold` |
| [`reconcile`](internal/reconcile/AGENTS.md) | 每分钟：卡住的确认重做、到期会话；每天：孤儿、缺失与回来、清除后剩下的对象、过期凭证 | — |

共用的：[`core`](internal/core/AGENTS.md)（名字、状态、参数、类别快照、`Base`、凭证的随机值与摘要、校验）。

| 想改 | 从这里开始 | 然后 |
| --- | --- | --- |
| 附件在某个状态下能做什么 | `docs/design.md` 的状态表 | 写状态只经 `records.SetState`；各模块里对应的命令 |
| 谁能为哪个资源类型申请凭证 | AT02 | `internal/catalog` 的 `Check`、`Settle` |
| 上传内容怎样核对 | AT03、AT04 | `internal/uploads` 的 `Finalize`、`record` |
| 下载怎样给、严格类别的响应头 | AT06 | `internal/downloads`；SDK 的 `httpx.Context.Download` |
| 什么时候清除 | AT08–AT11 | `internal/lifecycle` 的 `Purge`、`Erase` |

## 构建与测试

- `go test ./...`（`make test`）：清单的 `configSchema` 与 `events`、OpenAPI 与路由、合并安全（`attachment/contract_test.go`）。
- `make test-integration`（`go test -tags integration ./itest/`）：要 `BE_SDK_TEST_PG_ADMIN`、`BE_SDK_TEST_NATS_URL` 与 `BE_SDK_TEST_S3_*`（装配项目的 `itenv.sh`），用 SDK 测试的桶 `be-sdk-test`、每次一个新前缀。
- `make -C contract check`：契约的 protobuf 风格、生成代码与构建。
- `brickkit lint`

## 设计取舍

- **扫描与下载只针对最终键**：确认时先把暂存对象条件复制到不可覆盖的最终键，再读最终对象检查；客户端之后再写暂存键改变不了它。
- **凭证只存摘要，作废是原子的条件更新**，与它引起的状态变化同一事务。
- **属主核对被拒时安全审计另开事务**：命令的事务因拒绝而回滚（`catalog.Settle` 在回滚之后写）。
- **先改记录、再动对象**：对象删不掉、复制卡住，都由对账补齐。

## 易错点

- 不要信客户端声明的类型与大小：按最终对象自己算与检测。
- 不要在命令的事务里写被拒的安全审计（会跟着回滚）：返回 `*catalog.Denied`，由入口的 `Settle` 写。
- 不要把预签名地址写进日志（里面有签名与文件名）。

## 改代码前自查

- 改了契约（`contract/`）：proto 改后 `make -C contract gen`，发布 `contract/v…`，再改这里的依赖。
- 改了版本号吗？每次改动都升版本。

## BrickKit

这是一个 BrickKit 组件：平台只读 `component.yaml`。它依赖的规则：

- `configSchema` 的键就是代码读的环境变量名。不能用保留名：`COMPONENT_ID`、`COMPONENT_VERSION`、`PORT`、`BRICKKIT_SERVED_MEMBERS`、`BRICKKIT_SERVED_MEMBERS_CONFIG`，以及任何 `*_ENDPOINT`。
- 依赖写精确版本。依赖的地址以 `<ID>_ENDPOINT` 注入；缺席的可选依赖根本没有这个变量，读的时候要带兜底。
- `/healthz` 只查本进程，不查依赖。迁移命令用同一个镜像跑，遇到不认识的参数必须直接失败；`local.migrationCommand` 从源码跑同一条迁移，给 `mode: local` 用。
- `BRICKKIT.md` 会随版本进入每个使用它的项目，在那里是脱离仓库单独读的：跟代码一起改，不放相对链接。
- 发版：改 `metadata.version`，提交、推送，`brickkit release`。`brickkit lint` 会检查清单和这些文档——在项目里、在这个目录下跑，只查这个组件（`--all` 查整个项目）。
- 完整规则在 `brickkit-component` 技能里（装了技能的项目或仓库根目录下的 `.claude/skills/brickkit-component/SKILL.md`；`brickkit skills update` 会装上）；参数问 `brickkit <命令> --help`；BrickKit 自己的文档用 `brickkit docs` 看。
