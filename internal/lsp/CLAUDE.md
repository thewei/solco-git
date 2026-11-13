# lsp/
> L2 | 父级: ../../CLAUDE.md

语言服务器客户端（2026-11-13 决定第 (4) 条，E4 只读：诊断 / 悬停 / 跳转定义）。只用用户机器上已装的服务器（PATH，外加 Finder 启动的应用看不到的常见目录），不下载、不安装、不管版本；找不到的语言安静回退为只有语法高亮。每个工作区每种语言至多一个 stdio 子进程，首次打开该语言的文件时才起，随工作区结束；每分钟最多重启 3 次，再坏就保持关闭并留一句话给状态栏。包不碰界面、不碰文件系统（文本由调用方交来）；不 import MyGo、Herdr 或任何 `solco` 的包。

JSON-RPC 帧与请求配对用 `github.com/sourcegraph/jsonrpc2`（MIT，2026-11 仍在维护，无传递依赖）；协议类型只有这里用到的几个，自己写（成熟的类型库 `go.lsp.dev/protocol` 会带进 zap 等依赖，而只读能力只要这几个结构）。位置一律是 LSP 默认的 UTF-16，与编辑器的字节偏移的换算只在 `protocol.go`。

日志只有操作名、语言、计数、耗时、错误类别——从不记路径、文本、诊断、请求；服务器的 stderr 丢弃。

成员清单
- `catalog.go`：`Specs`（go / typescript+javascript / rust / python / clangd / swift / zig 的命令候选、扩展名 → languageId）、`SpecFor`、`Resolve` / `Detect`（Settings 的清单）、`findExecutable`（PATH 再加常见目录）、`childEnv`（给子进程补全 PATH，Finder 启动的应用才找得到 `go` / `node` / `cargo`）。
- `protocol.go`：`Position` / `Range` / `Location` / `Diagnostic`、`UTF16Col` ⇄ `ByteCol`、`FileURI` / `URIPath`、行索引（字节偏移 ⇄ 协议位置）、hover 内容转纯文本（字符串 / MarkedString / MarkupContent / 数组，去掉代码围栏）、definition 结果（Location / Location[] / LocationLink[]）。
- `client.go`：一个服务器进程——initialize 握手（只声明文档同步、publishDiagnostics 带版本、hover、definition）、open / change（整份文本，服务器要求 None 时不发）/ save / close 通知、hover 与 definition 请求（8 s 超时）、对服务器发来的请求的最朴素应答（configuration → 全 null、applyEdit → 不应用，因为本阶段只读）、有序关闭（shutdown → exit → 超时才 kill）。
- `manager.go`：每工作区一个 `Manager`——**对账式同步**：界面只说「这个文档现在是（版本，文本）」，服务器端的 open / change / close / save 由 `reconcile` 按序发出（`sendMu` 串行），崩溃重启后整体重放；诊断按它们所属的版本换算成字节偏移后交给界面，版本已落后的丢掉；`Hover` / `Definition` 先等服务器就绪并对账，请求的版本与文档当前版本不同就返回 `ErrStale`；`Status`（无 / 未装 / 空闲 / 启动中 / 运行 / 关闭）。2 MiB 以上的文档不同步。
- `lsptest/`：测试用的迷你服务器（测试二进制自己，`LSP_FAKE=1`），说真实帧化 JSON-RPC：含 BAD 的行报错（UTF-16 位置、带版本）、hover、definition（仓库内 / null / 仓库外）、含 CRASH 的变更让进程退出、把收到的每个方法写进日志文件；`Serve` 由各包的 TestMain 先调用。不进应用二进制。
- `manager_test.go`：诊断换算成字节偏移并带版本、一次变更换掉诊断且通知有序（initialize → didOpen → didChange → didSave → didClose）、hover / definition 的编辑器语义、旧版本请求被丢弃、没装服务器与不认识的语言都是安静的状态不是错误、崩溃重启并重放文档后超过上限保持关闭、关闭按序结束、列的双向换算与越界收敛、`SpecFor` 与 languageId、hover / definition 各种 JSON 形态。变异检查：UTF-16 换算改成直通、重启不重放，对应测试必红。
- `real_gopls_test.go`：装了 gopls 就对一个一次性 Go 模块跑真的（诊断落在 `Greet` 调用上、hover 带签名与注释、定义落在 lib.go 第 3 行），没装就跳过（`SOLCOGIT_GOPLS` 可指定）。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
