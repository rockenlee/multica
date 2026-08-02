# 企业集成设置功能需求

> 状态：Requirements Draft v7
> 日期：2026-06-26
> 范围：Settings 信息架构、账号管理、资源、Git / 禅道 / 飞书、集成、Project 资源联动、Issue 双向同步协议、企业级权限边界与阶段验收。

## 0. 参考依据

本需求不是重新设计一个独立后台，而是在 Multica 现有产品结构上做企业集成能力的增量升级。依据包括：

- `docs/product-overview.md`：Workspace、issue、project、agent、runtime、Settings 的产品边界。
- `docs/design.md`：Multica UI 的克制、密度、灰度层级、卡片和 Settings 风格约束。
- `apps/docs/content/docs/developers/conventions.zh.mdx`：中文术语与产品文案约束。
- `apps/docs/content/docs/auth-tokens.zh.mdx`：现有 Multica PAT 的创建、展示、撤销规则。
- `apps/docs/content/docs/github-integration.zh.mdx`：现有 GitHub App 集成能力与边界。
- `apps/docs/content/docs/lark-bot-integration.zh.mdx`：现有飞书 Bot 与 agent 一对一绑定能力。
- `apps/docs/content/docs/project-resources.zh.mdx`：现有 project resource 模型。
- `packages/views/settings/components/settings-page.tsx`：现有 Settings 左侧导航和 tab 归属。
- `packages/views/settings/components/tokens-tab.tsx`、`repositories-tab.tsx`、`github-tab.tsx`、`integrations-tab.tsx`：现有设置页行为。
- `server/migrations/251_integration_module.up.sql`、`server/pkg/db/queries/integration.sql`：当前通用集成模块雏形。

### 0.1 对原有 docs 的继承关系

本需求必须继承原有 docs 中已经稳定的产品边界，不把企业集成做成另一个并行系统：

| 原有 docs / 模块 | 已有边界 | 本次增量 |
| --- | --- | --- |
| `docs/product-overview.md` | Workspace 是 issue、project、agent、成员和 Settings 的隔离边界。 | GitLab、禅道、飞书连接都归属于 workspace；成员账号、项目绑定和同步状态不能越过 workspace。 |
| `apps/docs/content/docs/auth-tokens.zh.mdx` | Multica PAT 是用户级 token；创建后只展示一次完整明文；用于 CLI / API。 | “账号管理”不是移除 PAT，而是在原 PAT 页上增加外部 CLI 账号。中文主入口叫“账号管理”，英文主入口叫“Access Tokens”。 |
| `apps/docs/content/docs/github-integration.zh.mdx` | GitHub App 是工作区级集成，负责 PR 关联、PR 侧栏、issue 自动推进等。 | GitHub 原能力保留在新的 Git 页；GitLab 以相同设置范式补齐 MR / issue / 归因 / 同步状态。 |
| `apps/docs/content/docs/lark-bot-integration.zh.mdx` | 飞书 Bot 与 Multica agent 一对一绑定，成员完成身份绑定后可在飞书中交互。 | 飞书设置页保留 Bot 能力，同时增加飞书 App、IM、云盘、Wiki 和资源入口。中文 UI 一律称“飞书”。 |
| `apps/docs/content/docs/project-resources.zh.mdx` | Project resource 通过 `resource_type + resource_ref` 挂载项目上下文。 | 不重做项目模型；扩展 GitLab 仓库、飞书云盘 / Wiki、禅道项目 / 产品为新的资源类型。 |
| `docs/design.md` | Settings 是高密度配置页，保持窄内容区、轻量卡片、开关行和小 badge。 | 本次页面必须贴着原设置页改，不做全新后台、营销页或 Figma 式大重构。 |

因此，第一阶段目标是把“用户从哪里配置”和“配置如何进入 Multica 数据模型”做清楚；真正的同步 worker、webhook、轮询和双向 issue 幂等协议属于后续阶段，但必须按本文协议预留数据和 UI 入口。

### 0.1.1 原 docs 事实矩阵

以下事实来自现有产品 docs 和当前源码，是后续产品、设计、开发评审时的硬约束：

| 事实来源 | 已验证事实 | 对本需求的约束 |
| --- | --- | --- |
| `apps/docs/content/docs/auth-tokens.zh.mdx` | PAT 是用户级 API token，`mul_` 前缀，创建后完整内容只展示一次，数据库只存哈希；daemon token 是 `mdt_` 前缀并绑定单 workspace。 | “账号管理”必须保留 PAT 的一次性展示、撤销和列表能力；外部账号不能复用 PAT 的明文展示模型；daemon token 不进入普通账号管理。 |
| `apps/docs/content/docs/github-integration.zh.mdx` | GitHub 集成是 GitHub App + webhook，当前只镜像 PR metadata，权限是 PR / Metadata 只读，不写 commit、评论或 status check。 | Git 页面必须保留 GitHub 的窄边界；GitLab 能力新增时不能暗示 GitHub 会变成双向 issue 写入。 |
| `apps/docs/content/docs/lark-bot-integration.zh.mdx` | 飞书 Bot 是 agent 级一对一绑定；成员首次使用需要绑定飞书身份；Bot 只处理私聊或群内 @，`/issue` 可创建 Multica issue。 | Workspace / 飞书设置页只能汇总和配置飞书能力，不能把现有 agent-Bot 绑定改成一个 workspace 级共享 Bot。 |
| `apps/docs/content/docs/project-resources.zh.mdx` | Project resource 是项目上的类型化指针，当前 `github_repo` 支持 worktree，`local_directory` 支持桌面端本地目录并受 daemon 约束。 | 新增 `gitlab_repo`、`feishu_drive`、`feishu_wiki`、`zentao_project`、`zentao_product` 时，仍沿用同一资源模型和 Project Resources 入口。 |
| `docs/product-overview.md` | Workspace 是隔离边界；Issue 是核心工作对象；Project 是 issue 的高层容器；agent 和成员都可作为 actor。 | 外部系统只能映射到 workspace、project、issue、comment、metadata、activity 这些既有对象，不新增平行 ticket 系统。 |

### 0.1.2 原 docs 约束的需求化解释

原有 docs 里的能力不是本次集成的“旧版本参考”，而是必须继续成立的产品事实：

- `workspaces.zh.mdx` 强调 workspace 切换后内容完全隔离。因此 GitLab group、禅道项目、飞书企业空间都只能作为某个 Multica workspace 下的外部连接配置出现，不能跨 workspace 复用同步结果。
- `issues.zh.mdx` 强调 issue 分配给 agent 会自动开工。因此外部 issue 入站时默认不能自动分配给 agent，否则同步本身会触发实际执行；入站镜像默认应该落到未分配或成员分配状态。
- `projects.zh.mdx` 强调 project 是 issue 容器，不是权限边界。因此 project 外部绑定决定上下文和同步路由，但不能替代 workspace owner / admin 的连接管理权限。
- `project-resources.zh.mdx` 强调资源会进入 agent 的项目上下文。因此飞书云盘 / Wiki、禅道项目说明、GitLab 仓库一旦作为 project resource 绑定，就应被视为 agent 可感知上下文，需要权限和可见性控制。
- `github-integration.zh.mdx` 强调 GitHub 集成是编号驱动的 PR 镜像，不写 GitHub。新增 GitLab issue 双向同步不能改变 GitHub 的现有语义，除非后续另立 GitHub issue 同步需求。
- `lark-bot-integration.zh.mdx` 强调飞书 Bot 是 agent 级一对一绑定。Workspace / 飞书页可以汇总 Bot 状态，但不能把现有 Bot 能力改成一个 workspace 共享 Bot。

这些解释用于解决评审中最容易混淆的三件事：同步不是自动派活，资源绑定不是权限授权，集成不是全量配置后台。

### 0.2 当前页面实现快照

截至本轮页面验证，现有实现已经能够表达目标信息架构，并且整体仍在原 Settings 框架内增量修改：左侧导航、窄内容区、卡片、开关行、badge、表单密度都沿用了现有 Multica 风格。但它还不能被理解为完整同步系统已经上线：

| 区域 | 当前页面状态 | 需求判断 |
| --- | --- | --- |
| 我的账号 / 账号管理 | 已从单一 PAT 入口扩展为账号管理语义，页面上保留 Multica PAT，并露出 GitLab、禅道、飞书账号表单。 | 符合方向；下一步需要把多账号、默认账号、测试连接、权限范围、过期时间做成真实模型。 |
| Workspace / 资源 | 保留代码仓库配置，并增加飞书云盘 / Wiki、禅道项目 / 产品的说明入口。 | 符合方向；后续要从这里清晰跳转到 Project Resources，让资源列表、项目绑定、同步健康打通，而不是只停留在说明文案。 |
| Workspace / Git | GitHub 原 tab 继续存在，页面内并列展示 GitHub 与 GitLab 能力。 | 符合“在原 GitHub 能力上扩展成 Git 页”的要求；GitHub App 行为不能回退。 |
| Workspace / 禅道 | 已有独立设置页，能表达禅道连接、总开关、账号引导和连接健康。 | 符合 workspace 级配置入口；项目 / 产品绑定必须放在 Project Resources，真实禅道 API / CLI 健康检查和同步 worker 仍需补齐。 |
| Workspace / 飞书 | 已有独立设置页，并复用现有飞书 Bot 设置，同时表达 App、IM、云盘 / Wiki 权限能力。 | 符合“飞书就是飞书”的中文命名要求；云盘 / Wiki 资源绑定必须放在 Project Resources，Bot 与 agent 一对一的旧能力不能被资源同步覆盖。 |
| Workspace / 集成 | 已成为总览页，展示 GitLab、禅道、飞书连接状态、账号数量、项目绑定和最近事件。 | 符合“集成 = 总览”的定位；不得继续把所有 token 输入和原始 JSON 表单塞回这里。 |
| Project 详情 / Resources | 已支持 GitLab 仓库、飞书云盘、飞书 Wiki、禅道项目、禅道产品等资源类型，并能看到同步绑定摘要。 | 符合项目属性联动方向；后续需要把 issue 创建时的出站路由和 `sync_out` 显式动作接入。 |

