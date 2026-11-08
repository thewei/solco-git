# codehl/
> L2 | 父级: ../../CLAUDE.md

diff 评审给代码上色用的 token 类别与调色板（codiff 的 Licht / Dunkel 主题）；分词用 MyGo 自己的 `ui/editor` tokenizer，与 Files 代码视图同一引擎，全应用只有一套代码着色。

成员清单
- `codehl.go`：`Class`（token 类别）、按语言分词（`Lines`，跨行状态）、两套调色板。
- `codehl_test.go`：分行与跨行状态、常见语言着色、类别与 token 一一对应、跨 goroutine 安全。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
