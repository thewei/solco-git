package nativeui

/**
 * [INPUT]: 依赖 window.go 的工作区/打开/设置、shortcuts.go 的加速键常量、当前工作区的 Git 动作（fetch/pull/push/stash/…）、MyGo 菜单 API
 * [OUTPUT]: 对外提供 MenuBar（macOS 菜单栏：Solco Git / File / Repository / View（Changes/Commits/Files、Back ⌘[ / Forward ⌘]、Toggle Sidebar、Refresh、Show Next/Previous Workspace ⇧⌘] ⇧⌘[、Highlight Moved Lines 开关、Hide Stashes 开关）/ Edit / Window（Minimize、Zoom、Show Next/Previous Workspace ⌃⇥ ⌃⇧⇥）/ Help，结构与键位同设计稿；Solco Git 菜单里 About 之后是 main 经 SetUpdateItem 注入的 Check for Updates…）、refreshMenu（最近列表、是否有仓库、历史能否 Back/Forward、"隐藏 stash"变化时重装菜单）
 * [POS]: 菜单栏的单一来源：菜单项只调窗口或当前工作区已有的方法，不自带逻辑；⌘B 不在这里（窗口内快捷键，见 window_view.go shortcuts）；⌃Tab / ⌃⇧Tab 只能是可见的菜单项（AppKit 不把它们交给视图，Hidden 项的键位也不触发，见 shortcuts.go）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/thewei/solco-gitkit/gitworkbench"

	"github.com/thewei/solco-git/internal/settings"
)

// menuState is what the menu bar's items depend on; the menu is rebuilt when
// it changes.
type menuState struct {
	hasRepo     bool
	canBack     bool
	canForward  bool
	hideStashes bool
	recent      string
}

func (w *Window) currentMenuState() menuState {
	state := menuState{
		hasRepo: w.activeShell() != nil, canBack: w.nav.canBack(), canForward: w.nav.canForward(),
		hideStashes: w.settings.Workbench.HideStashes,
	}
	for _, root := range w.settings.Window.Recent {
		state.recent += root + "\n"
	}
	return state
}

// refreshMenu installs the menu bar again when something it shows changed.
func (w *Window) refreshMenu() {
	if runtime.GOOS != "darwin" || w.win == nil {
		return
	}
	w.onUI(func() {
		state := w.currentMenuState()
		if w.menuInstalled && state == w.menuShown {
			return
		}
		w.menuInstalled, w.menuShown = true, state
		mygo.App.SetMenu(w.MenuBar())
	})
}

// MenuBar builds the macOS menu bar.
func (w *Window) MenuBar() *mygo.Menu {
	has := w.activeShell() != nil
	do := func(label, accelerator string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: accelerator, Click: func(*mygo.MenuItem, *mygo.Window) { w.onUI(fn) }}
	}
	// repo is an item of the workspace in front: disabled on the welcome page.
	repo := func(label, accelerator string, fn func(sh *Shell)) *mygo.MenuItem {
		item := do(label, accelerator, func() {
			if sh := w.activeShell(); sh != nil {
				fn(sh)
			}
		})
		item.Disabled = !has
		return item
	}

	appMenu := []*mygo.MenuItem{{Role: mygo.RoleAbout}}
	if w.updateItem != nil {
		appMenu = append(appMenu, w.updateItem())
	}
	appMenu = append(appMenu,
		mygo.Separator(),
		do("Settings…", accelSettings, w.openSettings),
		mygo.Separator(),
		&mygo.MenuItem{Role: mygo.RoleHide},
		&mygo.MenuItem{Role: mygo.RoleHideOthers},
		&mygo.MenuItem{Role: mygo.RoleUnhide},
		mygo.Separator(),
		&mygo.MenuItem{Role: mygo.RoleQuit},
	)

	recent := []*mygo.MenuItem{}
	for _, root := range w.settings.Window.Recent {
		root := root
		recent = append(recent, do(pathBase(root), "", func() { w.OpenRepository(root, "", "") }))
	}
	if len(recent) == 0 {
		recent = append(recent, &mygo.MenuItem{Label: "No Recent Repositories", Disabled: true})
	} else {
		recent = append(recent, mygo.Separator(), do("Clear Menu", "", w.clearRecent))
	}
	fileMenu := []*mygo.MenuItem{
		do("Open Repository…", accelOpen, w.chooseRepository),
		{Label: "Open Recent", Submenu: recent},
		do("Quick Launch…", accelQuickLaunch, w.openQuickLaunch),
		repo("Go to File…", accelGoToFile, func(*Shell) { w.openQuickTab(quickTabFiles) }),
		repo("Find in Files…", accelFindInFiles, func(*Shell) { w.openQuickTab(quickTabText) }),
		mygo.Separator(),
		repo("Save", accelSave, func(sh *Shell) { sh.saveEdit() }),
		mygo.Separator(),
		do("Close Window", accelCloseWindow, w.closeWindow),
		mygo.Separator(),
		repo("Show in Finder", "", func(sh *Shell) { sh.openExternalPathParent(sh.dir) }),
		repo("Copy Path", "", func(sh *Shell) { sh.copyToClipboard(sh.dir) }),
	}

	repositoryMenu := []*mygo.MenuItem{
		repo("Fetch", "", func(sh *Shell) { sh.fetchRepo() }),
		repo("Fetch All", "", func(sh *Shell) { sh.fetchAll(false) }),
		repo("Fetch All and Prune", "", func(sh *Shell) { sh.fetchAll(true) }),
		{Label: "Pull", Submenu: []*mygo.MenuItem{
			repo("Fast-Forward Only", "", func(sh *Shell) { sh.pullRepo() }),
			repo("Merge", "", func(sh *Shell) { sh.pullWith(gitworkbench.PullMerge, "Pull and Merge") }),
			repo("Rebase", "", func(sh *Shell) { sh.pullWith(gitworkbench.PullRebase, "Pull and Rebase") }),
		}},
		repo("Push", "", func(sh *Shell) { sh.pushRepo() }),
		repo("Push Multiple…", "", func(sh *Shell) { sh.openMultiPush() }),
		mygo.Separator(),
		repo("New Branch…", "", func(sh *Shell) { sh.openNewBranchDialog() }),
		repo("New Tag…", "", func(sh *Shell) { sh.openTagDialog("") }),
		mygo.Separator(),
		repo("Stash…", "", func(sh *Shell) { sh.openSaveStashDialog() }),
		repo("Apply Patch…", "", func(sh *Shell) { sh.applyPatchFile() }),
		repo("Save Changes as Patch…", "", func(sh *Shell) { sh.saveWorkingPatch() }),
		repo("Reflog…", "", func(sh *Shell) { sh.openReflogSheet() }),
		repo("Recovery…", "", func(sh *Shell) { sh.openRecoverySheet() }),
		repo("Command Log…", "", func(sh *Shell) { sh.openCommandLog() }),
		mygo.Separator(),
		repo("Undo Git Action…", accelUndoGit, func(sh *Shell) { sh.requestReplay(false) }),
		repo("Redo Git Action…", accelRedoGit, func(sh *Shell) { sh.requestReplay(true) }),
	}

	back := do("Back", accelBack, w.navBack)
	back.Disabled = !w.nav.canBack()
	forward := do("Forward", accelForward, w.navForward)
	forward.Disabled = !w.nav.canForward()

	viewMenu := []*mygo.MenuItem{
		repo("Changes", accelChanges, func(sh *Shell) { sh.showView(viewChanges) }),
		repo("Commits", accelCommits, func(sh *Shell) { sh.showView(viewCommits) }),
		repo("Files", accelFiles, func(sh *Shell) { sh.showView(viewFiles) }),
		mygo.Separator(),
		back,
		forward,
		mygo.Separator(),
		// No accelerator: ⌘B is the window's own shortcut, and a toggle must
		// not have two ways to fire in one keystroke.
		do("Toggle Sidebar", "", w.toggleSidebar),
		repo("Refresh", accelRefresh, func(sh *Shell) { sh.refreshCurrent() }),
		mygo.Separator(),
		do("Show Next Workspace", accelNextWorkspace, func() { w.stepWorkspace(1) }),
		do("Show Previous Workspace", accelPrevWorkspace, func() { w.stepWorkspace(-1) }),
		mygo.Separator(),
		{Label: "Highlight Moved Lines", Type: mygo.MenuItemCheckbox, Checked: !w.settings.Workbench.HideMovedLines,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				w.onUI(func() {
					hide := !w.settings.Workbench.HideMovedLines
					w.change(func(v *settings.Settings) { v.Workbench.HideMovedLines = hide })
				})
			}},
		{Label: "Hide Stashes in Commit List", Type: mygo.MenuItemCheckbox, Checked: w.settings.Workbench.HideStashes,
			Disabled: !has, Click: func(*mygo.MenuItem, *mygo.Window) {
				w.onUI(func() {
					hide := !w.settings.Workbench.HideStashes
					w.change(func(v *settings.Settings) { v.Workbench.HideStashes = hide })
					if sh := w.activeShell(); sh != nil {
						sh.loadBranches()
					}
				})
			}},
	}

	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Solco Git", Submenu: appMenu},
		{Label: "File", Submenu: fileMenu},
		{Label: "Repository", Submenu: repositoryMenu},
		{Label: "View", Submenu: viewMenu},
		{Role: mygo.RoleEditMenu},
		// A custom Window menu: Minimize and Zoom without the stock "Close
		// Window ⌘W", which the File menu already carries.
		//
		// ⌃⇥ / ⌃⇧⇥ step the workspaces here, as Finder's Window menu steps
		// its tabs: they have to be visible menu items — AppKit never hands
		// the keys to a view (see shortcuts.go), and a hidden item's key
		// equivalent does not fire (2026-11-08 real-machine run). The View
		// menu's ⇧⌘] / ⇧⌘[ do the same.
		{Label: "Window", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize},
			{Role: mygo.RoleZoom},
			mygo.Separator(),
			do("Show Next Workspace", accelNextCtrlTab, func() { w.stepWorkspace(1) }),
			do("Show Previous Workspace", accelPrevCtrlTab, func() { w.stepWorkspace(-1) }),
		}},
		{Role: mygo.RoleHelp},
	})
}

// clearRecent empties the recent list.
func (w *Window) clearRecent() {
	w.change(func(v *settings.Settings) { v.Window.Recent = nil })
}

// closeWindow is ⌘W: the window hides (its workspaces stay), like the close
// button.
func (w *Window) closeWindow() {
	if w.win != nil {
		w.win.Close()
	}
}