文案和 UI 必须区分“配置入口已经存在”和“同步能力已经生效”。在同步 worker、webhook、幂等键、失败审计完成前，页面不应承诺“已自动同步所有事项”。

### 0.3 本轮需求收敛

结合页面验证和原有 docs，本次功能不是“重新设计一个企业集成后台”，而是把企业集成能力接入 Multica 已有产品模型：

1. Settings 只调整信息架构和页面归属，不重做交互框架。
2. 账号管理继续承接 Multica PAT，同时扩展 GitLab / 禅道 / 飞书个人账号。
3. Workspace 连接负责企业级 provider 开关、服务账号、健康检查和项目绑定。
4. Project resource 继续作为 agent 执行上下文入口，不新增一套平行资源系统。
5. Issue 仍是唯一工作对象；外部事项进入 Multica 后必须落到 issue、comment、metadata 或 activity，不引入另一套 ticket 对象。
6. 集成只承担健康总览、最近事件、成员映射摘要和快捷跳转，不承载所有配置细节。
7. 双向同步是后续能力，但第一阶段页面和数据模型必须为 `source_*` metadata、`sync_out` 显式触发、幂等和审计预留位置。

### 0.4 当前版本的边界说明

需求文档需要明确区分四种状态，避免页面评审时误判：

| 状态 | 含义 | 示例 |
| --- | --- | --- |
| 已有原能力 | 产品原本已经可用，必须保留。 | PAT 创建 / 撤销、GitHub PR 关联、飞书 Bot 绑定 agent、Project Resources。 |
| 已做页面骨架 | UI 已经表达入口和字段，但能力可能还只是配置保存。 | 账号管理中的外部账号表单、禅道 / 飞书设置页、集成卡片。 |
| 已做数据底座 | DB / API 可以保存连接、账号、绑定或同步事件，但还没有完整 worker。 | `integration_connection`、`integration_user_account`、`integration_project_binding`、`integration_sync_event`。 |
| 待实现同步闭环 | 需要 webhook / polling / worker / 外部 API 调用 / 幂等测试完成后才能宣称上线。 | GitLab issue 入站、禅道 Bug 入站、Multica issue 反向创建、飞书云盘全文同步。 |

验收和对外文案必须使用这个边界。配置页可以写“已配置”“同步已开启”“等待同步”“最近事件”，不能在 worker 未完成前写“已自动同步企业事项”。

## 1. 背景

Multica 的 Settings 已经有稳定的信息架构：

- 我的账号：Profile、Preferences、Notifications、Tokens。
- 工作区：General、Repositories、GitHub、Integrations、Labs、Members。

现有产品事实也已经清楚：

- Multica 是工作区协作系统。Workspace 是所有 issue、agent、project、skill、成员和设置的隔离边界。
- Issue 是工作单元。人和 agent 都可以创建、评论、订阅和处理 issue。
- Project 是 issue 的上层归属。现有 project resource 模型已经允许项目挂载 GitHub 仓库、本地目录等资源。
- Settings 不是营销页，而是“配置影响 agent 实际执行上下文”的位置。
- GitHub 当前是工作区级 GitHub App 集成，做 PR 镜像、issue 编号关联和 PR merge 后状态推进。
- 飞书当前是 agent 级 Bot 能力：一个飞书 Bot 绑定一个 Multica agent，成员完成身份绑定后可以在飞书中对话或创建 Multica issue。
- Multica PAT 当前是用户级 API token，用于 CLI、脚本和直接 API 调用，完整 token 只在创建时展示一次。

这次企业集成需求的核心是：把 GitLab、禅道、飞书纳入 Multica 的原生 integration module，而不是外挂同步脚本或一个完全独立的后台页面。

## 2. 产品定位

### 2.1 一句话目标

让企业工作区能够在 Multica 内统一连接 Git、禅道和飞书，并把外部 issue、代码资源、知识资源和成员账号映射到 Multica 的项目、issue 和 agent 执行上下文中。

### 2.2 角色定位

- GitLab、禅道、飞书仍然是各自业务域的事实源。
- Multica 是统一协作层和 AI 执行层。
- 集成模块负责连接、映射、同步、审计和健康观测。
- Agent 使用这些集成结果作为上下文执行任务，但默认不因为同步而自动开工。

### 2.3 不是外挂同步服务

第一版可以有后台 worker、队列或定时任务，但产品形态必须是 Multica 内置集成模块：

- 配置入口在 Multica Settings。
- 账号入口在 Multica 我的账号。
- 项目绑定入口在 Multica project 详情和 Workspace 设置。
- 同步状态、错误和审计在 Multica 集成可见。
- 不能要求用户理解或手工维护一个外部 sidecar 才能完成日常配置。

若部署上需要独立进程，仍应作为 Multica server 端的内部 worker 管理，并读取 Multica 数据库中的连接、账号、绑定和同步状态。

### 2.4 第一版 MVP

第一版不是把 GitLab、禅道、飞书全部深度替换成 Multica，而是让企业管理员和成员能完成一条闭环配置路径：

1. Workspace owner / admin 在 Workspace 设置中开启 Git、禅道、飞书连接，并保存服务地址、服务账号或 App 配置。
2. 成员在“我的账号 / 账号管理”中添加自己的 GitLab、禅道、飞书账号，能看到是否已配置、是否启用、是否过期。
3. 管理员先在 Workspace 设置中开通外部连接，再在 Project 详情 / Resources 中把外部仓库、禅道项目 / 产品、飞书云盘 / Wiki 绑定到 Multica project。
4. 集成能看到每个 provider 的连接状态、账号数量、项目绑定数量、最近同步事件和最近错误。
5. Issue 详情中能识别该 issue 是否已绑定外部来源；对未绑定来源的 issue 不展示反向创建动作。
6. 对已绑定来源的 project，用户必须显式触发 `sync_out` 后才会进入出站同步队列。

第一版可先做到“配置 + 绑定 + 队列请求 + 审计事件”。外部系统真实创建、评论反写、飞书云盘全文索引可以作为后续 worker 能力逐步接入，但 UI 和文档必须明确状态。

### 2.5 非目标

以下事项不是本次第一版目标：

- 不把 Multica 做成 GitLab、禅道、飞书的完整替代品；外部系统仍是各自事实源。
- 不把 GitLab、禅道、飞书的整个企业空间无差别导入 Multica。
- 不因为外部事项同步进来就自动分配 agent、自动启动 task 或自动加入 squad。
- 不在集成直接输入所有 token、原始 JSON、项目绑定细节；集成只做总览。
- 不把飞书 Bot 绑定能力改造成 workspace 级单 Bot；现有 agent-Bot 一对一模型必须保留。
- 不把 GitHub 的 PR 只读集成升级成隐含的 GitHub issue 双向同步，除非后续另立需求。
- 不在凭据、日志、toast、同步事件、系统评论中展示 token 明文。

## 3. 术语和命名

| 场景 | 中文 UI 主标签 | 英文 UI 主标签 | 说明 |
| --- | --- | --- | --- |
| 我的账号里的 token 和外部 CLI 账号 | 账号管理 | Access Tokens | 中文界面不再叫 API Token。 |
| 工作区的代码、文档、项目资源 | 资源 | Resources | 原“代码仓库”升级为更广义资源入口。 |
| GitHub / GitLab 设置 | Git | Git | GitHub App 与 GitLab App 并列。 |
| 禅道设置 | 禅道 | ZenTao | 工作区级项目管理系统连接。 |
| 飞书设置 | 飞书 | Lark | 中文主标签只使用“飞书”。英文界面才使用 Lark。 |
| 企业集成总览 | 集成 | Integrations | 只做总览、健康和跳转，不承载全部配置。 |

内部 provider key 可以继续使用 `github`、`gitlab`、`zentao`、`feishu`。用户界面按上表展示。

## 4. 设计原则

1. 在现有 Settings 上增量调整，不重做导航、布局和视觉体系。
2. 工作区连接、个人账号、项目绑定、资源、同步事件必须分层。
3. 集成是总览，不是所有表单的集合。
4. 外部凭据不展示明文，不写入日志、toast、同步评论或错误详情。
5. 外部系统仍是事实源。Multica 不默认覆盖外部状态，不默认反写评论，不默认派发 agent。
6. 反向创建必须显式触发，且只能发生在已绑定外部来源的 project 内。
7. 同步必须幂等，避免重复 issue、重复评论和回声同步。
8. GitHub 现有能力、飞书 Bot 现有能力、代码仓库和 local directory 现有能力不能回退。

