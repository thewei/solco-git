# nativeui/
> L2 | 父级: ../../CLAUDE.md

Solco Git 的全部呈现层；每个文件开头的 L3 头（`[INPUT]/[OUTPUT]/[POS]`，或首段说明）是该文件的完整说明，这里只按职责分组、一行一组。Git 与 Files 的数据读取与变更全部走 `solco-gitkit`，这里不跑第二条 `git` 路径，也没有 Herdr/Pane/终端（「工作区」只是本应用自己的仓库清单）。

成员清单（按职责分组）

窗口与工作区（Solco Git 自己的，Solco 里没有；「工作区」是本应用自己的仓库清单，与 Herdr 的 workspace 无关）
- `window.go`：Window 状态机——工作区清单（同一仓库根只有一个）、移除确认（`removeAsk`）、偏好与最近列表、待处理链接队列、设置落盘与广播；每一种打开方式（⌘O、Quick Launch、拖放、`solcogit://` 链接、`open -a`、Recent、worktree）都收敛到 `OpenRepository`：新的就加入清单，并显示。
- `window_workspaces.go`：工作区清单的视图——侧栏页脚的工作区芯片（Solco 页脚设备芯片的样子，始终是当前工作区；点开向上弹出带搜索、有最大高度、超出滚动的列表，↵ 切换并刷新；侧栏收起时同一个下拉改在标题带里，两颗芯片只有一颗画下拉）、`workspaceLabels`（同名加父目录）、设置里的工作区管理（↑↓ 排序、移除、Add Workspace…）、移除有草稿或在跑操作的工作区前的确认。
- `window_nav.go`：Back / Forward——（工作区，视图）位置的历史栈（`navHistory`，每帧由 `Window.View` 记录，所以每条导航路径都进历史；上限 64、移除工作区时折掉它的位置、不落盘），侧栏带右缘的 Back/Forward 一对（Solco 同位同形，无处可去时是禁用脸；侧栏收起后落在侧栏钮旁）与 View 菜单的 ⌘[ / ⌘]；Commit 编辑器未收起时和其他导航一样被拦住。
- `window_view.go`：窗口的单一帧入口：标题带（分支 + 工具；侧栏收起时才多出工作区下拉）+ 活动工作区的工作台；侧栏宽度属于窗口；侧栏钮与 Back/Forward 两组悬浮钮。
- `window_menu.go`：菜单栏（File / Repository / View / Window）的单一来源，只调已有方法；⌃⇥ / ⌃⇧⇥ 是 Window 菜单的可见项（AppKit 不把 Control-Tab 交给视图，Hidden 项的键位也不触发）。
- `window_dialogs.go`：不属于任何仓库的浮层（设置、移除工作区确认）。
- `welcome.go`：零工作区页：复用资源包嵌入的应用图标，显示 Open Repository…（⌘O）、最近列表与拖入提示。
- `quick_launch.go`：快开浮层的壳（Files / Text / Repos 三标签，切标签保留输入）与 Repos 标签：⇧⌘O，已开工作区与最近仓库的筛选列表（工作区下拉复用它的行与匹配）。
- `terminal_drawer.go`：底部终端抽屉（⌃`，每个工作区一个用户登录 shell，工作目录 = 仓库根；官方 plugins/terminal，库在 Contents/Resources）：Restart / Close 在有进程运行时先问，移除工作区同；用 sh 包装写下 shell 的 pid 来判断「有进程在跑」（插件没有这个接口）；本应用自己的本地 shell，不是 Herdr、不持久化、不记终端字节；`openTerminal` 按设置「终端归属」分流：In Solco Git 开抽屉，In Solco 经 `platform.SolcoLink` 把仓库根交给 Solco（没装就说明、不回退）（2026-11-13 决定第 (5) 条）。
- `quick_open.go`：快开的 Files（⌘P，子序列排名、空输入列最近打开的）与 Text（⇧⌘F，内容搜索、停顿后才起进程）两标签：↑↓ / ↵ 打开（Text 落在命中行）、右侧只读预览（读前 256 KiB）；文件索引与侧栏搜索共用，不需要 fzf（2026-11-13 决定第 (3) 条）。
- `external_open.go`：`solcogit://open?repo=&view=&file=` 与 `open -a` 的唯一入口（解析、校验、不记日志）。
- `shortcuts.go`：键位单一来源——菜单里列出的键就是绑定的键（⌘B 是唯一的窗口内快捷键，其余都是菜单加速键）。
- `shell.go` / `shell_tab.go`：一个工作区 = 一个 Shell（Git + Files 所需的那部分状态）及其作为窗口成员的生命周期接缝；`view.go` 是工作区内的三个视图（Changes / Commits / Files），没有页面栈。
- `frame.go`：三栏版面（侧栏｜标题带｜舞台）落点，侧栏主体按视图加 key；`viewTabs` 是直接铺在侧栏材质上的三行视图开关（2026-11-12 起，原页签条已退役）：一行一个图形加名称，选中的是整行铺满的侧栏选中色（`materialNavGroup` / `materialNavRow`，行高与圆角同树行），Changes 的计数贴行尾，行下一条细线，仍是名为 View 的单选组；侧栏页脚 `sidebarFoot` = 工作区芯片 + Settings（原来的 "Fetched … ago" 搬进 Fetch 钮的提示 `fetchTip`）。
- `boundary.go`：从 Solco 复制来的代码原本跨出去调用的几个小边界，在这里用本应用的语义落地。

