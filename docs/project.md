# 微光项目文档

[返回项目首页](../README.md)

## 本地开发

需要 Go 1.27.3 或更新版本。进入本目录后运行：

```sh
go run .
```

英文字典、简体中文字典和 MyGo 生成的本地化访问器位于 [`assets/locales`](../assets/locales)。

macOS 本地打包：

```sh
go run github.com/egoist/mygo/cmd/mygo@v0.9.0 build -skip-dmg
```

## 模型与网络

打开「设置 → 模型」，可添加命名的 Provider，为每组配置 Base URL、API Key、可选模型和生成参数，并指定默认模型。聊天顶部切换 Provider，输入框右下角选择具体模型。Base URL 填接口根地址，例如 `https://api.openai.com/v1`。生成参数留空时使用模型默认值，保存后的配置从下一次发送开始生效。

「设置 → 通用 → 网络」默认使用系统代理，也可选择直连或填写自定义 HTTP、HTTPS、SOCKS5 代理地址。代理保存在 SQLite 的 `config.proxy` 配置项，聊天、上下文压缩、测试连接、HTTP MCP 和网页工具共用这套配置。连接测试使用表单当前值。Linux 桌面的系统代理与 PAC 由 GIO 的桌面代理后端解析，常见发行版通过 `glib-networking` 提供。无桌面代理后端时读取 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY` 和 `NO_PROXY`。

初次启动默认填入 DeepSeek 官方地址和 `deepseek-flash`。在设置页输入服务商的 API Key，可先点「测试连接」，通过后再保存。已保存的密钥每个字符显示为一个圆点，直接输入新密钥即可替换。

## 本地数据与模型请求

会话、消息和连接设置保存在系统用户应用数据目录的 `微光/microlight.sqlite3`，由纯 Go 的 `modernc.org/sqlite` 驱动。首次打开数据库会导入可用的旧版 JSON 数据。API Key 随配置以明文存入本机数据库，macOS 上新建数据库文件的权限为 `0600`。发送消息时，模型请求会携带本轮输入、所需的会话上下文及适用的附件或工具结果。

在输入框粘贴文件，或从「＋」菜单选择「上传文件」。每条消息最多 8 个附件，单个最多 8 MiB，合计最多 32 MiB。附件随草稿和消息保存在本机数据库。JPEG、PNG、GIF 和 WebP 使用支持图像输入的模型处理。UTF-8 文本附件每条消息最多向模型提供 32 KiB 内容，PDF 等二进制附件向模型提供文件名。

Enter 发送，Shift + Enter 换行。可停止生成，也可编辑消息后重发或重新生成回复。

## Agent 与工作区

聊天请求由 Eino ADK Runner 驱动 Deep Agent。权限模式按会话配置，可在聊天中切换。文件、命令、网络与 MCP 工具的审批取决于当前会话的权限和操作类型。MCP 支持 Streamable HTTP 和 stdio。

为会话选择工作区后，每次发送或继续执行都会读取从 Git 仓库根到工作区目录这一段路径上的 `AGENTS.md`；非 Git 目录只读工作区自身。更具体的目录说明优先，进入更深的子目录时 Agent 按需通过文件工具查看说明。项目说明仅用于指导相关任务，不会改变文件、网络或工具的审批规则。可复用流程放在项目的 `.agents/skills/<名称>/SKILL.md` 或全局的 `~/.agents/skills/`；设置页的技能列表只显示全局技能。

## 会话与上下文

Eino 版本由 [`go.mod`](../go.mod) 固定。SQLite 的 `adk_session_events` 保存模型侧的会话事件，`adk_checkpoints` 保存等待澄清时的执行检查点。旧会话首次续聊时以已有原始消息初始化 Session，之后每轮只提交新消息，由 Runner 重放历史。编辑旧消息创建的分支会从编辑点之前的原始消息建立独立 Session。

长对话超过「上下文压缩阈值」（默认估算 256,000 Token）时，Eino 的 Summarization middleware 摘要较早的完整轮次，并写入 `messages_replaced` 事件。后续请求由 Runner 恢复压缩后的上下文，界面仍从原始消息表读取全文和附件。新版生成的澄清卡片可以在重启后继续作答，旧版未完成的澄清需要重新生成。

## 代码结构

根目录的 `main.go` 是程序入口，也负责嵌入 `assets/`。桌面启动在 `internal/app`，聊天和设置界面分别在 `internal/pages/chat` 和 `internal/pages/settings`。输入框和文件预览有独立组件包。SQLite、Eino 会话事件、工具审批、原生命令、MCP、技能和工作区各有独立包。Linux 系统代理设置页的桌面环境适配在 `internal/desktop`。

`pkg/diff` 提供文本行差异，`pkg/markup` 为 MyGo 提供 Markdown 和 LaTeX 渲染选项，`pkg/proxy` 提供支持 macOS、Windows 和 Linux 系统代理的 HTTP transport。可独立复用的包放在 `pkg/`。

## A2UI

`pkg/a2ui` 提供 A2UI 1.0 协议、离线 Schema 校验、界面状态、数据绑定和双向函数调用。`pkg/a2uimygo` 将 Basic Catalog 渲染为原生 MyGo 控件，并允许宿主注册自定义组件。远程媒体使用宿主提供的 HTTP client。音频使用 MyGo 解码，普通视频由 MyGo 的系统媒体后端解码，macOS 使用 AVFoundation，Windows 使用 Media Foundation，Linux 使用 GStreamer。宿主可通过 `MediaOptions.DecodeVideo` 覆盖默认解码。

`pkg/a2uiagent` 提供目录协商、模型提示词、Direct JSON 和 Express 的解析与流式输出。聊天中的澄清表单使用 A2UI，并将提交交给 Eino 恢复执行。普通聊天回复仍使用既有的 Markdown 渲染路径。

官方 Schema、Basic Catalog 和测试用例固定在上游提交 `84474c34f151f19eaebd1f416065b027722adc15`。以下命令重跑已接入的官方用例：

```sh
CGO_ENABLED=0 go test ./pkg/a2ui ./pkg/a2uiagent ./pkg/a2uimygo -run TestOfficial
```

测试覆盖协议校验、响应编译与流式解析、MyGo 原生控件交互和录制的多轮会话。各测试包的官方用例目录含来源说明，测试输出列出已接入但暂未覆盖的用例及原因。当前仍有官方套件待接入。

## CI 与发布

普通分支 push 和 PR 在 Ubuntu 24.04 跑全量 Go 测试。平台后缀、构建约束和桌面功能入口决定是否加跑 macOS 或 Windows 测试。依赖、打包配置或 CI 配置改动会测试三个系统。Actions 的「Microlight CI → Run workflow」也可以手动运行三平台测试。

推送 `vX.Y.Z` tag 后，三个系统的测试通过才会打包并发布到对应的 GitHub Release。MyGo CLI 版本来自 `go.mod`，安装包内部版本来自 tag。Go 测试和打包统一设置 `CGO_ENABLED=0`。发布文件从 `dist/` 顶层上传。详见 [GitHub Actions workflow](../.github/workflows/ci.yml)。

| 平台 | 架构 | 发布文件 |
| --- | --- | --- |
| macOS | Universal（Apple Silicon 和 Intel） | `microlight-vX.Y.Z-darwin-universal.dmg` |
| Windows | amd64 | `microlight-vX.Y.Z-windows-amd64.exe` |
| Linux | amd64 | `microlight-vX.Y.Z-linux-amd64.deb`、`.rpm`、`.tar.gz` |