### 4.1 从原有 docs 派生出的不可回退约束

本次企业集成不是平铺新增三个外部系统，而是把外部系统接进 Multica 已有协作模型。以下行为必须保留：

- Workspace 仍然是所有配置和数据的坐标系。任何连接、账号映射、同步事件、资源绑定都必须挂在某个 Multica workspace 下。
- Issue 仍然是工作单元。外部事项进入 Multica 后必须成为 issue 或 issue 附属信息，不能创建另一套平行任务对象。
- Project 仍然是 issue 的高层容器。外部项目、产品、仓库、云盘资源只能通过 project 资源或 project 绑定进入项目上下文。
- Agent 仍然通过 issue、评论、chat、autopilot 等既有触发方式工作。同步外部事项本身不自动分配 agent，也不自动生成 agent task。
- Project resource 仍然采用 `resource_type + resource_ref` 的扩展形态。新增 GitLab、飞书、禅道资源类型时，不新增第二套资源表单和第二套资源 CRUD。
- GitHub 集成仍然是只读 PR / metadata 类能力，保留 issue 编号关联和 PR merge 后状态推进，不因为 GitLab 加入而改坏原有 GitHub 行为。
- 飞书 Bot 仍然是“一个 Bot 绑定一个 Multica agent”的能力，成员身份绑定和 `/issue` 创建 Multica issue 的路径继续有效。
- Multica PAT 仍然是用户级 API token，创建后只展示一次完整明文。账号管理不能把 PAT 降级成普通外部账号的一行。

## 5. 信息架构

### 5.1 我的账号

| 当前入口 | 目标入口 | 职责 |
| --- | --- | --- |
| Tokens | 账号管理 | 保留 Multica PAT；新增 GitLab CLI 账号、禅道 CLI 账号、飞书 CLI 账号。 |

账号管理是成员自己的入口，不管理工作区级连接开关。

### 5.2 工作区

| 当前入口 | 目标入口 | 职责 |
| --- | --- | --- |
| Repositories | 资源 | 保留现有 `workspace.repos` 代码仓库白名单；新增飞书云盘 / Wiki、禅道项目 / 产品资源入口。 |
| GitHub | Git | GitHub App 与 GitLab App 的工作区级连接、开关和功能项。旧 `tab=github` 继续兼容。 |
| Integrations | 集成 | 集成健康总览、成员映射摘要、快捷入口。 |
| 新增 | 禅道 | 禅道服务连接、同步总开关、账号引导和连接健康；项目 / 产品绑定从 Project 详情 / Resources 配置。 |
| 新增 | 飞书 | 飞书总开关、Bot / App 状态、IM 能力和连接健康；云盘 / Wiki 资源绑定从 Project 详情 / Resources 配置。 |

### 5.3 Project 详情

Project 详情页需要和企业集成联动：

- 项目资源区支持绑定 GitLab 仓库、飞书云盘、飞书 Wiki、禅道项目、禅道产品。
- 项目属性或资源区能看到该 project 已绑定的外部来源。
- 对已绑定外部来源的 project，创建 issue 时可以选择是否显式触发 `sync_out`。
- 对未绑定外部来源的 project，不展示反向创建动作，或展示为不可用并说明原因。

### 5.4 页面到对象的关系

不同设置页解决不同层级的问题，不能互相混用：

| 用户问题 | 应去页面 | 落到对象 |
| --- | --- | --- |
| 我自己的 GitLab / 禅道 / 飞书账号在哪里加？ | 我的账号 / 账号管理 | `integration_user_account` |
| 这个 workspace 是否允许连接 GitLab / 禅道 / 飞书？ | Workspace / Git、禅道、飞书 | `integration_connection` |
| 这个 project 关联哪个外部项目、仓库、云盘或 Wiki？ | Project 详情 / Resources | `project_resource`、`integration_project_binding` |
| 这个外部连接最近同步是否正常？ | Workspace / 集成 | `integration_sync_event`、连接健康字段 |
| 一个新 Multica issue 要不要反向创建到外部系统？ | Issue 创建 / 编辑流程中的显式动作 | issue metadata、project binding、同步事件 |

这张关系表是后续实现和验收的依据。若某个页面开始承载不属于自己的对象，例如在集成直接编辑个人 token，应该回退到对应职责页面。

### 5.5 Settings 入口迁移矩阵

本次调整必须保留原入口的核心能力，并通过兼容 tab 或清晰跳转降低迁移成本：

| 原入口 | 目标入口 | 必须保留 | 新增能力 | 兼容要求 |
| --- | --- | --- | --- | --- |
| My Account / API Tokens | My Account / 账号管理 | Multica PAT 创建、一次性展示、列表、撤销。 | GitLab CLI 账号、禅道 CLI 账号、飞书 CLI 账号，多账号和测试连接。 | 原 PAT 数据和 API 不变；中文页面标题不再用 API Token 作为一级入口。 |
| Workspace / Repositories | Workspace / 资源 | `workspace.repos`、GitHub repo 白名单、agent repo context。 | GitLab 仓库、飞书云盘 / Wiki、禅道项目 / 产品的资源入口。 | 不能影响 daemon checkout、worktree、local directory 逻辑。 |
| Workspace / GitHub | Workspace / Git | GitHub App 连接、主开关、PR 侧栏、auto-link、Co-authored-by、merge 后推进状态。 | GitLab App / 企业实例连接、MR 侧栏、GitLab issue / MR 关联。 | `?tab=github` 可以继续进入 Git 页；GitHub 原功能不降级。 |
| Workspace / Integrations | Workspace / 集成 | 飞书 Bot 列表和原集成状态入口的可发现性。 | Git / 禅道 / 飞书健康总览、最近同步事件、成员映射摘要。 | 不能把集成变成所有 provider 的大表单集合。 |
| Agent / Integrations | Agent / 集成 + Workspace / 飞书 | agent 级飞书 Bot 一对一绑定。 | Workspace 飞书设置页汇总 Bot、IM、云盘、Wiki 能力。 | 不能把 Bot 绑定能力误迁移成 workspace 级单一 Bot。 |
| Project / Resources | Project / Resources + 同步绑定配置 | `github_repo`、`local_directory` 资源和执行上下文注入。 | GitLab repo、飞书云盘 / Wiki、禅道项目 / 产品、同步绑定摘要与当前项目的同步路由。 | Project 页只配置当前项目相关绑定，不替代 workspace 连接设置。 |

迁移后的页面结构必须能回答三个问题：

- 个人凭据在哪里：我的账号 / 账号管理。
- 企业连接在哪里：Workspace / Git、禅道、飞书。
- 项目上下文和同步路由在哪里：Project / Resources。

### 5.6 端到端配置路径

页面拆分之后，用户仍需要一条清楚的配置路径。第一版必须支持以下端到端路径：

#### 5.6.1 管理员开通企业连接

1. Workspace owner / admin 进入 Workspace / Git、禅道或飞书。
2. 打开 provider 总开关。
3. 填写企业级连接信息，例如 GitLab 实例地址、禅道 V2 / CLI base URL、飞书开放平台 App / Bot 配置。
4. 保存后页面显示 `configured`，如果健康检查通过再显示 `ready`。
5. 集成出现该 provider 卡片、连接状态和快捷入口。

此路径只完成 workspace 级连接，不等于任何成员已经绑定个人账号，也不等于 project 已经同步。

#### 5.6.2 成员绑定个人账号

1. 成员进入 我的账号 / 账号管理。
2. 页面只展示当前 workspace 已开通 provider 的账号入口。
3. 成员按引导添加 GitLab、禅道或飞书账号。
4. 保存后只显示遮蔽态、权限范围、过期时间和启用状态。
5. 集成成员映射摘要显示该成员已配置对应账号。

此路径解决身份映射和权限归因，不自动扩大 workspace 同步范围。

#### 5.6.3 管理员绑定项目资源与同步路由

1. 管理员进入具体 Project 的详情页 / Resources。
2. 绑定 GitLab 仓库、飞书云盘 / Wiki、禅道项目 / 产品等资源。
3. 在同一 Project Resources 区配置该 project 的入站、出站、issue 同步、知识同步等开关。
4. 如需批量查看多个 provider 状态，进入 Workspace / 集成查看摘要和快捷入口。
5. Project 详情展示资源和同步绑定摘要；集成展示绑定数量和最近事件。

此路径把外部资源接入 project 上下文。只有同时配置了 `outbound_enabled` 与 `issue_sync_enabled` 的绑定，才可能支持 Multica issue 反向创建。

#### 5.6.4 成员在 issue 上显式触发同步

1. 成员在一个已归属 project 的 Multica issue 上查看外部同步区。
2. 系统读取该 project 的同步绑定，判断是否有可用出站目标。
3. 用户点击“同步到外部”或等价显式动作。
4. API 写入 `sync_out` metadata 和同步事件，状态变为 `queued`。
5. 后续 worker 创建外部 issue 成功后，回填 `source_*` metadata 和外部链接。

此路径是受控出站同步。选择 project、保存 issue 或绑定资源本身都不能自动创建外部事项。

#### 5.6.5 Agent 使用集成上下文

