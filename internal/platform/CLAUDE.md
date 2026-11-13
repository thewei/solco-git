# platform/
> L2 | 父级: ../../CLAUDE.md

有时限的桌面小助手：用系统默认编辑器打开文件（MyGo 的 `Shell.OpenPath` 按类型打开，`open -t` 的文本编辑器框架没有对应调用）；Quick Look（`qlmanage -p`，框架没有对应调用，每次一个子进程、不等它退出）；到 Solco 的单向链接（`solco://terminal?cwd=`：找到 Solco.app 并 `open -a` 把仓库根交给它，由 Herdr 拥有终端；不 import solco，链接里没有 command，路径不进日志）。参数一律 argv，不拼 shell 字符串；超时 `DesktopActionTimeout`（3 s）。

成员清单
- `desktop_actions.go`：`OpenFile`、`QuickLook` 与超时常量。
- `solco_link.go`：`SolcoTerminalURL`（只带一个绝对路径）、`SolcoLink`（`SOLCO_APP` 覆盖 → `/Applications` → `~/Applications`，必须是 `.app` 目录，答案缓存 5 s 因为界面每帧都在问；`Available` / `OpenTerminal`，只用 argv、链接交给选中的那个包；非 macOS 恒为「没有」）、`NewSolcoLinkAt`（测试用：固定包路径与执行函数，不会启动 Solco）。
- `solco_link_test.go`：链接形状（首条字面量与 Solco 的 `TestParseTerminalURL` 各钉一次）、查找顺序 / 缓存 / 重新检查 / 非 macOS / 非 .app、打开用的包与链接、先拒绝后问系统。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
