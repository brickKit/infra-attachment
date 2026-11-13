# infra-attachment

组件 `infra/attachment`（附件）的仓库：替父记录的属主保存附件的记录与对象，按属主申请的短期凭证让用户上传与下载，保证下载的内容就是确认时检查过的那份。用法与部署在 `BRICKKIT.md`，设计在 `docs/design.md`，代码地图在 `AGENTS.md`。

本仓库按终局设计从零开始（D59），版本从 `3.0.0` 起（A110）。设计依据：装配项目 [be-assembly-standard](https://github.com/brickKit/be-assembly-standard) 的边界卡 `docs/zh/07-components/infra.md`（`infra/attachment` 一节）与协议 12；对象的读写经线协议第 19 章的存储端口。

## 文档

| 问题 | 看哪里 |
| --- | --- |
| 它做什么、怎样部署与配置 | `BRICKKIT.md` |
| 为什么这样设计 | `docs/design.md` |
| 怎样改它 | `AGENTS.md` |
| 契约 | `contract/README.md` |