当 agent 处理某个 project 内的 issue 时，它能看到该 project 的资源上下文，但这和同步权限是两件事：

- GitLab / GitHub 仓库资源告诉 agent 应该参考或 checkout 哪些代码。
- 飞书云盘 / Wiki 资源告诉 agent 可使用哪些知识来源。
- 禅道项目 / 产品资源告诉 agent 该项目在外部项目管理系统中的业务上下文。
- 是否能反向创建外部 issue，仍由 workspace 连接、成员账号、project 同步绑定和显式 `sync_out` 决定。

因此，Project Resources 是 agent 上下文入口，不是外部系统写权限入口。

## 6. 页面需求

### 6.1 我的账号 / 账号管理

页面主标题：

- 中文：`账号管理`
- 英文：`Access Tokens`

必须保留 Multica PAT 能力。PAT 是账号管理中的一个分组，不再作为整个页面的主入口名称：

- 创建 PAT。
- 设置名称和有效期。
- 创建后只展示一次完整 token。
- 列表展示名称、前缀、创建时间、最近使用、过期时间。
- 撤销 PAT。

新增外部账号管理：

- 支持多个 GitLab CLI 账号、禅道 CLI 账号、飞书 CLI 账号。
- 每个账号展示：账号名、平台、所属工作区连接、权限范围、状态、过期时间、最近使用时间、最近错误。
- 凭据只显示遮蔽态，例如“已配置”或 token 前缀，不显示完整明文。
- 支持添加、更新、停用、删除、测试连接。
- 支持开启或关闭该账号参与同步。

添加账号必须提供引导：

- GitLab：说明需要在 GitLab 里创建 Personal Access Token，并提示最小权限范围，例如 `read_api`、`read_repository`，需要反向创建时再增加 `api` 或对应写权限。
- 禅道：说明需要使用企业提供的禅道服务地址、账号和 token / 密钥。若企业要求走禅道 CLI 兼容入口，应在连接页明确填写 CLI 使用的 base URL，而不是假设 localhost 或旧端口。
- 飞书：说明由企业管理员提供开放平台应用、CLI profile 或成员授权方式。中文界面主标签写“飞书”。

账号引导不应该要求普通成员理解服务账号部署细节。成员只需要知道：

- 当前 workspace 是否已经由管理员开通 Git / 禅道 / 飞书连接。
- 自己需要在哪个外部系统生成或授权账号。
- 最小权限是什么，哪些权限只有反向创建或评论反写时才需要。
- token 只保存密文，保存后只能看到“已配置”状态。

数据模型差距：

- 当前 `integration_user_account` 以 `(connection_id, user_id)` 唯一，只能表达“每个连接每个用户一个账号”。
- 需求要求同一用户同一 provider 或同一连接下可配置多个账号。
- 实现需要新增 `account_name` / `account_key` / `is_default` 等字段，并调整唯一约束，例如 `(connection_id, user_id, account_key)`。

#### 6.1.1 账号添加引导

账号管理页需要提供“管理员已开通连接”和“成员添加个人账号”两层引导，避免用户不知道 token 从哪里来。

| Provider | 管理员前置动作 | 成员添加账号时看到的引导 | 最小权限建议 |
| --- | --- | --- | --- |
| GitLab | 在 Workspace / Git 中配置 GitLab 企业实例或 App，并决定允许同步的 group / project。 | 去 GitLab 用户设置创建 PAT，填写实例地址对应的账号名、token、过期时间和 scopes。 | 只读同步：`read_api`、`read_repository`；需要反向创建 issue / 评论时增加写入相关权限。 |
| 禅道 | 在 Workspace / 禅道中配置企业禅道地址、CLI/API base URL、服务账号或 `multica-sync-bot`。 | 填写禅道用户名或账号标识、CLI token / API token、过期时间。页面必须提示使用企业配置的服务地址，不假设 `127.0.0.1`。 | 只读同步：项目 / 产品 / Bug / 任务 / 需求读取；反向创建时需要对应创建和评论权限。 |
| 飞书 | 在 Workspace / 飞书中配置飞书 App / Bot 能力，并确认云盘 / Wiki 权限范围。 | 通过授权或 CLI profile 绑定自己的飞书身份；若是 token 形式，只展示遮蔽态。 | IM / Bot 身份绑定最小化；云盘 / Wiki 同步需要对应资源读取权限；反写或发送消息需要额外授权。 |

页面行为要求：

- 如果 workspace 没有对应 provider 连接，账号管理页只显示“等待管理员开通”的空态和跳转，不展示无效 token 表单。
- 如果服务端未启用凭据加密，token 输入框禁用，并显示“凭据存储未启用”。
- 保存后只显示“已配置”或 token 前缀，不展示明文。
- 测试连接失败时只显示原因类别，例如权限不足、token 过期、base URL 不可达，不显示原始 token 或完整请求头。
- 多账号场景下必须能标记默认账号；默认账号的作用域至少要明确是 provider 级、connection 级，还是 project binding 级。

#### 6.1.2 管理员账号与成员账号的职责

企业同步需要同时支持服务账号和成员账号，但两者不能混在同一层解释：

| 账号类型 | 配置位置 | 使用场景 | 归因方式 |
| --- | --- | --- | --- |
| 服务账号 / sync bot | Workspace / Git、禅道、飞书 | 全工作区入站同步、批量扫描、Webhook 回调、健康检查。 | 同步事件记录为系统或服务账号；不能伪装成普通成员。 |
| 成员个人账号 | 我的账号 / 账号管理 | 个人相关事项同步、权限校验、反向创建时按个人身份写入。 | 外部系统可归因到成员本人；Multica 中仍记录触发者和账号 id。 |
| Agent 执行身份 | Agent / Runtime / Issue | agent 处理 Multica issue，本身不直接等于外部账号。 | agent 的评论和状态更新仍按 Multica actor 模型记录。 |

默认策略：入站批量同步优先使用服务账号；需要以成员身份执行的反向创建、评论反写或外部审计动作，才使用成员账号。若成员没有配置账号，UI 应提示补齐，而不是静默退化成服务账号代写。

### 6.2 工作区 / 资源

资源是工作区资源入口，不只是代码仓库。

代码仓库必须保持兼容：

- `workspace.repos` JSONB 行为不破坏。
- 守护进程、worktree、仓库白名单和现有 repo checkout 逻辑继续可用。
- 现有 RepositoriesTab 的添加、编辑、删除、保存行为可以作为“代码仓库”分组保留。

新增资源分组：

- 代码资源：GitHub 仓库、GitLab 仓库、本地目录。
- 飞书资源：云盘文件夹、Wiki 空间、文档集合。
- 禅道资源：项目、产品、迭代，及其 Bug / 任务 / 需求范围。

资源模型沿用 `resource_type + resource_ref`：

| resource_type | resource_ref 要点 |
| --- | --- |
| `github_repo` | URL、默认分支 hint，保持现有行为。 |
| `gitlab_repo` | URL、默认分支 hint、可选 project path / project id。 |
| `local_directory` | 本机路径、daemon id、label，保持现有行为。 |
| `feishu_drive` | 云盘 URL、folder token / node token、label。 |
| `feishu_wiki` | Wiki URL、space id / node token、label。 |
| `zentao_project` | 禅道项目 id / key / URL、同步类型范围。 |
| `zentao_product` | 禅道产品 id / key / URL、同步类型范围。 |

每条资源需要展示连接来源、同步状态、最近同步、最近错误、绑定到哪些 project。

### 6.3 工作区 / Git

Git 页面是 GitHub 与 GitLab 的统一入口。

GitHub 部分必须保留现有能力：

- 主开关。
- GitHub App 连接 / 断开。
- PR 侧栏。
- 自动关联 issue。
- Co-authored-by。
- PR merge 后推进 issue 状态。
- 代码仓库快捷入口。

GitLab 部分尽量与 GitHub 对齐：

- GitLab App、OAuth 或企业实例连接入口。
- 主开关。
- 连接状态：实例地址、授权账号 / group、已连接项目数。
- 功能开关：MR 侧栏、自动关联 issue、提交归因、MR merge 后状态推进。
- 同步健康：最近 webhook / polling 时间、最近错误。
- 资源快捷入口。

Git 页面不管理成员个人 GitLab CLI token。成员账号在“账号管理”里配置。

GitHub 与 GitLab 在视觉和信息结构上尽量一致，但能力边界不能强行一致：

| 能力 | GitHub 第一版基线 | GitLab 目标能力 | 说明 |
| --- | --- | --- | --- |
| 工作区连接 | GitHub App installation。 | GitLab App / OAuth / 企业 PAT 连接方式待产品确认。 | 页面可以并列展示，但后端协议可不同。 |
| PR / MR 侧栏 | 已有 Pull requests 侧栏。 | 新增 Merge requests 侧栏。 | 两者都挂在 issue 详情侧栏，不新建外部工单页。 |
| 自动关联 | 扫 PR 分支 / 标题 / 正文里的 issue 编号。 | 扫 MR 分支 / 标题 / 正文里的 issue 编号，并可扩展 GitLab issue 关联。 | 仍以 workspace issue prefix 为边界。 |
| 状态推进 | PR merged 后推进 issue 到 done，cancelled 不覆盖。 | MR merged 后可采用同样默认策略。 | 后续需要 workspace 级状态映射。 |
| 外部写入 | GitHub 当前不写 commit、评论或 status check。 | GitLab 反向创建 issue 必须走显式 `sync_out`。 | 不能因为 GitLab 加入而暗示 GitHub 也会反写。 |