Git（复制自 Solco，随 `solco-gitkit` 演进）
- `git_service*.go`：Shell 一侧对 `gitworkbench` 的编排——快照缓存与后台刷新、提交事务、变更操作。
- `git_backend.go`：装到每个 `Runner.Backend` 的那一个 LazyGit `engine.Engine`（`SOLCOGIT_GIT_BACKEND=argv` 回到纯 argv 路径）；目前接管的能力见 solco-gitkit `engine/CLAUDE.md`。
- `git_commit_graph.go`：Commits 的路线图（2026-11-12 起整体换成 TanGit / LazyGit 的画法，无开关）：LazyGit getNextPipes 的管道集布局经 TanGit 移植（车道向左收拢），VS Code 风格画法——两像素线贴设备像素、底色光晕、三次贝塞尔 S 弯、带环圆点；槽宽有上限，每行文字起点随该行用到的车道；引用芯片仍用 `git_commit_chips.go` 的 `gitLaneColors`，车道色是自己的 `gitGraphColor`。
- `git_*.go`、`gd_*.go`、`surface.go`、`commit_surface.go`、`changes_panel.go`、`sidebar*.go`：Changes / Commits 的评审面（diff 行、行级暂存、图谱、ref 树、Stash/Reflog/补丁对话框、sequencer 状态条）。
- `git_undo.go`：Undo / Redo 的界面一侧（Repository 菜单，⌥⌘Z / ⇧⌥⌘Z；⌘Z 是 Edit 菜单的文本撤销，框架把它交给有焦点的输入框，视图收不到，所以不占用）：先读计划再出确认框写明将被撤销的动作，确认的是它的身份串，期间 HEAD 被别人动了就拒绝；底层是 solco-gitkit `reflog_undo.go`。
- `git_recovery.go`：Recovery 面板（Repository ▸ Recovery…，2026-11-12 决定第 (9) 条）：列出 Safety Journal 里可找回的丢弃 / 删除 / 删分支 / drop stash / Drop All / 应用补丁，每行 Recover（找回前内核重新校验，文件又变了就拒绝）；丢弃、删除、删分支、drop stash、Drop All 的确认框和应用补丁的文件选择框不再写「无法撤销」而指向这里；底层是 solco-gitkit `journal.go`。
- `git_conflict.go`：冲突编辑界面（Resolve Conflicts…：冲突文件右键菜单与合并横幅里的 Resolve…，2026-11-12 决定第 (10) 条）：整页弹层，每个冲突块一张卡（Current | Incoming 两栏、Base 勾选展开、Current / Incoming / Both / Base 四个选择、上下文只留块前后两行），下方可手改的 Result（MyGo editor），Apply & Stage；选块会重写 Result，Result 被手改后块按钮停用、出现 Reset Result，残留标记时 Apply 停用；不能逐行合并的文件只给原因与整文件选择；底层是 solco-gitkit `conflict.go`。
- `git_command_log.go`：命令日志界面（Repository ▸ Command Log…，2026-11-12 决定第 (11) 条）：本仓库这次运行里做过的 git 命令，新的在前、默认不含读（Show reads 勾选）、点一行复制、Copy All（旧到新，可当脚本）、Clear 只清本仓库；日志是全应用共享的内存 `sharedCommandLog`，Runner 与共享 Engine 同写，不落盘、不进 applog。
- `git_history_ui.go`：历史改写界面（2026-11-12 决定第 (12) 条）：提交列表 ⌘ / ⇧ 多选（`pickCommit`）、右键菜单的 Squash / Fixup / Drop / Move Up·Down / Reword、Amend Staged Changes Into This Commit…、Cherry-pick N Commits…；一切先弹「预览」表（`PreviewHistory` 的结果，已推送的有警告、提示 ⌥⌘Z 可退回）再执行，冲突由内核中止并还原。
- `git_patch_ui.go`：Custom Patch 界面（同一条决定）：提交审阅里整文件（文件菜单 Add File to Custom Patch）与按行（点行号，⇧ 连选、⌘ 追加、拖过；hunk 行 Add Hunk to Patch）两种选择，选择记在界面坐标里、执行时才用 `PatchSource` + `PatchLinesFor` 换成 Git 的行号（对不上即拒绝）；审阅顶部的 Custom Patch 条（Remove / Split / Move / Undo / Clear，Move 先选目标提交）；合并提交、stash、不在当前分支的提交不可用；换提交即清空选择。
- `git_courier_ui.go`：Change Courier 界面（2026-11-12 决定第 (13) 条）：Send to Worktree 面板——来源是 Changes 的文件菜单（整文件）、已选行 hunk 行上的 Send…、提交审阅 Custom Patch 条上的 Send…；目标是同一仓库的其它 worktree（单选列表，换目标即对新目标重新预检，旧答案作废）；预检不干净时给 Git 的原话且 Send 不可点；Move 复选框：成功后从源丢弃（工作区整文件 / 暂存之外的行 / 提交里 Remove），有暂存的文件或行不提供并说明；补丁面板打开时一次做好，预检与送达用同一份文本。
- `gd_moved_test.go` / `gd_diffstat.go` / `git_stash_lines_ui.go` / `git_commit_filter_ui.go`：2026-11-12 决定第 (14) 条的四项界面——移动行高亮（`gdLineCell` 用移动色而非增删色、悬停提示 “Moved to / from line N”，结果按 `gdFile` 记住、随 `resetDerived` 清空；View 菜单 “Highlight Moved Lines” 开关，默认开，设置里存反向的 `hide_moved_lines`）、文件行上 +N −N 旁的五格比例条（`gdStatBlocks`：有新增必有绿、有删除必有红、同数同格）、行级 stash（hunk 行上的 “Stash N Lines…”，N 是该文件里选中的全部行；问一句预填的说明；已暂存 / 未跟踪 / 暂存侧不可用并说明原因；对话框开着时仓库换了就拒）、提交过滤（列表头的 Filters 弹层：Path / Since / Until / Current branch only，点 Apply 才由 Git 查，结果在 `cfilter.commits`；文字搜索在其内再筛；筛选中的列表只画圆点；仓库刷新时重查，换仓库与 reveal 时清除）。
- `commit_tree.go`：Fork 的 File Tree——提交评审里 Diff | Tree 切换，按提交看整棵树（读 `ListTree`/`ReadBlob`，只读）。
- `multi_push.go`：Push Multiple…——一次推多个分支与标签（`ReadPushOffer`/`PushMany`），分叉的标为受阻，永不强推。
- `image_compare.go`：图片改动的 2-up / Swipe / Onion 对比，画在已解码位图上。

