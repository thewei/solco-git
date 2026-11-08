# platform/
> L2 | 父级: ../../CLAUDE.md

有时限的桌面小助手：用系统默认编辑器打开文件（MyGo 的 `Shell.OpenPath` 按类型打开，`open -t` 的文本编辑器框架没有对应调用）。参数一律 argv，不拼 shell 字符串；超时 `DesktopActionTimeout`（3 s）。

成员清单
- `desktop_actions.go`：`OpenFile` 与超时常量。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