GitLab 不应该被做成 GitHub 的简单复制。需求上追求一致体验，但实现上允许保留不同 provider 的授权模式、权限模型和 webhook 事件形态。

### 6.4 工作区 / 禅道

禅道页面是工作区级项目管理系统入口。

页面包含：

- 总开关。
- 连接状态：服务地址、连接名、服务账号状态、最近同步时间、最近错误。
- 禅道 CLI / API 连接引导：明确服务地址、账号、token / 密钥来源。
- 项目 / 产品绑定列表。
- 绑定新增 / 编辑：选择 Multica project，选择禅道项目 / 产品，选择同步类型。
- 同步类型开关：Bug、任务、需求、评论。
- 入站同步开关：禅道 -> Multica。
- 反向创建开关：Multica -> 禅道，默认关闭。
- 状态映射展示：禅道状态如何映射到 Multica issue 状态。

禅道连接必须支持企业管理员配置：

- 服务账号模式：由管理员创建 `multica-sync-bot` 或等价账号，用于工作区级同步。
- 成员账号模式：成员在“账号管理”里绑定自己的禅道 CLI / API 账号，用于个人范围内的同步或审计归因。
- CLI 兼容入口：如果企业通过禅道 CLI 访问新版服务，base URL 和端口必须作为连接配置保存，不能硬编码为 `127.0.0.1` 或旧端口。

### 6.5 工作区 / 飞书

飞书页面是工作区级飞书能力入口。

页面包含：

- 总开关。
- Bot / App 连接状态。
- 已连接 Bot 列表：Bot 名称、绑定 agent、状态、最近消息、管理入口。
- 云盘 / Wiki 同步设置。
- IM 能力设置：是否允许从飞书创建 Multica issue，是否允许 Bot 回复，是否允许群内 @。
- 飞书资源快捷入口：跳转到“资源”的飞书资源分组。
- 成员绑定状态：成员是否已绑定飞书身份。

必须保留现有飞书 Bot 能力：

- 每个 Bot 与一个 Multica agent 一对一绑定。
- 成员需要完成飞书身份绑定后才能使用 Bot。
- `/issue` 从飞书创建 Multica issue 的能力继续存在。
- Bot 只处理私聊或群里 @ 它的消息，不默认监听整个群。
- 断开连接需要 owner / admin 权限，历史安装记录保留用于审计。

新增云盘 / Wiki 同步能力：

- 支持选择云盘文件夹、Wiki 空间或文档集合作为资源。
- 支持按 project 绑定资源。
- 同步后作为 agent 执行上下文或检索上下文，不默认改写飞书原文。
- 云盘 / Wiki 资源同步不能替代 Bot 绑定；一个是知识资源，一个是交互入口。

### 6.6 工作区 / 集成

集成是企业集成总入口和健康总览。

必须展示：

- Git、禅道、飞书三类集成的启用状态。
- 连接状态：已连接 / 未连接 / 配置不完整 / 错误。
- 同步健康：最近成功、最近失败、失败原因。
- 最近同步事件。
- 快捷入口：去 Git、禅道、飞书、资源、账号管理。

成员映射摘要：

- 成员。
- GitLab 账号列表。
- 禅道账号列表。
- 飞书账号 / 可用 Bot。
- 同步状态。
- 最近同步时间。

集成不应该直接提供大段 token 输入、原始 JSON `external_ref` 编辑或项目绑定表单。这些细节在对应设置页完成。

### 6.7 Project 详情 / 资源与同步绑定

Project 详情页不是新的 workspace 设置页，它只展示和当前 project 直接相关的内容：

- Resources 区继续展示项目资源，包含 GitHub / GitLab 仓库、本地目录、飞书云盘、飞书 Wiki、禅道项目、禅道产品。
- 同步绑定摘要展示当前 project 已经绑定了哪些 workspace 级连接，以及每个绑定的入站、出站、issue 同步、知识同步开关。
- 如果绑定来自 Git、禅道或飞书设置页，Project 详情应展示结果和快捷入口，不要求用户在 project 页重新理解 workspace 连接配置。
- 没有同步绑定时，Project 详情只提示去 Git、禅道或飞书设置中配置，不展示无效的出站按钮。
- 有多个绑定时，需要展示 provider、连接名、外部引用摘要和路由含义，不能只展示内部 UUID。

Issue 创建或编辑时的联动要求：

- 当 issue 选择了 project，系统读取该 project 的同步绑定。
- 若没有出站可用绑定，`sync_out` 动作隐藏或禁用。
- 若只有一个可用出站绑定，按钮文案需要明确目标，例如“创建到禅道”或“创建到 GitLab”。
- 若存在多个可用绑定，必须先展示路由结果或让用户选择目标系统。
- `sync_out` 只能是显式动作，不能因为选择了 project 就自动反向创建外部事项。

Project 详情里展示同步绑定不是为了替代集成，而是让用户在创建和处理 issue 时能看到“这个项目连接到哪里”。

### 6.8 Issue 详情 / 外部同步动作

Issue 详情页需要把外部同步状态放在现有右侧信息结构中，和 Pull requests、Details、Resources 等信息保持同等密度，不新增独立外部工单页。

必须展示：

- 若 issue 已有 `source_system` / `source_id`，展示“已关联外部来源”、provider、外部链接和外部状态。
- 若 issue 已触发 `sync_out` 但尚未完成外部创建，展示排队或处理中状态，例如 `sync_out_status=queued`。
- 若 issue 属于 project，且 project 有可用出站绑定，展示目标 provider 和“同步到外部”动作。
- 若 issue 不属于 project，或 project 没有可用出站绑定，隐藏或禁用动作，并说明需要先绑定项目外部来源。
- 若同时存在多个可用出站绑定，必须让用户确认目标，或展示根据 issue 类型推导出的路由结果。

按钮行为：

- `同步到外部` 只能由用户显式点击或等价显式动作触发。
- 已关联外部来源的 issue 不允许再次反向创建。
- 已进入 `queued` / `processing` 的 issue 不允许重复排队。
- 成功接收请求后，按钮状态应立即反映 metadata 和同步事件，不等待真实外部创建完成。
- 失败时只显示失败类别和可重试状态，不展示 token、请求头、完整外部 API 响应。

这一区域的目标是让用户理解“当前 issue 是否已经和外部系统打通”，不是把 GitLab / 禅道 / 飞书完整工单详情搬进 Multica。

## 7. 工作区、外部空间和项目的关系

这里需要避免“工作区同步”概念混乱：

- Multica workspace：企业在 Multica 里的协作边界，也是所有连接配置的归属。
- GitLab group / project：GitLab 自己的组织和仓库边界。
- 禅道项目 / 产品：禅道自己的业务对象。
- 飞书企业 / 云盘 / Wiki：飞书自己的租户和知识对象。

“工作区级同步”指的是：在某个 Multica workspace 内，管理员配置哪些 GitLab group/project、禅道项目/产品、飞书资源可以被同步或绑定。它不表示把 GitLab、禅道、飞书的整个企业空间无差别同步进 Multica。

## 8. 同步协议

### 8.1 Issue 元数据

外部事项镜像到 Multica issue 后，必须写入统一 metadata：

- `source_system`
- `source_type`
- `source_id`
- `source_url`
- `external_status`
- `sync_hash`
- `last_synced_at`

这些字段用于幂等、回声抑制、外部链接展示和后续增量同步。

### 8.2 入站同步

第一版入站同步范围：

- GitLab：与成员、绑定仓库或绑定项目相关的 issue / MR 关联信息。
- 禅道：Bug、任务、需求、评论。
- 飞书：通过 Bot / IM 创建的 issue、选定云盘 / Wiki 资源元数据和内容索引。

入站同步必须幂等：

- 同一外部 issue 多次同步，只能对应一个 Multica issue。
- 同一外部评论多次同步，只能对应一个 Multica comment。
- 外部系统关闭、删除或状态变化时，Multica 只按映射规则更新状态，不误删本地讨论。

### 8.3 出站同步

第一版出站同步受控：

- Multica issue 必须属于已绑定外部来源的 project。
- 项目绑定必须允许出站。
- issue 必须显式标记 `sync_out=true`，或通过专用按钮触发。
- 已经有 `source_system/source_id` 的 issue 不再出站创建。

路由规则：

- 需求、任务、Bug 默认走禅道。
- 代码仓库类问题默认走 GitLab。
- 若一个 project 同时绑定禅道和 GitLab，页面必须展示当前路由结果，不能让用户猜。

评论同步：

- 第一版默认同步外部评论到 Multica。
- Multica 评论默认不反写外部。
- 只有显式标记 `sync_out=true` 的评论，或后续专用按钮触发的评论，才允许反写。

### 8.4 反向创建规则

- 只处理有 project 的 Multica issue。
- 只处理项目绑定里 `outbound_enabled=true` 的渠道。
- 已有 `source_system/source_id` 的 issue 不再反向创建，避免回声重复。
- 创建成功后在 Multica issue 评论一条系统记录，包含外部链接。
- 创建失败只记录同步事件和 warning comment，不盲目重试制造重复事项。