Files
- `page_files.go`、`right_panel*.go`、`files_*.go`：Files 视图——忽略感知的树、原生预览分级（2026-11-08「Files preview: native first」，与 Solco 同一份：`files_markdown.go` 的 `planPreviewTier` 把文本/代码交 `ui/editor`、≤ 2 MiB 的 Markdown 交 MyGo `ui/markdown`（Rendered | Source 一键切换，默认 Rendered；长文档由 `files_markdown_rows.go` 分行，`files_markdown_find.go` 管 ⌘F 查找，`files_markdown_html.go` 把原始 HTML 译成 Markdown 块，`files_markdown_rich.go` 画数学与 Mermaid）、≤ 1 MiB 的 SVG 画出来（Image | Source）、PNG/JPEG/GIF/WebP/BMP 原生解码；只有 AVIF/ICO 经 `files_webview.go` 里 Go 现拼的几百字节页面，底色 `filesWebviewFill` = 窗口画布）、起始页、搜索、文件操作；Quick Look / Copy Path / Reveal in Finder 由 `files_ops.go` 的 `filesPathItems` 给所有文件表面（树行、正文右键、Actions 菜单）。
- `files_edit.go`（2026-11-13 决定第 (1)(2) 条，E1）：Files 里的就地编辑——文件默认只读，头部铅笔图标钮开关编辑（`fileEditState`，随预览文件重置）；⌘S / Save 原子写盘（同目录临时文件 + 重命名，保留权限与换行，符号链接写到仓库内的目标）；Revert / Reload from disk；有未保存编辑时切文件、做会写工作区的 Git 操作（`dirtySafeOps` 之外的都问）、退出、移除工作区前先问（Save and Continue / Discard Edits / Cancel）；重命名 / 删除含脏文件的路径直接拒绝；`syncEditedFile` 随每次快照比对磁盘：干净缓冲区静默重载，脏缓冲区只出 Reload from disk / Keep mine 条且 Save 拒绝。编辑器自己不跑 git；截断 / 非 UTF-8 / 非文本不可编辑；未实现：自动保存设置、Compare…、另存为、LSP、⌘P、终端（E2–E5）。
- 快开的 ⌘↵ / ⌥↵（2026-11-13 用户定：⌘↵ = 在 Files 树里定位，⌥↵ = 在 Finder 里显示，↵ 仍是定位并打开）：`Shell.locateFile` 切到 Files、展开上级目录、把「Changed」切回「All」、清掉树搜索，`rightPanel.located` 让该行穿更强的选中色（`sidebarTreeSpec.Located` → `SidebarSelectStrong`，与已打开文件的柔和选中色区分；定位到的恰是打开的文件时仍是后者）、`locateScroll` 让它滚进视野一次；打开任何文件都会清掉 `located`；⌥↵ 走 `openExternalPathParent`。
- `quick_open_match.go`（E2 补）：快开结果行的匹配高亮——重放 `filesview.MatchFiles` 的逐词规则（名字前缀 / 名字子串 / 路径子串 / 名字子序列 / 路径子序列）得出命中的字节区间，Files 行标出名字与目录里命中的字符、Text 行标出命中行里的查询，颜色同 Markdown 查找标记；大小写折叠改变字节数的路径不标。
- `files_edit_more.go`（E1 遗留）：自动保存（`Workbench.AutoSave`，默认关；每帧 `autoSaveTick` 看缓冲区版本，停手 1 s 后 `autoSaveFire` 静默写盘，Git 操作进行中顺延重试，磁盘被改写 / 文件已不在时不写）；Compare…（磁盘被改写的横条上第三个按钮：把磁盘文本放进编辑器的 `DiffBase`，槽沟标出差异，Done / Keep mine / Reload 之后还原 HEAD 的槽沟；不自造 diff 视图）；Save As…（⇧⌘S 与「文件已不在」横条上的按钮，原生保存面板 → `saveAsTo`：仓库内写好后刷新 Files 树并打开它（面板给的路径可能经符号链接：`underRoot` 连候选路径一起解析，按仓库里的真实路径打开）、旧缓冲区算已存，仓库外只存副本、缓冲区不动，同一路径把被 Git 删掉的文件写回；写盘用 `writeAtomicOpt(create)`）。
- `files_lsp.go`（2026-11-13 决定第 (4) 条，E4）：Files 里的语言服务器只读能力——每个工作区一个 `lsp.Manager`（`lspState`，Shell.Close 时有序结束）；缓冲区是真相：每帧比一次 `Buffer.Version`，变了等 250 ms 静止把整份文本交给 Manager；发给服务器的版本 = 缓冲区版本 + 本文档的基数（新缓冲区——换文件、整个重建——基数抬高，LSP 版本永远递增）；诊断带版本回来 → `SetDiagnostics`（过期的由编辑器自己挪）；`OnHover` / `OnDefinition` 在 UI 道发起、请求在别处跑、结果回 UI 道并核对缓冲区版本；定义在仓库内走 `revealFileAt`、仓库外只 toast 一句；页脚一行（没装 / 启动中 / 反复崩溃后关闭 / 运行时的错误与警告计数，点它跳到下一个问题）；Settings 的语言服务器清单（`languageServers`，5 s 缓存，测试可固定）。
- `files_lsp_edit.go`（E5）：语言服务器的写能力——补全（`OnCompletion`，请求前先把缓冲区此刻的文本交给服务器，答案带版本、文本已变就丢；编辑器一次替换一段，取多数条目共有的起点、其余丢掉）、格式化（缓冲区里的一步撤销、不写盘；⌥⇧F 或右键；macOS 会在按键之后再把 ⌥ 字符 “Ï” 当文本送进来，`lspState.dropText` 让它被吞掉，否则缓冲区版本变了、格式化答案被当作过期丢弃）；服务器的改动（格式化、重命名对打开缓冲区）落下后关掉补全列表（`applyServerEdits` 里 `ShowCompletions(0, nil)`），列表的起点与过滤属于被改写前的文本、重命名（F2 或右键 → 共用文本对话框 → 服务器答复 → 先检查：会碰仓库外的文件一个也不写；打开的文件有未保存编辑就弹 Save All and Rename / Cancel；应用时先读好所有已关闭文件再写，已关闭的走 `writeAtomic` 原样字节（保留 CRLF），打开的走缓冲区一步撤销、保存仍归用户）；服务器没有该能力就不进菜单也不发请求；只在编辑模式。保存时自动格式化（`Workbench.FormatOnSave`，默认关，Settings → Language servers 里的开关）：只管 ⌘S 与 Save 按钮（`saveRequested`），先格式化、答复后再写；服务器慢（5 s）/失败/没有该能力/文本在等待中又变了，都照原样保存、从不拦住保存；「改前先保存」的几张表仍按缓冲区原样写。
- `files_git.go`：Files 比 Solco 多出的 Git 层——Changed | All、M/U 标记与文件夹点、槽边改动条（对照 HEAD）、Plain | Blame、History、底栏图例；全部读自 git 快照/blame/cat-file。

