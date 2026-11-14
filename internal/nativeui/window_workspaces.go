package nativeui

/**
 * [INPUT]: 依赖 window.go 的工作区清单与动作（切换并刷新/移除确认/排序/添加）、quick_launch.go 的 quickItems/quickRow（同一套搜索与行）、chrome_parts.go 的 toggleMenu/crumbItemRow、mygo/ui 的 Popover/Scroll/ContextMenu
 * [OUTPUT]: 对外提供 Window.workspaceSwitcher（侧栏页脚的工作区芯片，Solco 页脚设备芯片的样子：文件夹字形（有草稿/在跑时带警示点）+ 名字 + 下箭头，铺满一行，下拉向上弹出；始终是当前工作区）、Window.workspaceChip（侧栏收起时才出现在标题带里的同一个下拉，样式同过去被选中的标签，工作区永远够得着）、workspacePopover（两颗芯片共用一份下拉状态，侧栏滑动的几帧里两颗同屏，只有一颗画下拉——同一滚动状态绑两次会崩）；下拉本体：带搜索、有最大高度、超出滚动的列表——↑↓ 选、↵ 切换并刷新、esc 关，行右键 Show in Finder / Copy Path / Remove from Workspaces，底部 Open Repository… 与 Manage Workspaces…、workspaceLabels（同名仓库带上父目录区分）、workspacesGroup（设置里的工作区管理：列表、↑↓ 排序、移除、Add Workspace…）、removeConfirm（移除有草稿或在跑操作的工作区前的确认）
 * [POS]: 工作区清单的视图：点选走 chooseWorkspace、移除走 requestRemoveWorkspace（含确认）、添加走 addWorkspacePath；自己只保存下拉的开合/查询/高亮，没有工作区状态；「工作区」是 Solco Git 自己的仓库清单，与 Herdr 的 workspace 无关
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

const (
	// workspaceChipMax caps the title band's chip: a long repository name
	// ellipsizes instead of pushing the branch chip and the tools away.
	workspaceChipMax float32 = 260
	// workspacePanelWidth is the pull-down's width: room for a name, its path
	// and the check.
	workspacePanelWidth float32 = 380
	// workspaceListMaxHeight bounds the Settings list: past it the rows scroll
	// inside the group instead of growing the sheet.
	workspaceListMaxHeight float32 = 216
)

// workspacePicker is what the pull-down keeps between frames.
type workspacePicker struct {
	open  bool
	query string
	sel   int
	// scroll is how far the list is scrolled: kept so a new query can start
	// at the top, where its first match is.
	scroll ui.ScrollState
	// reveal asks for the highlighted row to be scrolled into view in the next
	// frame: when the panel opens and when a key moves the highlight — once,
	// since asking in every frame would keep the user from scrolling it away.
	reveal bool
}

// workspaceLabels names each workspace by its folder; two repositories with
// the same folder name are told apart by the folder above.
func (w *Window) workspaceLabels() []string {
	labels := make([]string, len(w.workspaces))
	seen := map[string]int{}
	for i, ws := range w.workspaces {
		labels[i] = pathBase(ws.sh.dir)
		seen[labels[i]]++
	}
	for i, ws := range w.workspaces {
		if seen[labels[i]] > 1 {
			labels[i] += " · " + pathBase(filepath.Dir(ws.sh.dir))
		}
	}
	return labels
}

// workspaceItems is the pull-down's rows for a query: the workspaces,
// filtered and ordered the way Quick Launch orders its open ones (the user's
// order when the query is empty). A typed path is Quick Launch's business.
func (w *Window) workspaceItems(query string) []quickItem {
	open, _ := w.quickRows()
	items := quickItems(open, nil, query)
	kept := items[:0]
	for _, item := range items {
		if !item.Typed {
			kept = append(kept, item)
		}
	}
	return kept
}

// workspaceSwitcher is the sidebar foot's workspace chip, in the face of
// Solco's footer chip (the device and session switcher): the folder glyph —
// with a warning dot while something in the workspace is unsaved or running —
// the name, the chevron, filling the row; its pull-down opens upward. It
// always names the workspace in front.
func (w *Window) workspaceSwitcher(c *ui.Context, k *gorexColors) {
	if w.active < 0 || w.active >= len(w.workspaces) {
		ui.Spacer(c)
		return
	}
	t := c.Theme()
	front := w.workspaces[w.active]
	label := w.workspaceLabels()[w.active]
	var chip ui.Element
	ui.Box(c).Grow(1).MinWidth(0).Children(func() {
		chip = ui.ButtonBase(c.Key("foot-workspace")).FillWidth().Height(Controls().RegularHeight).
			Radius(Radius().Row).Padding(0, 8).Gap(6).AlignItems(ui.Center).Label("Workspace: " + label)
		tipped(c, chip, tildePath(front.sh.dir))
		if chip.Hovered() {
			chip.Background(t.SurfaceHover)
		}
		chip.Children(func() {
			ui.Box(c).Size(14, 14).Shrink(0).Center().Children(func() {
				ui.Icon(c, iconFolder).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).PassThrough()
				if w.workspaceNeedsConfirm(front) {
					ui.Box(c).Absolute().Left(9).Top(7).Size(7, 7).Radius(Radius().Pill).PassThrough().
						Background(designTokens(t.Dark).StatusColor(ToneWarning, t.Dark))
				}
			})
			ui.Text(c, label).FontSize(Typography().Caption).TextColor(t.Text).
				SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			ui.Box(c).Grow(1).PassThrough()
			ui.Icon(c, iconChevronDown).Size(Controls().IconSmall, Controls().IconSmall).TextColor(t.TextMuted).Shrink(0)
		})
	})
	// While the sidebar slides shut the band's chip is already there beside
	// this one; the pull-down belongs to the band's from that moment.
	w.workspacePopover(c, k, chip, !w.sidebarCollapsed)
}

// workspaceChip is the title band's workspace pull-down, shown only while the
// sidebar is shut (the foot chip is hidden with it), so the workspace is never
// out of reach. It keeps the face the selected tab used to have: a dot while
// something in it is unsaved or running, the name, the chevron.
func (w *Window) workspaceChip(c *ui.Context, k *gorexColors) {
	if w.active < 0 || w.active >= len(w.workspaces) {
		return
	}
	t := c.Theme()
	front := w.workspaces[w.active]
	label := w.workspaceLabels()[w.active]
	chip := tipped(c, ui.ButtonBase(c.Key("band-workspace")).Height(Controls().RegularHeight).
		Padding(0, Spacing().S, 0, 10).Gap(7).Radius(Radius().Row).AlignItems(ui.Center).
		Shrink(1).MinWidth(0).MaxWidth(workspaceChipMax).Label("Workspace: "+label), tildePath(front.sh.dir))
	gorexToggleFace(chip, k, true)
	chip.Children(func() {
		if w.workspaceNeedsConfirm(front) {
			ui.Box(c).Size(6, 6).Radius(Radius().Pill).Shrink(0).PassThrough().
				Background(designTokens(t.Dark).StatusColor(ToneWarning, t.Dark))
		}
		ui.Text(c, label).FontSize(Typography().Section).FontWeight(WeightSemibold).TextColor(t.Text).
			SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).PassThrough()
		ui.Icon(c, iconChevronDown).Size(Controls().IconSmall, Controls().IconSmall).TextColor(t.TextMuted).Shrink(0).PassThrough()
	})
	w.workspacePopover(c, k, chip, true)
}

// workspacePopover opens the pull-down on a click of either chip and draws it
// on the owner: one picker state, so the two chips are one control. Both chips
// are on screen while the sidebar slides, and two popovers on one state would
// bind its scroll twice in one build — so exactly one chip draws it.
func (w *Window) workspacePopover(c *ui.Context, k *gorexColors, chip ui.Element, owner bool) {
	p := &w.picker
	wasOpen := p.open
	toggleMenu(&p.open, chip)
	if p.open && !wasOpen {
		// A fresh opening starts on the workspace in front, scrolled to it.
		p.query, p.sel, p.reveal = "", w.active, true
	}
	if p.open && owner {
		ui.Popover(c, chip, &p.open, func() { w.workspacePanel(c, k) })
	}
}

// workspacePanel is the pull-down's body: a search field, the workspaces in a
// list that scrolls past workspaceListMaxHeight (↑↓ move, ↵ switches), and two
// footer rows.
func (w *Window) workspacePanel(c *ui.Context, k *gorexColors) {
	p := &w.picker
	t := c.Theme()
	sp := Spacing()
	items := w.workspaceItems(p.query)
	p.sel = clamp(p.sel, 0, max(len(items)-1, 0))
	reveal := p.reveal
	p.reveal = false
	ui.Column(c).Width(workspacePanelWidth).Padding(sp.S).Gap(sp.XS).Children(func() {
		field := searchField(c, &p.query).FillWidth().AutoFocus().Label("Search workspaces")
		if field.Changed() {
			p.sel, p.scroll.Y = 0, 0
		}
		if field.Shortcut(0, ui.KeyDown) {
			p.sel, p.reveal = min(p.sel+1, max(len(items)-1, 0)), true
		}
		if field.Shortcut(0, ui.KeyUp) {
			p.sel, p.reveal = max(p.sel-1, 0), true
		}
		if field.Submitted() && len(items) > 0 {
			p.open = false
			w.chooseWorkspace(items[p.sel].WorkspaceID)
			return
		}
		ui.Scroll(c).TrackScroll(&p.scroll).FillWidth().MaxHeight(switcherListMaxHeight).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for i, item := range items {
					item := item
					row := w.quickRow(c, k, item, p.query, i == p.sel, "workspace")
					w.dragWorkspaceRow(c, row, item.WorkspaceID)
					if reveal && i == p.sel {
						row.ScrollIntoView()
					}
					if row.Clicked() {
						p.open = false
						w.chooseWorkspace(item.WorkspaceID)
					}
					row.ContextMenu(func(m *ui.Menu) { w.workspaceMenu(m, item.WorkspaceID) })
				}
				if len(items) == 0 {
					ui.Text(c, "No matching workspaces").FontSize(Typography().Caption).
						TextColor(t.TextMuted).Padding(sp.S)
				}
			})
		})
		ui.Divider(c)
		if crumbItemRow(c, iconFolderOpen, "Open Repository…", false).Clicked() {
			p.open = false
			w.chooseRepository()
		}
		if crumbItemRow(c, iconSettings, "Manage Workspaces…", false).Clicked() {
			p.open = false
			w.openSettings()
		}
	})
}

// workspaceDrag is what a dragged workspace row carries.
type workspaceDrag struct{ ID int }

// dragWorkspaceRow makes row (the workspace id) both a handle and a target of
// the list's order: drag it onto another row to take that row's place. The
// row under a drag wears the accent wash, the one being dragged dims.
func (w *Window) dragWorkspaceRow(c *ui.Context, row ui.Element, id int) {
	row.Drag(workspaceDrag{id})
	if d, over := ui.DragOver[workspaceDrag](row); over && d.ID != id {
		row.Background(c.Theme().Accent.Alpha(0.16))
	}
	if row.Dragging() {
		row.Opacity(0.5)
	}
	if d, ok := ui.Drop[workspaceDrag](row); ok {
		w.dropWorkspace(d.ID, w.indexOfID(id))
	}
}

// workspaceMenu is a workspace row's context menu. Removing only takes the
// repository out of the list; the folder on disk is untouched.
func (w *Window) workspaceMenu(m *ui.Menu, id int) {
	i := w.indexOfID(id)
	if i < 0 {
		return
	}
	sh := w.workspaces[i].sh
	if m.Item("Show in Finder").Chosen() {
		sh.openExternalPathParent(sh.dir)
	}
	if m.Item("Copy Path").Chosen() {
		sh.copyToClipboard(sh.dir)
	}
	m.Separator()
	if m.Item("Remove from Workspaces").Chosen() {
		w.picker.open = false
		w.requestRemoveWorkspace(w.indexOfID(id))
	}
}

// removeConfirm asks before a workspace holding a commit message or a running
// Git operation is removed from the list. Cancel is the last button so Enter
// dismisses safely.
func (w *Window) removeConfirm(c *ui.Context) {
	if w.removeAsk == 0 {
		return
	}
	i := w.indexOfID(w.removeAsk)
	if i < 0 {
		w.removeAsk = 0
		return
	}
	id, sh := w.removeAsk, w.workspaces[i].sh
	message := "Its Git operation is still running; removing the workspace does not stop it."
	if sh.commitDraftPresent() {
		message = "The commit message you started in it will be lost."
	}
	if sh.term.askBusy {
		message = "A process is still running in its terminal; removing the workspace ends it."
	}
	if n := sh.dirtyFileName(); n != "" {
		message = "Your unsaved edits in “" + n + "” will be lost."
	}
	open := true
	if ui.AlertDialog(c, &open, "Remove “"+pathBase(sh.dir)+"” from Workspaces?", message, "Remove Workspace", "Cancel") == 0 {
		w.removeAsk = 0
		w.removeWorkspace(w.indexOfID(id))
		return
	}
	if !open {
		w.removeAsk = 0
	}
}

// workspacesGroup is Settings → Workspaces: the list the pull-down shows, in
// its order, each row with its move and remove controls (the list scrolls past
// workspaceListMaxHeight), and the way to add one. Adding only joins the list;
// every other way of opening a repository joins it too and shows it.
func (w *Window) workspacesGroup(c *ui.Context) {
	t := c.Theme()
	sp := Spacing()
	tokens := designTokens(t.Dark)
	k := gorexColorsOf(t.Dark)
	labels := w.workspaceLabels()
	hairline := func() {
		ui.Row(c).FillWidth().Height(1).Shrink(0).Children(func() {
			ui.Box(c).Width(rowInset).Shrink(0)
			ui.Box(c).Grow(1).FillHeight().Background(tokens.BorderSubtle)
		})
	}
	ui.Column(c).FillWidth().Gap(sp.S).Children(func() {
		groupLabel(c, "Workspaces")
		ui.Column(c).FillWidth().Radius(Radius().Card).Background(tokens.Panel).
			Border(1, tokens.BorderSubtle).Clip().Children(func() {
			if len(w.workspaces) > 0 {
				ui.Scroll(c).FillWidth().MaxHeight(workspaceListMaxHeight).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for i, ws := range w.workspaces {
							if i > 0 {
								hairline()
							}
							w.workspaceManageRow(c, k, i, ws, labels[i])
						}
					})
				})
				hairline()
			}
			formRowLead(c, nil, "Add workspace", "Choose Git repositories, or folders inside them.", func() {
				if regularButton(c, "Add Workspace…").Clicked() {
					w.chooseWorkspacesToAdd()
				}
			})
		})
	})
}

// workspaceManageRow is one Settings row, draggable: the folder, its path, and
// the move up / move down / remove buttons.
func (w *Window) workspaceManageRow(c *ui.Context, k *gorexColors, i int, ws *workspace, label string) {
	id := ws.id
	row := ui.Column(c.Key("workspace-manage:" + itoa(id))).FillWidth()
	row.Children(func() { w.workspaceManageFace(c, k, i, ws, label) })
	w.dragWorkspaceRow(c, row, id)
}

func (w *Window) workspaceManageFace(c *ui.Context, k *gorexColors, i int, ws *workspace, label string) {
	t := c.Theme()
	id := ws.id
	formRowLead(c, func() {
		ui.Icon(c, iconFolder).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).Shrink(0)
	}, label, tildePath(ws.sh.dir), func() {
		ui.Row(c.Key("workspace-row:" + itoa(id))).Gap(2).Shrink(0).AlignItems(ui.Center).Children(func() {
			up := gorexIconButton(c, k, iconArrowUp, "Move "+label+" up", "Move Up", false, gorexRowBtn, gorexRowGlyph)
			down := gorexIconButton(c, k, iconArrowDown, "Move "+label+" down", "Move Down", false, gorexRowBtn, gorexRowGlyph)
			remove := gorexIconButton(c, k, iconTrash, "Remove "+label, "Remove from Workspaces", false, gorexRowBtn, gorexRowGlyph)
			up.Disabled(i == 0)
			down.Disabled(i == len(w.workspaces)-1)
			switch {
			case up.Clicked() && i > 0:
				w.moveWorkspace(id, -1)
			case down.Clicked() && i < len(w.workspaces)-1:
				w.moveWorkspace(id, 1)
			case remove.Clicked():
				w.requestRemoveWorkspace(w.indexOfID(id))
			}
		})
	})
}