### 8.4.1 项目来源与 Multica issue 反向同步

如果一个 Multica project 是从 GitLab 或禅道导入、绑定或映射而来，那么该 project 内新建的 Multica issue 应该具备“反向同步到来源系统”的能力，但不能默认自动发生。

判断规则：

| Project 状态 | Multica issue 行为 |
| --- | --- |
| Project 没有任何外部绑定 | Issue 正常创建在 Multica；不展示或禁用反向同步动作。 |
| Project 只有资源绑定，例如 GitLab repo 或飞书云盘，但没有同步绑定 | Issue 正常创建；资源进入 agent 上下文，但不反向创建外部事项。 |
| Project 绑定 GitLab 且 `outbound_enabled=true`、`issue_sync_enabled=true` | Issue 创建后可以显式同步到 GitLab issue。 |
| Project 绑定禅道项目 / 产品且 `outbound_enabled=true`、`issue_sync_enabled=true` | Issue 创建后可以显式同步到禅道 Bug、任务或需求。 |
| Project 同时绑定 GitLab 与禅道 | 需要根据 issue 类型或用户选择路由；不能静默创建到两个系统。 |
| Issue 已经由外部事项入站镜像生成，已有 `source_system/source_id` | 不允许再次反向创建；只允许后续按同一外部来源增量同步。 |
| Issue 已经处于 `sync_out_status=queued|processing` | 重复点击或重复请求只返回当前状态，不产生新的外部事项或新的队列项。 |

Project 的“来源”不能只靠名称推断。必须来自明确数据：

- `integration_project_binding.provider`
- `integration_project_binding.external_ref`
- `integration_project_binding.outbound_enabled`
- `integration_project_binding.issue_sync_enabled`
- 可选的 `project_resource.resource_type`

当 project 绑定来自禅道时，issue 类型决定外部对象类型：

| Multica issue 类型 / 路由 hint | 默认外部对象 |
| --- | --- |
| Bug / 缺陷 | 禅道 Bug |
| Task / 任务 | 禅道任务 |
| Story / 需求 | 禅道需求 |
| 未指定类型 | UI 要求用户选择，或使用 project binding 的默认类型。 |

当 project 绑定来自 GitLab 时，第一版只反向创建 GitLab issue，不反向创建 MR。MR 仍由 Git / GitLab 代码协作能力独立处理。

因此，答案是：应该支持反向同步，但必须满足 project 明确绑定、绑定允许出站、用户显式触发、幂等检查通过这四个条件。

### 8.5 资源绑定、同步绑定和账号绑定的区别

三类绑定必须分开建模、分开展示：

| 类型 | 解决的问题 | 例子 | 是否等于自动同步 |
| --- | --- | --- | --- |
| 账号绑定 | 这个 Multica 成员对应外部系统里的哪个账号。 | 用户 A 绑定自己的 GitLab PAT、禅道账号、飞书身份。 | 否。账号只解决身份、权限和归因。 |
| 资源绑定 | 这个 project 的上下文里有哪些外部资源。 | GitLab repo、飞书云盘文件夹、飞书 Wiki、禅道项目。 | 否。资源只说明上下文来源。 |
| 同步绑定 | 这个 project 和哪个外部连接之间允许同步哪些对象。 | 禅道 Bug 入站、GitLab issue 出站、飞书知识同步。 | 取决于开关和 worker 状态。 |

因此，Project 详情里出现一个 GitLab 仓库资源，不代表 GitLab issue 已经同步；账号管理里出现一个 GitLab PAT，也不代表该账号有权把所有仓库同步进 Multica。自动同步必须同时满足 workspace 连接、成员或服务账号权限、project 同步绑定、同步 worker 可用、幂等记录正常这几个条件。

### 8.6 出站同步请求协议

在真实 GitLab / 禅道 / 飞书写入 worker 完成前，可以先实现一个受控的“出站同步请求”切片，用来打通 UI、权限、路由、metadata 和审计事件。该切片的语义是“已接收并排队”，不是“外部事项已创建成功”。

触发条件：

- Issue 必须属于某个 Multica project。
- Project 必须存在至少一个同步绑定。
- 绑定所属 workspace connection 必须启用，并处于可参与同步的状态。
- 绑定必须开启 `outbound_enabled` 和 `issue_sync_enabled`。
- Issue 不能已有 `source_system` / `source_id`，避免镜像 issue 再次反向创建。
- 用户必须显式触发，例如按钮、字段或 `sync_out=true` 标签。

路由结果：

- 若只有一个可用出站绑定，自动选择该 provider。
- 若用户指定 provider，只能在匹配且可用的绑定中选择。
- 若同时存在禅道和 GitLab，按 issue 类型路由：Bug / 任务 / 需求默认禅道，代码仓库类问题默认 GitLab。
- 若路由不唯一，API 返回冲突状态，UI 要求用户选择目标系统。

请求成功后写入：

- Issue metadata：`sync_out=true`、`sync_out_provider`、`sync_out_connection_id`、`sync_out_status=queued`、`sync_out_requested_at`、`sync_out_event_id`。
- 同步事件：`direction=outbound`、`object_type=issue`、`status=success` 或更准确的 `queued` 状态。
- Realtime 事件：至少刷新 issue metadata 和集成最近事件。

真实外部创建成功后再补写：

- `source_system`
- `source_type`
- `source_id`
- `source_url`
- `external_status`
- `sync_hash`
- `last_synced_at`

如果外部创建失败：

- 不自动重复创建，除非 worker 有外部幂等键或安全的 retry key。
- 记录同步事件和 warning comment。
- UI 显示失败原因类别和最近失败时间。
- 保留 `sync_out_status=failed` 或等价状态，便于人工重试。

### 8.7 入站同步 worker 要求

完整入站同步不能只靠配置保存。每个 provider 至少需要：

| Provider | 触发方式 | 幂等键 | 最小对象 |
| --- | --- | --- | --- |
| GitLab | webhook + polling fallback | `provider + project/repo + external issue/MR id` | issue、MR 关联、评论摘要。 |
| 禅道 | polling / CLI / webhook，按企业版本能力选择 | `provider + zentao project/product + bug/task/story id` | Bug、任务、需求、评论。 |
| 飞书 | Bot 事件、云盘 / Wiki 定时扫描或事件回调 | `provider + tenant/app + message/file/wiki node id` | `/issue` 事件、Bot 身份、云盘 / Wiki 资源元数据。 |

worker 必须写 `integration_sync_event` 或等价审计记录。失败不能中断其他 provider、其他 project、其他成员账号的同步批次。

### 8.8 同步生命周期

完整闭环需要把“配置成功”和“同步成功”拆成多个可观测阶段：

| 阶段 | 触发 | 数据写入 | 用户可见位置 | 成功标准 |
| --- | --- | --- | --- | --- |
| 连接配置 | 管理员在 Git / 禅道 / 飞书页保存连接。 | `integration_connection`，内部 `integration_sync_event`。 | 对应 provider 页、集成。 | 连接保存成功，状态不是“同步完成”。 |
| 账号绑定 | 成员在账号管理中保存个人账号。 | `integration_user_account`。 | 账号管理、集成成员映射摘要。 | 凭据遮蔽展示；未测试前只能显示“已配置”。 |
| 项目绑定 | 管理员绑定 project 与外部项目 / 仓库 / 资源。 | `integration_project_binding`、可选 `project_resource`。 | Project Resources、集成摘要。 | 能明确看到 provider、连接名、外部引用和同步开关。 |
| 出站请求 | 用户在 issue 上显式点击 `sync_out`。 | issue metadata、`integration_sync_event`。 | Issue 详情、集成最近事件。 | 进入 `queued`，且同一 issue 重复点击不会重复排队。 |
| 外部创建 | worker 调用 GitLab / 禅道 / 飞书 API。 | `source_*` metadata、外部链接、成功事件。 | Issue 详情外部来源区。 | 外部系统有且仅有一个对应事项，Multica 回填外部 id。 |
| 入站增量 | webhook / polling / CLI 扫描外部变化。 | issue、comment、metadata、sync event。 | Issue 详情、列表、集成。 | 同一外部对象重复扫描不产生重复 Multica 对象。 |
| 失败审计 | 任一阶段失败。 | `integration_sync_event.status=error|warning|skipped`，必要时 warning comment。 | 集成、Issue 详情。 | 不中断其他 provider / project / member 的同步；错误不泄露凭据。 |

任何页面如果只完成前 3 个阶段，只能写“已配置”“已绑定”“同步已开启”。只有外部创建或入站增量阶段成功后，才能写“最近同步成功”或展示外部来源链接。

## 9. 权限与安全

权限规则：

- 工作区 owner / admin 可以管理工作区连接、功能开关、项目绑定、资源。
- 成员可以管理自己的账号管理项。
- 普通成员可以查看连接状态和同步健康，但不能看到服务账号凭据，也不能修改工作区连接。
- 成员映射页面可以展示“是否已配置账号”，但不能展示 token 明文。

凭据规则：

- 所有外部凭据必须加密存储。
- 当服务端未配置集成凭据加密能力时，UI 必须提示“凭据存储未启用”，并禁用保存 token。
- 日志、toast、同步事件、评论里不得输出 token 明文。
- 删除账号或断开连接需要清理后续同步资格，但历史同步记录保留用于审计。

