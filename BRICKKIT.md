# infra/attachment

## 组件定位

附件：替父记录的属主（采购订单、合同……的拥有组件）保存附件的记录与对象。
- **上传**：属主按自己的授权判断用户能否加附件，经系统面申请上传会话；用户经预签名地址直传到暂存键，凭确认凭证确认；本组件把暂存对象条件复制到不可覆盖的最终键，再读最终对象自己算摘要与大小、按内容检测类型，按类别检查。
- **下载**：只给可用的附件。普通类别给属主一个 5 分钟的预签名地址（保存为文件）；严格类别给一次性凭证，用户凭它从本组件取字节，响应头使内容只被保存、不被执行。
- **生命周期**：属主命令解除引用（30 天宽限期内可恢复）、设置保留起算、擦除、保全；宽限期与最短保留都过、没有保全时清除。
- **属主核对**：只有父资源类型的属主（授权成员的定义目录说的）能申请凭证与下达命令；别的被拒并写安全审计。新业务组件上线只在自己的资源定义里声明附件类别，本组件不改动、不重新发布。

本组件从不调用属主（D68）。

**3.0.2**：一致性考试之后的修正：确认后删暂存对象；替换内容与保留起算让状态事件的版本号加一；保留起算只往后移；最长保留已过时告警（`event=attachment_retention_exceeded`）；保全与清除互斥、已清除的不能保全；上传凭证的签发写审计。be-sdk-go alpha.131。

**3.0.1**：只改代码说明（每层的“文件”写成表，装配项目的布局检查）；行为不变。

**3.0.0**（第一个重建的版本，契约 `contract/v3.0.0-alpha.1`）：上述全部；扫描端口首批只有 `off`（附件标“未扫描”）；分段上传、属主读内容自己转发、按类别与父记录集合的保全以后做。

## 部署前准备

- 数据库：PostgreSQL 16 及以上；schema `infra_attachment`，属主角色 `infra_attachment_owner`（只供迁移）与运行角色 `infra_attachment_rw`；三者由部署方建好。
- 对象存储：本组件自己的桶或前缀与只能访问它的凭据（`OBJECT_STORE_URL`、`OBJECT_STORE_BUCKET`、`OBJECT_STORE_PREFIX`、`OBJECT_STORE_ACCESS_KEY`、`OBJECT_STORE_SECRET_KEY`）。存储要遵守条件写入（`If-None-Match`），不遵守的存储本组件拒绝条件写入并告警。
- **浏览器可达的存储地址** `OBJECT_STORE_PUBLIC_URL`：预签名的上传与下载地址按它签（组件自己的请求仍走 `OBJECT_STORE_URL`）；桶要设跨源规则（允许前端的来源 `PUT` 与 `GET`）与暂存前缀 `staging/` 的过期规则。
- 消息系统（`BUS_URL`）与实现实例：本组件在流 `infra.attachment` 上发附件状态。
- 授权成员要配 `ATTACHMENT_COMPONENT=infra/attachment`（只有它能读附件类别目录）。
- `ATTACHMENT_RETENTION`：每个保留类别的最短与最长天数（`{"purchase_contract": {"min_days": 3650, "max_days": 5475}}`）；没给的类别是待确认，从不清除（年限是法规与客户合同的事，由客户在上线时给）。
- `ATTACHMENT_SCAN`：首批只能是 `off`。
- 授权：`infra.attachment.upload.confirm`、`infra.attachment.download.read` 授予所有要用附件的人（能做什么由属主签发的凭证限定）。
- 大文件的流量直达对象存储，只有严格类别经本组件转发：适合外壳（`be/go-infra`）或单跑。

## 依赖说明

- 必需：数据库、消息系统、对象存储、身份成员、授权成员（类别目录）。
- 可选：审计组件。
- 下游：父记录的属主经系统面调用本组件（不声明依赖）。

## 配置指南

全部键见 `component.yaml` 的 `configSchema`（由 SDK 生成，测试核对）。

| 键 | 说明 |
| --- | --- |
| `OBJECT_STORE_PUBLIC_URL` | 浏览器可达的存储地址（见部署前准备） |
| `ATTACHMENT_RETENTION` | 保留类别的年限 |
| `ATTACHMENT_SCAN` | 扫描端口（首批只有 `off`） |
| `OBJECT_STORE_*`（必填） | 自己的桶与前缀、凭据 |
| `PG_HOST`、`PG_DATABASE`、`PG_USER`、`PG_PASSWORD`、`PG_SCHEMA`（必填） | 自己的数据库，密码是密钥；迁移另用 `PG_OWNER_USER`、`PG_OWNER_PASSWORD` |
| `BUS_URL`（必填，密钥）、`IMPLEMENTATION_INSTANCE`（必填） | 消息系统；实现实例是安装时生成的随机身份 |
| `DEPLOYMENT_ID`、`IAM_ISSUER`、`IAM_JWKS_URL`、`AUTHZ_GRPC_URL`、`SERVICE_TLS_CERT_FILE`、`SERVICE_TLS_KEY_FILE`、`SERVICE_TLS_CA_FILE`（必填） | 受保护的用户面与 mTLS 的系统面；授权成员的地址另用来读附件类别目录 |

## 契约索引

- **系统面**：`contract/proto/be/attachment/v3/attachment.proto` 的 `AttachmentService`（`CreateUpload`、`IssueDownload`、`List`、`Detach`、`Restore`、`SetRetentionStart`、`Erase`、`Hold`、`ReleaseHold`），操作 `infra.attachment.system.manage`（任何组件可调，每个命令核对属主）。
- **用户面**：`contract/openapi.yaml`。`POST /api/attachment/uploads/confirm`（`infra.attachment.upload.confirm`）、`POST /api/attachment/downloads`（`infra.attachment.download.read`）。上传没到时确认答 `UPLOAD_INCOMPLETE`（409，可重试，凭证不被用掉）。
- **事件**：发布 `infra.attachment.changed.v1`（`contract/events/`）。

## 外壳声明

不是外壳。可以被 Go 的基础外壳（`be/go-infra`）承载。