UI kit（复制自 Solco；`scripts/kit-parity.sh` 列出与 `../solco` 的差异）
- `theme.go`、`design_system.go`、`ds_components.go`、`components.go`、`controls.go`、`gorex_style.go`、`icons.go`、`tooltip.go`、`motion.go`、`constants.go`、`list_nav.go`、`dialogs.go`、`code_theme.go`、`chrome_parts.go`、`script_actions.go`、`workbench_ui.go`、`page_common.go`、`page_settings.go`：token、控件、图标、提示、动效、列表键盘导航、设置页等共享部件。改视觉先改 token 或共享默认，不在业务组件上 inline 覆盖。侧栏没有自己的底色（vibrancy 透出来），放在它上面的控件（视图三行、Files 的 Changed | All、搜索的 Files | Content）一律用 `ds_components.go` 的 `materialFace` 一族——选中是侧栏选中色的一层淡墨、悬停是侧栏悬停色，不是 `segmentedTrack` 那种不透明的白槽；`segmentedTrack` 只留给画布上的控件。

测试（无原生窗口，直接驱动 Window/Shell 模型；后台结果进队列，由测试线程排空）
- `testutil_test.go`：共享夹具（真实临时仓库 `gitFixture`、`newTestWindow`、`uiPump`、`settle`、`saveEvidence`）。
- `window_nav_test.go`：Back/Forward 的合同——历史栈（记录、去重、回退后新去处丢前方、上限、移除工作区折叠）、跨工作区与视图往返且不自己再写历史、Commit 编辑器拦住、按钮真点、位置（侧栏带右缘 → 收起后落在侧栏钮旁，内容让位）、View 菜单随历史启停。
- `window_foot_test.go`：页脚工作区芯片在侧栏最下方并向上弹出、收起时才在标题带里、侧栏滑动的几帧里只有一颗芯片画下拉（否则同一滚动状态绑两次会崩）、Fetch 钮提示带 "Fetched … ago"。
- `window_test.go`：工作区/最近/外部链接（含 view=changes 带文件时在 Changes 里选中）/Quick Launch/设置规范化的回归；首帧的分支芯片不说 Detached HEAD；Theme 偏好映射到框架的 theme source。
- `files_markdown_test.go`：预览原生优先合同——每种文件的分级归属、Markdown 渲染⇄源码切换且不重读（Source 仍带对 HEAD 的槽边）、搜索命中落在 Source、链接只放行该放行的、图片只读仓库里且有像素预算、主题随外观只换一次、AVIF/ICO 整页只有几百字节且底色 = 窗口画布、设置里没有旧的 Rich file preview 开关；设置 SOLCOGIT_EVIDENCE 时落下 Markdown 渲染的 PNG；各条都做过变异检查。
- `log_privacy_test.go`：日志合同——打开仓库、跑一次 Git 操作、收到一条被拒的链接后，默认级别的日志有操作名与工作区数，没有仓库路径/目录名/文件名/链接参数（抓取本身也被断言）。
- `quick_commit_test.go`：空闲提交条上第一次点击 Commit 就打开提交对话框（按钮 KeepFocus；去掉则红：按下展开表单、按钮换了父级，松手落空）。
- `staged_pane_test.go`：Staged 列表的高度——分隔条真的改它（旧的 Height 被 Grow 的 0% basis 吞掉，两个列表等分）、一个暂存文件不把 Unstaged 行挤到 Staged 头后面。
- `view_tabs_test.go`：侧栏三行视图的合同——Changes / Commits / Files 自上而下三行、整行宽、名称在 200/252/400 三种侧栏宽度下都画出；Changes 的三位数计数贴行尾且完整；选中是侧栏选中色的一层淡墨整行铺满、不是白纸（明暗两种外观），Files 树的 Changed | All 与搜索的 Files | Content 同样；点、方向键、悬停淡墨与提示（名称 + 计数 + ⌘1–3）。
- `git_commit_graph_test.go`：路线图的合同——布局（线性、分叉/合并、车道收拢、多尖端与窗口外的父、分支线色沿线不变）、几何有界（槽宽不超上限、每行宽度随该行用到的车道）、画法（点与线真的画在车道上、车道之外是纸）、真实合并仓库经真实读取后画成两车道并渲染；变异（丢颜色传递、关掉收拢、合并标记）各自变红已核对。
- `git_undo_test.go`：先问后动（确认前 HEAD 不动）、确认期间 HEAD 被动过则拒绝、没有可撤销时只给提示、Repository 菜单两项带 ⌥⌘Z 系加速键（不占 Edit 菜单的 ⌘Z）且各开对应确认。
- `git_recovery_test.go`：丢弃 + 删除后列表有对应条目且新的在前、Recover 放回内容、确认框指向 Recovery、Repository 菜单有 Recovery…、删掉的分支与 drop 的 stash 一并找回、应用的补丁与清空的 stash 一并找回。
- `git_conflict_test.go`：真实合并冲突上的打开、点 Incoming 改写 Result 且 Apply 前不落盘、手改锁块与 Reset Result、Apply 暂存、残留标记时不能 Apply、一方删除只给整文件选择、Recovery 重开冲突；`SOLCOGIT_EVIDENCE` 保留 PNG。
- `git_command_log_test.go`：丢弃后表里有写命令且默认不含读、Show reads 后读也出现、Copy All 旧到新每命令一行、Clear 只清本仓库、Repository 菜单有 Command Log…；SOLCOGIT_EVIDENCE 落 PNG。
- `git_history_ui_test.go`：先预览再改写（Drop / Squash / Fixup / Move / Reword / Amend / Cherry-pick N）、取消不动仓库、预览拒绝成 toast、已推送警告、冲突的改写被中止且仓库原样、改写可由 Undo 退回、⌘ / ⇧ 多选规则；SOLCOGIT_EVIDENCE 落 PNG。
- `git_patch_ui_test.go`：选择规则（点选 / ⌘ / ⇧ / 整文件被行选收窄 / 换提交清空）、合并 / 他分支提交不可用、Remove / Split / Move / Undo 的结果与选择一致、改名文件的按行移除、冲突与对不上时仓库原样；每条规则做过变异检查。
- `git_courier_ui_test.go`：文件送达后在目标里是未暂存改动、源不变；Move 整文件（含未跟踪）与行；有暂存的文件 / 行可送不可移；对不上时不能送；换目标重新预检；没有其它 worktree 时给原因不开面板；提交里 Custom Patch 的选择送出并 Move（从提交里移除）；每条规则做过变异检查。
- `gd_moved_test.go` / `gd_diffstat_test.go` / `git_stash_lines_ui_test.go` / `git_commit_filter_ui_test.go`：移动行用移动色、悬停提示方向、开关关掉后一个移动色像素都不剩；比例条的格数规则与文件行上真有一条；跨 hunk 选行后点 “Stash N Lines…”，stash 只含选中行、工作区只少这些行，已暂存 / 未跟踪 / 暂存侧不提供并说明原因，过期对话框被拒；Filters 弹层里填路径点 Apply 只剩 Git 答复的提交且画出来的行也只剩它们，文字搜索在其内，当前分支 / 被隐藏分支 / 错误日期保留旧列表 / 历史前进后重查 / reveal 清除。
- `window_menu_test.go`：应用菜单合同——About 之后才是 Check for Updates…（main 接上更新器才有，每次重装菜单拿新菜单项），Settings… 与 Quit 不丢。
- `window_workspaces_test.go`：工作区下拉与管理的合同——chip 始终是当前工作区；搜索（过滤、↑↓、↵ 切换并刷新、无命中时 ↵ 什么也不做）、最大高度与滚动、行右键移除、设置里的排序/移除/Add Workspace…（添加不抢前台）、每一种打开方式都加入清单、旧版 `tabs` 键读成工作区、清单上限、共享图标按钮是 Tab 停靠点。
- `window_landing_test.go`：冷启动经链接落点——首读之后列表顶上是被点名文件的卡，内容分批读到后也不动，过了揭示窗口选中行仍是它。
- `window_polish_test.go`：首读之前侧栏说 Reading references… 而不是五个 0；外部改动直接读到、没有「Files changed on disk」提示；窄卡不画 Viewed 框；提交计数按单复数（1 commit）；欢迎页的两句话各占一行。
- `window_render_test.go`、`files_evidence_test.go`：与设计稿逐屏对照的 PNG 证据（设置 `SOLCOGIT_EVIDENCE` 才落盘）。
- `terminal_solco_test.go`：终端归属的合同——默认是抽屉；选 In Solco 后 ⌃` / 按钮 / View 菜单项把仓库根交给 Solco 且不开抽屉；Solco 没装就说明、不悄悄退回抽屉；设置页的二选一写进偏好、菜单项与按钮随之改；去掉归属判断 / 没装时回退 / 换目录三个变异必红。
- `terminal_drawer_test.go`：shell 在仓库根起、包装层的 pid 就是 shell 的 pid、Close / Restart 无进程直接结束有进程先问（Cancel 不动、确认才杀进程）、移除工作区有进程先问、shell 退出后显示退出码；取不到终端库时整组跳过；去掉询问 / Dir / pid 文件三个变异检查必红。
- `quick_open_test.go`：Files 排名与 ↵ 打开、名字命中先于文件夹命中、空输入列最近打开的与无命中说明、Text 落在命中行与不足两字不搜、切标签保留输入、无工作区时落在 Repos；预览 / 排名 / ↵ 打开做过变异检查。
- `files_edit_test.go`：默认只读与铅笔、原子写盘（CRLF / 权限 / 无残留）、符号链接与仓库边界、写失败不动原文件、Git 写操作与切文件前先问（Cancel / Save / Discard 三条路）、磁盘被改写的三种处理与 Save 自查、不可编辑的文件、退出与移除工作区的确认、重命名 / 删除拒绝；每条规则做过变异检查。
- `files_lsp_test.go`：E4 的合同——打开 Go 文件后行内诊断与页脚计数（点计数跳到问题）、干净文件写「No problems」、没装服务器的文件页脚说一句且不弹错误、编辑静止后整份文本才发出且诊断随之更新、保存前先交文本再通知 didSave、换文件服务器收到 didClose、悬停显示服务器的话且文本已变的过期答案被丢弃、定义在仓库内落到 Files 的那一行那一列（UTF-16 → 字节）而仓库外只说一句、没装服务器时跳转定义有解释、Settings 列出找到 / 没找到的服务器。变异检查：页脚不计数、保存不先交文本、悬停不比版本，对应测试必红。
- `quick_open_test.go` 的 ⌘↵ / ⌥↵ 用例：⌘↵ 关面板、不换已打开的文件、行被标记且目录展开、Changed 切回 All、滚动只做一次，打开别的文件后标记消失；⌥↵ 把那一行的路径交给 Finder 且什么都不打开。变异检查：⌘↵ 改成打开、滚动不清、不切回 All、⌥↵ 改成定位，对应测试必红。另有 `TestTheLocatedRowReadsApartFromTheOpenFile`：定位行与打开文件在像素上不是同一个底色（去掉 `Located` 分支必红）。
- `quick_open_test.go` 的标记用例：每类命中（名字 / 子序列 / 文件夹 / 多词 / 大小写）标出的正是命中处，折叠改变偏移的路径不标，行带着自己的标记并画出来。
- `terminal_drawer_test.go` 的退出用例：空闲 shell 不拦退出、有进程在跑时 ⌘Q 弹「Quit with a running process?」、Cancel 什么都不动、Quit Anyway 才退（`Window.QuitNeedsAsk` / `BusyTerminals`，`main.go` 的 OnBeforeQuit 用它）。
- `files_edit_more_test.go`：自动保存默认关且开了之后静默写盘、Git 忙时顺延、磁盘被改写不写；Compare… 的 DiffBase 与还原；Save As 仓库内 / 仓库外 / 同一路径 / 覆盖已有文件 / 经符号链接指向仓库的路径（也要列进 Files 树）。变异检查：自动保存忽略开关（两处守卫都去掉才红）/ 忽略 Git 忙、Compare 不还原、Save As 仓库内不标已存、`underRoot` 不解析候选路径、Save As 后不刷新 Files 树，对应测试必红。
- `files_lsp_edit_test.go`：E5 的合同——补全列出条目且不列需要 import 的、文本已变的答案丢掉、只读不发请求、格式化一步撤销且不写盘、重命名原样改已关闭文件（CRLF 与整词）且缓冲区一步撤销、有未保存编辑时先弹表（取消什么都不动、保存后一并改）、碰仓库外的重命名不写任何东西、没有能力不发请求、F2 / ⌥⇧F 的路由、⌥⇧F 后紧跟的 “Ï” 文本被吞（去掉 `dropText` 必红）。格式化会关掉打开着的补全列表。变异检查：补全不比版本、重命名不查仓库外、脏缓冲区不弹表、已关闭文件按 LF 写、保存时格式化忽略开关或应用过期的格式化，对应测试必红。
- `gd_shortcuts_test.go`：Changes 视图的单键快捷键（J / K / ⌥Z）在提交主题输入框取走文本时让路——键入 jk 与 macOS 随后送来的 “Ω” 都进输入框、⌥Z 不切换换行；输入框之外 ⌥Z 照常切换。
- `files_git_test.go`、`commit_tree_test.go`、`multi_push_test.go`、`image_compare_test.go`：2026-11-07「Solco Git」决定里 Files 一节、按提交看整棵树、多引用推送、图片对比的回归防线。

[PROTOCOL]: 变更时更新此头部，然后检查 ../../CLAUDE.md
