# settings/
> L2 | 父级: ../../CLAUDE.md

Solco Git 自己的偏好存储：外观/密度开关、几项工作台装饰（自动抓取间隔、分栏 diff、隐藏的 ref、折叠的合并、ref 树折叠、隐藏 Stash、关闭 diff 的移动行高亮、终端归属 In Solco）、窗口的可恢复状态（工作区的仓库路径与视图、当前工作区、最近列表、侧栏宽度；旧版的 `tabs` 键由 `WindowSettings.Migrate` 读成工作区，保存只写新键）。不含凭据、提交信息、diff 或文件内容；不 import 任何 `solco` 的包（2026-11-07「Solco Git」决定第 4 条）。

成员清单
- `settings.go`：`Settings`/`GeneralSettings`/`WorkbenchSettings`/`WindowSettings`（含 `Workspaces` 与 `Migrate`）、`WorkspaceSettings`、`PaneSizes`（`window.panes`：Staged 列表高、历史列表高、文件导航宽、终端高；0 = 默认，窗口级，不含内容；2026-11-14）、`Default()`、`Store` 接口、`MemoryStore`；`CurrentSchemaVersion` 在字段含义变化时递增。
- `file_store.go`：`FileStore`——`settings.json` 原子写（临时文件 + rename，0600），缺文件 = 首次启动（默认值），坏 JSON 或更新的 schema = 默认值 + 错误（调用方记日志），保存别的 schema 被拒。
- `settings_test.go`：往返、缺文件、不可读文档、无版本旧文档、拒存外来 schema、已取消的 ctx、键白名单（新增字段必须是有意的决定）、旧 `tabs` 迁移成工作区、已退役的 `webview_files_preview` 键读得进写不回、MemoryStore。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