## 10. 数据与 API 需求

现有集成模块可作为底座，但需要补齐以下能力：

| 对象 | 当前能力 | 需求差距 |
| --- | --- | --- |
| `integration_connection` | provider、name、base_url、config、status、sync_enabled | 需要连接健康、最近同步、最近错误、连接类型、服务账号状态。 |
| `integration_user_account` | 每连接每用户一个账号、加密凭据、同步开关 | 需要同用户多账号、账号名、权限范围、过期时间、最近使用、最近错误、测试连接结果、默认账号。 |
| `integration_project_binding` | project + connection + external_ref + 同步开关 | 需要类型化外部引用、路由规则、状态映射、绑定健康、最近同步。 |
| 同步事件 | 暂无统一模型 | 需要 `integration_sync_event` 或等价审计记录，记录 provider、对象类型、方向、结果、错误、外部链接。 |
| 项目资源 | 已有 `resource_type + resource_ref` 模型 | 需要扩展 GitLab、飞书、禅道资源类型，并在资源页和 project 详情页可管理。 |

API 层要求：

- 工作区级接口继续按 `workspace_id` 隔离。
- 所有 Settings 页面查询使用 React Query，query key 包含 `wsId`。
- API 响应进入 `packages/core/api/schemas.ts` 解析，前端不能直接 cast 网络 JSON。
- mutation 后需要更新或失效对应 query，不能直接写 Zustand 持久化服务端数据。

当前实现切片必须在文档和验收中标明状态：

- 已落地的配置底座可以包含 Settings 导航、账号表单、连接表单、project resource 类型扩展和 project 详情绑定入口。
- 未落地的同步能力不能在 UI 或文案里承诺“已经自动同步”。在 worker / webhook / 幂等模型完成前，只能称为“配置”“绑定”“准备同步”或“同步设置”。
- 后端若先使用内置 worker 以外的进程承载同步，也必须由 Multica server 配置和数据库统一管理，不能要求最终用户手工维护一个外挂 sidecar。

### 10.1 第一版数据状态语义

为了避免页面“看起来已开启”但后台并未同步，第一版需要统一状态文案：

| 状态 | 含义 | 可以展示在 |
| --- | --- | --- |
| `not_configured` | workspace 还没有该 provider 连接。 | Git / 禅道 / 飞书、集成、账号管理空态 |
| `configured` | 连接或账号已保存，但未完成健康检查。 | 连接卡片、账号列表 |
| `ready` | 凭据可用、健康检查通过，允许参与同步。 | 连接状态、项目绑定摘要 |
| `sync_enabled` | 该连接或绑定开启同步开关，但不表示本次同步已经成功。 | provider 卡片、绑定行 |
| `syncing` | worker 正在处理该 provider 或绑定。 | 集成、绑定详情 |
| `last_success` | 最近一次同步成功，显示时间和对象数量。 | 集成、资源、Project 详情 |
| `error` | 最近一次连接、权限或同步失败。 | 集成、provider 详情、账号行 |
| `disabled` | 管理员关闭连接、账号或绑定。 | 所有相关页面 |

UI 文案应优先使用“已配置”“已连接”“同步已开启”“最近同步成功”这类精确状态，不用一个“已同步”覆盖所有状态。

### 10.2 当前实现状态与差距

当前仓库中的实现已经覆盖配置骨架，但同步闭环仍未完成。需求验收时按下表判断：

| 模块 | 当前实现状态 | 可验收内容 | 不能宣称的内容 |
| --- | --- | --- | --- |
| Settings 导航 | `settings-page.tsx` 已增加账号管理、资源、Git、禅道、飞书、集成入口。 | 信息架构、术语、旧 tab 兼容、原 Settings 风格。 | 自动同步已生效。 |
| 账号管理 | `tokens-tab.tsx` 保留 PAT，并新增外部账号表单。 | PAT 不回退、外部账号配置入口、遮蔽态、凭据加密禁用态。 | GitLab / 禅道 / 飞书 token 已真实校验并可同步。 |
| 资源 | `repositories-tab.tsx` 保留 `workspace.repos`，增加飞书 / 禅道资源快捷入口。 | 代码仓库不回退、资源命名、到 Project Resources 的路径。 | 飞书云盘 / 禅道项目已经被后台扫描。 |
| Git 页面 | `github` tab 内保留 GitHub，并并列 GitLab 配置卡。 | GitHub 原能力可继续操作；GitLab 连接配置入口清晰。 | GitLab MR / issue webhook 已全量接通。 |
| 禅道页面 | 通过 provider settings page 表达 workspace 级连接。 | 禅道连接、开关和账号引导入口。 | 禅道 API / CLI 已完成真实同步；project binding 不在此页编辑。 |
| 飞书页面 | 保留现有飞书 Bot 组件能力，并新增 provider settings page。 | 飞书 Bot 原能力可发现；飞书 workspace 级连接和开关入口。 | 飞书云盘 / Wiki 全文同步已经上线；project binding 不在此页编辑。 |
| 集成 | `IntegrationsTab` 展示 provider 卡、账号数、绑定数、最近事件。 | “总览”定位、健康摘要、快捷入口。 | 所有事件都来自真实 worker。 |
| Project Resources | `project-resources-section.tsx` 支持 GitLab、飞书、禅道资源类型和当前项目同步绑定配置。 | Project 属性联动、资源类型扩展、绑定摘要、当前项目同步路由保存。 | 创建 issue 时已自动反向创建外部事项。 |
| Integration DB/API | 已有 connection、user account、project binding、sync event 模型雏形。 | 配置保存、列表展示、基础审计。 | provider SDK、webhook、worker、外部幂等全部完成。 |

下一步实现优先级应该是：

1. 把页面上的外部账号和连接从“可保存”升级到“可测试连接”。
2. 继续把 Project Resources 里的 project binding `external_ref` 从轻量输入升级为更完整的 provider 类型化表单。
3. 实现 issue `sync_out` 显式请求和审计事件。
4. 再实现 provider worker / webhook 的真实外部读写。
5. 最后补评论同步、状态映射、成员多账号默认选择和批量失败恢复。

### 10.3 页面操作到数据对象的映射

每个页面操作都必须落到明确的数据对象，避免把不同层级的配置混在同一个表单里：

| 页面操作 | 主要对象 | 必要字段 / 副作用 | 不应该做的事 |
| --- | --- | --- | --- |
| 创建 Multica PAT | `personal_access_token` | 生成 `mul_` token，只展示一次完整明文，数据库只存哈希。 | 不把 PAT 当外部账号凭据保存。 |
| 添加 GitLab / 禅道 / 飞书个人账号 | `integration_user_account` | 保存 provider、connection、account_name、credential 密文、scopes、expires_at、sync_enabled。 | 不修改 workspace 连接开关，不展示 token 明文。 |
| 保存 GitLab / 禅道 / 飞书 workspace 连接 | `integration_connection` | 保存 provider、name、base_url、config、status、sync_enabled、服务账号状态。 | 不自动创建项目绑定，不自动同步所有外部空间。 |
| 绑定 project 外部来源 | `integration_project_binding` | 保存 project、connection、external_ref、sync flags、route/default type。 | 不要求成员个人 token，不自动创建外部 issue。 |
| 添加 project resource | `project_resource` | 保存 resource_type、resource_ref，并进入 agent 项目上下文。 | 不等同于打开 issue 同步或外部写权限。 |
| 点击 issue `sync_out` | issue metadata、`integration_sync_event` | 写入 `sync_out_*` metadata，记录 outbound event，刷新 Issue 详情与集成。 | 不绕过 project binding，不重复排队，不直接暴露 provider API 错误细节。 |
| worker 完成外部创建 | issue metadata、comment / activity、`integration_sync_event` | 回填 `source_*`、外部链接、外部状态，写系统记录。 | 不再创建第二个外部事项，不覆盖用户手动取消状态。 |
| 入站同步外部变更 | issue、comment、metadata、`integration_sync_event` | 用外部幂等键 upsert，按状态映射更新。 | 不删除本地讨论，不自动分配 agent。 |

### 10.4 最小 API 行为要求

第一版 API 不需要一次性实现所有 provider SDK，但基础行为必须稳定：

- 所有集成 API 必须按 workspace 过滤，不能只凭 connection id 查询或修改跨 workspace 数据。
- 创建 / 更新凭据时，服务端必须确认凭据加密能力可用；不可用时返回可解释错误。
- 账号、连接、项目绑定、同步事件的列表接口都要支持空态，不能让前端靠异常判断“未配置”。
- `sync_out` 请求必须是幂等的：已有 `source_system/source_id`、已有 `queued|processing` 状态、或无可用绑定时，应返回明确的 skipped / conflict / validation 状态。
- provider 健康检查和真实同步 worker 可以后置，但 API 响应必须区分 `configured`、`ready`、`sync_enabled`、`last_success`、`error`。
- 错误响应只能包含错误类别、provider、对象 id 和可行动作，不包含 token、请求头、完整外部 API body。

## 11. UI 约束

必须遵守现有 Multica Settings 风格：

- 左侧设置导航不重做。
- 右侧内容区保持窄内容宽度，不做全屏大后台。
- 使用现有 `Card`、`CardContent`、`Button`、`Switch`、`Input`、`Select`、`Tooltip` 等组件。
- 字号以 `text-sm`、`text-xs` 为主，不使用营销式大标题。
- 状态用小面积 badge、文案、icon 表达，不做大面积彩色装饰。
- 列表、表格、开关行保持高信息密度。
- 详细配置页可以有表单；集成只做总览。

这次页面设计必须是在原功能上修改，不是完全重新设计。

### 11.1 页面设计基准

Figma 或页面实现必须以当前 Settings 页面为基准，而不是新建一套企业后台视觉：

- 页面宽度沿用 `max-w-3xl` 级别的窄内容区。
- 左侧 Settings tab 分组仍是 My Account 和 Workspace，不新增企业后台顶栏。
- 一级标题使用现有 `text-sm font-semibold` 级别，不使用大号 hero 标题。
- Provider 卡片只能承载连接状态和少量操作，不做大屏仪表盘。
- 表格和列表优先展示对象状态，避免长段说明文字。
- 说明文字只用于解释配置边界和下一步动作，不介绍产品理念。
- 状态 badge 使用中性色或小面积语义色，不能用大面积彩色块表达 provider 品牌。
- 所有新增入口必须能从旧入口自然过渡，例如 GitHub -> Git、Repositories -> 资源、API Tokens -> 账号管理。

### 11.2 Figma 交付校准

若继续产出 Figma frame，必须按以下方式评审：

| Frame | 设计目标 | 不能出现 |
| --- | --- | --- |
| Settings 信息架构总览 | 展示原 Settings 左侧导航如何增量变化。 | 新后台首页、大卡片导航、营销式说明页。 |
| 我的账号 / 账号管理 | 在 PAT 原能力旁边增加外部账号管理。 | 把 PAT 隐藏成普通账号、展示 token 明文。 |
| Workspace / 资源 | 保留代码仓库分组，并补飞书 / 禅道资源入口。 | 把 repo 管理改成全新资源市场。 |
| Workspace / Git | GitHub 与 GitLab 并列，但 GitHub 原功能可识别。 | 把 GitHub 原 install / webhook 边界抹掉。 |
| Workspace / 禅道 | 连接、总开关、账号引导、连接健康和同步能力说明。 | 在这里编辑具体 project 的项目 / 产品绑定，或要求普通成员在这里填个人 token。 |
| Workspace / 飞书 | Bot、App、IM、云盘 / Wiki 权限范围和连接健康。 | 在这里编辑具体 project 的云盘 / Wiki 绑定，把中文主标签写成英文名，或把 Bot 绑定能力删掉。 |
| Workspace / 集成 | 总览、健康、最近事件、成员映射摘要。 | 在集成堆所有 token 表单和原始 JSON。 |

Figma 只证明布局和信息架构，不证明同步能力已经实现。任何 frame 中出现“已同步全部事项”之类文案，都应改为“同步已开启”“最近同步”“等待首次同步”等状态文案。

## 12. 验收标准

信息架构验收：

- 用户能明确知道：账号在哪里加、资源在哪里绑定、Git / 禅道 / 飞书在哪里开启、总体健康在哪里看。
- `账号管理`、`资源`、`Git`、`禅道`、`飞书`、`集成`在设置导航中职责清晰。
- 集成不再承载所有细节配置。
- Issue 详情能明确展示“已关联外部来源”“等待出站同步”“无法同步，需要先绑定 project 来源”三类状态。

功能验收：

- 现有 Multica PAT 创建 / 撤销不回退。
- 现有工作区代码仓库配置不回退。
- 现有 GitHub App PR 关联能力不回退。
- 现有飞书 Bot 绑定 agent 的能力不回退。
- GitLab、禅道、飞书账号支持多账号管理。
- Project 详情支持绑定 GitLab 仓库、飞书云盘 / Wiki、禅道项目 / 产品。
- 项目绑定支持入站、出站、issue 同步、知识同步开关。
- 未绑定外部来源的 project 不会反向创建外部事项。
- 已绑定外部来源的 issue 只能通过显式动作进入 `sync_out` 队列，并写入 metadata 和同步事件。

同步验收：

- 同一外部 issue 重跑同步不会创建重复 Multica issue。
- 同一 Multica issue 反向创建成功后，重跑不会再次创建外部 issue。
- 同一 Multica issue 进入 `queued` 后，重复点击或重复请求不会再次创建新的外部事项。
- GitLab / 禅道 / 飞书 token 失效时，只记录失败，不中断其他 provider 同步。
- Multica 写入失败时记录同步事件，不吞掉错误。
- 不出现重复 issue、重复评论、误关外部事项。

术语验收：

- 中文界面主标签使用“账号管理”“飞书”“集成”。
- 中文界面不把飞书主入口写成英文名。
- 中文界面不把账号管理主入口写成英文令牌管理。
- 英文界面账号管理主入口使用“Access Tokens”，不再使用“API Tokens”作为一级页签和页面标题。
- “API Token”可以作为 Multica PAT 分组或技术对象名称出现，但不能替代“账号管理 / Access Tokens”的页面级命名。

视觉验收：

- 使用现有 Settings 左侧导航和右侧窄内容区。
- 文字不溢出、不重叠。
- 卡片密度、开关行、状态 badge 与现有 GitHub / Tokens / Repositories 页面一致。
- 不出现偏营销化、仪表盘化的全新视觉体系。

## 13. 实施阶段建议

### Phase 1：文档和信息架构

- 确认本需求文档。
- 确认 Settings 导航项、术语和页面归属。
- 确认 `集成 = 总览`，不是大表单集合。
- 保留旧 query tab 的兼容策略。

当前状态：页面布局和术语方向已经基本验证，本文档 v7 用作后续实现口径。

### Phase 2：页面拆分与现有能力归位

- Tokens 升级为账号管理。
- Repositories 升级为资源。
- GitHub 升级为 Git，GitHub 原功能不回退。
- Integrations 改为集成总览。
- 新增禅道、飞书工作区设置页。

当前状态：配置页骨架已经形成；后续主要是把表单字段、状态、错误和权限从演示态收敛到真实数据模型。页面不能因为骨架完成就宣称同步上线。

### Phase 3：账号、连接、绑定模型补齐

- 支持成员多账号。
- 支持服务账号和成员账号分层。
- 支持 GitLab / 禅道 / 飞书连接健康检查。
- 支持 project 级外部绑定和资源绑定。

当前状态：通用 integration 表和项目资源扩展已具备基础；多账号唯一约束、凭据加密状态、健康检查和类型化外部引用还需要补齐。账号保存、连接保存、绑定保存应先做到可测试、可审计。

### Phase 4：同步健康与审计

- 增加同步事件模型。
- 增加连接健康、最近同步、最近错误。
- 集成展示总览和成员映射摘要。

当前状态：已有最近事件展示入口；需要保证事件由真实 worker / webhook 或出站请求 API 写入，并覆盖失败、跳过、重试、权限不足等状态。

### Phase 5：双向 issue 与资源同步

- 外部 issue / 评论入站同步。
- 受控出站创建。
- 飞书云盘 / Wiki 资源进入 project context 或检索上下文。
- 幂等和失败场景测试。

当前状态：这是下一阶段核心能力。未完成前，任何页面都不能把“配置完成”写成“同步完成”。优先切片是 `sync_out` 显式请求、metadata 和同步事件，再接真实 provider 写入。

## 14. 文档级验收清单

本需求文档完成后，后续产品、设计、开发应能用它直接判断实现是否偏航：

- 能明确解释为什么这是 Multica 内置 integration module，而不是外挂同步服务。
- 能明确说明本需求继承了 PAT、GitHub PR、飞书 Bot、Project Resources 等原有 docs 中的稳定能力。
- 能明确解释 Workspace、账号、连接、项目资源、项目同步绑定、issue metadata 的边界。
- 能明确回答 GitLab / 禅道 / 飞书的“工作区同步”指的是 Multica workspace 内的授权范围，不是无差别同步外部企业空间。
- 能明确回答普通成员在哪里添加个人账号，管理员在哪里配置企业连接。
- 能明确回答 Project 详情为什么只展示当前 project 的资源和绑定，而不是替代 workspace 设置。
- 能明确回答双向 issue 同步何时允许、何时禁止、如何避免重复。
- 能明确回答 Issue 详情的外部同步动作为什么只是显式请求，不等于外部事项已创建成功。
- 能明确回答当前页面实现可以验收哪些内容，哪些仍需 worker / webhook / provider API 完成。
- 能明确约束 Figma 和 UI 必须沿用原 Settings 风格，而不是完全重新设计。

## 15. 需要产品确认的问题

1. GitLab 第一版采用 GitLab App / OAuth，还是企业 PAT + webhook 的轻量连接方式？
2. 禅道第一版以禅道 CLI 账号为主，还是以服务账号 + API token 为主？
3. 飞书云盘同步第一版同步全文内容，还是只同步资源元数据并按需拉取？
4. 成员多账号在 UI 上是否允许设置默认账号？如果允许，默认账号按 provider 维度还是按 workspace connection 维度？
5. 反向创建的显式触发使用标签、字段、按钮，还是三者都支持但按钮优先？
6. Project 详情里的外部绑定入口放在资源区、属性区，还是拆成“资源”和“同步绑定”两个区块？
