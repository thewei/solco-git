package nativeui

/**
 * [INPUT]: 依赖 settings.Store（偏好与窗口可恢复状态）、gitworkbench.Runner（只用来把目录解析成仓库根）、MyGo 窗口句柄、shell.go 的 Shell（一个仓库工作区的工作台）
 * [OUTPUT]: 对外提供 Window（一个窗口 = 一份工作区清单 + 当前工作区：打开/去重/切换并刷新/排序/移除确认/恢复）、NewWindow、AttachWindow、OpenRepository/OpenURL（外部入口：打开即加入清单）、addWorkspacePath（设置里「添加工作区」：只加入，不抢走当前）、偏好写入 updateSettings、themeSourceFor / syncThemeSource（Theme 偏好 → 框架的 theme source：窗口侧栏材质、菜单与 WebView 页跟着变，而不只是每帧挑的颜色）、Close
 * [POS]: Solco Git 的最外层状态机：工作区 = 用户留在标题带下拉里的一个仓库（Solco Git 自己的清单，与 Herdr 的 workspace 无关）；同一个仓库根只有一个工作区（再打开就切过去）；移除有草稿或正在跑操作的工作区先确认；清单、当前工作区与每个工作区的视图写进偏好，重开时恢复（不恢复提交信息草稿）；没有 Pane/终端/Herdr
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/thewei/solco-gitkit/gitworkbench"

	"github.com/thewei/solco-git/internal/platform"
	"github.com/thewei/solco-git/internal/settings"
)

const (
	// maxWorkspaces bounds the list: a workspace is a watcher-less, timer-light
	// Shell, and only the ones that were shown hold a Git snapshot and a Files
	// cache.
	maxWorkspaces = 100
	// pollEvery is how often the front workspace asks its snapshot cache whether a
	// background refresh is due (the cache itself decides: 30 s).
	pollEvery = 4 * time.Second
)

// workspace is one entry of the title band's pull-down: a repository's whole
// workbench under an id that is never reused, so a click, a key or a pending
// confirm can name it safely. The list is Solco Git's own — nothing to do with
// Herdr's workspaces, which this app has none of.
type workspace struct {
	id int
	sh *Shell
}

// Window is the one Solco Git window: the workspace in front over the shared
// chrome (sidebar frame, title band with its workspace pull-down, settings,
// Quick Launch).
type Window struct {
	win    *mygo.Window
	store  settings.Store
	runner *gitworkbench.Runner
	solco  *platform.SolcoLink // the one-way link to Solco (terminal owner: In Solco)
	// servers lists the installed language servers for Settings; swapped in tests.
	servers languageServers

	setMu    sync.Mutex // serializes read-modify-write of the preferences
	settings settings.Settings

	workspaces []*workspace
	active     int // index into workspaces; -1 on the welcome page
	nextID     int

	// The sidebar is window chrome: its width and fold are shared by every
	// workspace (each Shell is handed them at the start of its frame and hands
	// them back at the end).
	sidebarW         float32
	sidebarCollapsed bool
	sidebarSlideW    float32
	sidebarWDragging bool

	picker workspacePicker

	// nav is the Back / Forward stack: (workspace, view) places, this run only.
	nav navHistory

	settingsOpen bool
	// removeAsk is the id of the workspace whose removal awaits confirmation.
	removeAsk int
	quick     quickLaunch
	// quitAsk is set while a quit waits on unsaved edits (files_edit.go); the
	// quit it holds is run when the user chooses to quit anyway.
	quitAsk func()
	// quitProcs is how many terminals had a process running when the quit was held.
	quitProcs int

	// recentBranch is each recent repository's current branch, read once in
	// the background for the welcome page and Quick Launch; recentAsked marks
	// the roots already being read.
	recentBranch map[string]string
	recentAsked  map[string]bool

	// uiApply is a test seam: when set, every workspace's background results are
	// handed to it instead of the window's update lane (see Shell.uiApplyOverride).
	uiApply func(fn func())

	// pending holds the solcogit:// links that arrived before the window was
	// attached; attached flips once the last session is restored.
	pendingMu sync.Mutex
	pending   []openRequest
	attached  bool

	// menuInstalled/menuShown remember the state the installed menu bar was
	// built from, so it is rebuilt only when it would differ.
	menuInstalled bool
	menuShown     menuState

	// toast is flushed on the next frame (background lanes cannot toast).
	toast string
	// raise brings the window to the front (set by main once it exists).
	raise func()
	// updateItem makes the app menu's Check for Updates… item; main sets it
	// when the updater plugin is in use, so headless windows have none.
	updateItem func() *mygo.MenuItem

	pollStop chan struct{}
}

// NewWindow reads the preferences and makes an empty window; AttachWindow
// restores its workspaces once the native window exists.
func NewWindow(store settings.Store) *Window {
	w := &Window{
		store:    store,
		runner:   &gitworkbench.Runner{},
		solco:    platform.NewSolcoLink(),
		active:   -1,
		nextID:   1,
		sidebarW: sidebarWidth,
	}
	cfg, err := store.Load(context.Background())
	if err != nil {
		slog.Warn("load settings failed; starting from defaults", "error", err)
		cfg = settings.Default()
	}
	w.settings = normalizeSettings(cfg)
	if width := float32(w.settings.Window.SidebarWidth); width > 0 {
		w.sidebarW = clampF(width, sidebarMinWidth, sidebarMaxWidth)
	}
	return w
}

// normalizeSettings clamps a loaded or edited document to what the UI offers.
func normalizeSettings(cfg settings.Settings) settings.Settings {
	cfg.Window.Migrate()
	if len(cfg.Window.Workspaces) > maxWorkspaces {
		cfg.Window.Workspaces = cfg.Window.Workspaces[:maxWorkspaces]
	}
	switch cfg.General.Appearance {
	case "system", "light", "dark":
	default:
		cfg.General.Appearance = "system"
	}
	switch cfg.General.Density {
	case "", "compact", "default", "comfortable":
	default:
		cfg.General.Density = ""
	}
	known := false
	for _, choice := range autoFetchChoices {
		known = known || choice.minutes == cfg.Workbench.AutoFetchMinutes
	}
	if !known {
		cfg.Workbench.AutoFetchMinutes = 0
	}
	if len(cfg.Window.Recent) > settings.RecentMax {
		cfg.Window.Recent = cfg.Window.Recent[:settings.RecentMax]
	}
	return cfg
}

// WindowStateKey is the MyGo window-state key: empty turns restoration off.
func (w *Window) WindowStateKey() string {
	if !w.settings.General.RestoreWindow {
		return ""
	}
	return "main"
}

// AttachWindow binds the native window, restores the workspaces of the last
// run and starts the front workspace's refresh poll.
func (w *Window) AttachWindow(win *mygo.Window) {
	w.win = win
	w.syncThemeSource()
	win.OnFileDrop(func(e *mygo.FileDropEvent) {
		paths := e.Paths
		w.onUI(func() { w.dropFolders(paths) })
	})
	w.restoreWorkspaces()
	w.finishAttach()
	w.startPoll()
	w.refreshMenu()
}

// SaveState writes the workspace list (and window preferences) now; the close
// button calls it, since closing only hides the window.
func (w *Window) SaveState() { w.saveState() }

// SetRaise installs the callback that brings the window to the front.
func (w *Window) SetRaise(raise func()) { w.raise = raise }

// SetUpdateItem installs the maker of the app menu's Check for Updates… item
// (the updater plugin's, which main owns). The menu is rebuilt when its state
// changes, so it asks for a fresh item each time.
func (w *Window) SetUpdateItem(newItem func() *mygo.MenuItem) { w.updateItem = newItem }

// Close stops the poll and every workspace; call once at quit.
func (w *Window) Close() {
	if w.pollStop != nil {
		close(w.pollStop)
		w.pollStop = nil
	}
	for _, ws := range w.workspaces {
		ws.sh.Close()
	}
}

// onUI runs fn on the window's update lane, inline without a window.
func (w *Window) onUI(fn func()) {
	if w.win == nil {
		fn()
		return
	}
	w.win.Update(fn)
}

// notice shows one toast on the next frame.
func (w *Window) notice(text string) {
	w.toast = text
	if w.win != nil {
		w.win.Invalidate()
	}
}

// activeShell is the front workspace's workbench; nil on the welcome page.
func (w *Window) activeShell() *Shell {
	if w.active < 0 || w.active >= len(w.workspaces) {
		return nil
	}
	return w.workspaces[w.active].sh
}

// startPoll asks the front workspace, every few seconds, whether its Git snapshot
// is due for a background refresh. The cache decides (30 s); an idle window
// costs one cheap check.
func (w *Window) startPoll() {
	if w.win == nil || w.pollStop != nil {
		return
	}
	stop := make(chan struct{})
	w.pollStop = stop
	go func() {
		tick := time.NewTicker(pollEvery)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if !w.win.IsVisible() {
					continue
				}
				w.win.Update(func() {
					if sh := w.activeShell(); sh != nil {
						sh.pollGit()
					}
				})
			}
		}
	}()
}

// ---- workspaces ------------------------------------------------------------

// indexOfRoot finds the workspace that already shows a repository root.
func (w *Window) indexOfRoot(root string) int {
	for i, ws := range w.workspaces {
		if ws.sh.dir == root {
			return i
		}
	}
	return -1
}

// indexOfID finds a workspace by its id; -1 when it is gone.
func (w *Window) indexOfID(id int) int {
	for i, ws := range w.workspaces {
		if ws.id == id {
			return i
		}
	}
	return -1
}

// addWorkspace adds a workspace on an already resolved root. show brings it to
// the front; without it the front stays where it is (unless nothing was in
// front, which is the welcome page).
func (w *Window) addWorkspace(root string, view viewKind, show bool) *workspace {
	if view == "" {
		view = viewChanges
	}
	sh := NewShell(w, root)
	// The root is known (it was just resolved), so the first frame goes
	// straight to reading it.
	sh.git.rootOf[root] = root
	ws := &workspace{id: w.nextID, sh: sh}
	w.nextID++
	w.workspaces = append(w.workspaces, ws)
	if show || w.active < 0 {
		w.active = len(w.workspaces) - 1
	}
	sh.setView(view)
	return ws
}

// selectWorkspace brings workspace i to the front. One that was shown before
// is read again on the way in — choosing it from the pull-down is the refresh
// the title band promises; a first visit reads it anyway.
func (w *Window) selectWorkspace(i int) {
	if i < 0 || i >= len(w.workspaces) || i == w.active {
		return
	}
	if from := w.activeShell(); from != nil && from.holdCommitNavigation() {
		return
	}
	w.active = i
	sh := w.workspaces[i].sh
	seen := sh.git != nil && sh.git.root != ""
	sh.arrive()
	if seen {
		sh.refreshCurrent()
	}
	w.syncTitle()
	w.saveState()
}

// chooseWorkspace is a pick in the pull-down: another workspace comes to the
// front, and the one already in front is simply read again.
func (w *Window) chooseWorkspace(id int) {
	i := w.indexOfID(id)
	switch {
	case i < 0:
	case i == w.active:
		w.workspaces[i].sh.refreshCurrent()
	default:
		w.selectWorkspace(i)
	}
}

// stepWorkspace moves the front workspace by delta, wrapping.
func (w *Window) stepWorkspace(delta int) {
	if len(w.workspaces) < 2 {
		return
	}
	w.selectWorkspace((w.active + delta + len(w.workspaces)) % len(w.workspaces))
}

// moveWorkspace shifts workspace id up (-1) or down (+1) the list: the order
// the pull-down shows. The front workspace stays in front.
func (w *Window) moveWorkspace(id, delta int) {
	from := w.indexOfID(id)
	to := from + delta
	if from < 0 || to < 0 || to >= len(w.workspaces) {
		return
	}
	front := w.workspaces[w.active]
	w.workspaces[from], w.workspaces[to] = w.workspaces[to], w.workspaces[from]
	w.active = w.indexOfID(front.id)
	w.saveState()
}

// workspaceNeedsConfirm reports a workspace whose removal would lose
// something: a commit message being written, or a Git operation still running.
func (w *Window) workspaceNeedsConfirm(ws *workspace) bool {
	sh := ws.sh
	return sh.commitDraftPresent() || sh.gitBusy() || sh.fileDirty()
}

// requestRemoveWorkspace removes workspace i, asking first when it holds a
// draft or a running operation.
func (w *Window) requestRemoveWorkspace(i int) {
	if i < 0 || i >= len(w.workspaces) {
		return
	}
	sh := w.workspaces[i].sh
	sh.term.askBusy = sh.termBusy() // asked once, here, not per frame
	if w.workspaceNeedsConfirm(w.workspaces[i]) || sh.term.askBusy {
		w.removeAsk = w.workspaces[i].id
		return
	}
	w.removeWorkspace(i)
}

// removeWorkspace takes workspace i out of the list — only the list: the
// repository on disk is untouched, and it stays in Recent. The last one
// leaves the welcome page, not a closed window.
func (w *Window) removeWorkspace(i int) {
	if i < 0 || i >= len(w.workspaces) {
		return
	}
	gone := w.workspaces[i]
	w.workspaces = append(w.workspaces[:i], w.workspaces[i+1:]...)
	w.nav.forget(gone.id) // Back and Forward never lead to a workspace that left
	switch {
	case len(w.workspaces) == 0:
		w.active = -1
	case i < w.active:
		w.active--
	case i == w.active:
		w.active = min(i, len(w.workspaces)-1)
		w.workspaces[w.active].sh.arrive()
	}
	gone.sh.Close()
	w.syncTitle()
	w.saveState()
}

// syncTitle names the native window after the front repository.
func (w *Window) syncTitle() {
	if w.win == nil {
		return
	}
	title := "Solco Git"
	if sh := w.activeShell(); sh != nil && sh.dir != "" {
		title = filepath.Base(sh.dir) + " — Solco Git"
	}
	w.win.SetTitle(title)
	w.refreshMenu()
}

// ---- opening ---------------------------------------------------------------

// OpenRepository opens a directory — the repository root or anything inside
// it — as a workspace: a repository not in the list yet is added to it, and
// either way it comes to the front. view and file are optional
// (Changes/Commits/Files; a path inside the repository). Every way in — ⌘O,
// Quick Launch, a drop, a solcogit:// link — comes through here.
func (w *Window) OpenRepository(path string, view viewKind, file string) {
	w.openPath(path, view, file, true)
}

// addWorkspacePath is Settings → Workspaces → Add Workspace…: the repository
// joins the list without taking the front, unless there is none yet.
func (w *Window) addWorkspacePath(path string) { w.openPath(path, "", "", false) }

func (w *Window) openPath(path string, view viewKind, file string, show bool) {
	path = expandHome(path)
	if path == "" || !filepath.IsAbs(path) {
		w.notice("Choose a folder to open")
		return
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path) // a file stands for the folder it is in
	}
	resolve := func() {
		root, err := w.runner.ResolveRoot(context.Background(), path)
		w.onUI(func() {
			if err != nil || root == "" {
				w.notice("“" + filepath.Base(path) + "” is not a Git repository")
				return
			}
			if show {
				w.openResolved(root, view, file)
			} else {
				w.addResolved(root)
			}
		})
	}
	if w.win == nil {
		resolve() // headless: inline
		return
	}
	go resolve()
}

// openRepository is the in-package spelling shells use.
func (w *Window) openRepository(path string, view viewKind, file string) {
	w.OpenRepository(path, view, file)
}

// addResolved is addWorkspacePath's UI-lane half: a new repository is
// appended and remembered; one already listed says so rather than doing
// nothing.
func (w *Window) addResolved(root string) {
	if i := w.indexOfRoot(root); i >= 0 {
		w.notice("“" + filepath.Base(root) + "” is already a workspace")
		return
	}
	if len(w.workspaces) >= maxWorkspaces {
		w.notice("Remove a workspace before adding another")
		return
	}
	ws := w.addWorkspace(root, "", false)
	if w.activeShell() == ws.sh {
		ws.sh.arrive() // the list was empty: the new one is in front
		w.syncTitle()
	}
	w.noteRecent(root)
	w.saveState()
	w.refreshMenu()
	slog.Info("workspace added", "workspaces", len(w.workspaces))
}

// openResolved is OpenRepository's UI-lane half.
func (w *Window) openResolved(root string, view viewKind, file string) {
	reused := false
	if i := w.indexOfRoot(root); i >= 0 {
		reused = true
		w.selectWorkspace(i)
	} else {
		if len(w.workspaces) >= maxWorkspaces {
			w.notice("Remove a workspace before opening another repository")
			return
		}
		w.addWorkspace(root, view, true)
	}
	sh := w.activeShell()
	if sh == nil {
		return
	}
	if view != "" {
		sh.setView(view)
	}
	if file != "" {
		if view == viewChanges {
			sh.selectChangedFile(file) // Changes names a changed file in its own list
		} else {
			sh.revealFile(file)
		}
	}
	sh.arrive()
	w.noteRecent(root)
	w.syncTitle()
	w.saveState()
	slog.Info("repository shown", "workspaces", len(w.workspaces), "reused", reused, "view", string(view))
	if w.raise != nil {
		w.raise()
	}
}

// chooseRepository is ⌘O: the system folder picker, then OpenRepository.
func (w *Window) chooseRepository() {
	w.pickFolders("Open Repository", "Choose a Git repository, or any folder inside one.", false, func(path string) {
		w.OpenRepository(path, "", "")
	})
}

// chooseWorkspacesToAdd is Settings → Workspaces → Add Workspace…: the same
// picker, several folders at once, each joining the list without taking the
// front.
func (w *Window) chooseWorkspacesToAdd() {
	w.pickFolders("Add Workspace", "Choose Git repositories, or folders inside them.", true, w.addWorkspacePath)
}

func (w *Window) pickFolders(title, message string, multiple bool, each func(path string)) {
	if w.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    w.win,
			Title:     title,
			Message:   message,
			Directory: true,
			Multiple:  multiple,
		})
		if err != nil {
			return
		}
		for _, path := range paths {
			if path != "" {
				each(path)
			}
		}
	}()
}

// expandHome expands a leading ~ so a typed or restored path is absolute.
func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

// noteRecent moves a root to the front of the recent list.
func (w *Window) noteRecent(root string) {
	_, _ = w.mutateSettings(func(cfg *settings.Settings) error {
		recent := []string{root}
		for _, r := range cfg.Window.Recent {
			if r != root && len(recent) < settings.RecentMax {
				recent = append(recent, r)
			}
		}
		cfg.Window.Recent = recent
		return nil
	})
}

// ---- persistence -----------------------------------------------------------

// restoreWorkspaces reopens the workspaces of the last run (repositories that
// are still directories), with each one's view and the front workspace. A
// commit message draft is deliberately not restored.
func (w *Window) restoreWorkspaces() {
	cfg := w.settings.Window
	for _, saved := range cfg.Workspaces {
		if len(w.workspaces) >= maxWorkspaces {
			break
		}
		root := expandHome(saved.Root)
		if info, err := os.Stat(root); err != nil || !info.IsDir() || w.indexOfRoot(root) >= 0 {
			continue
		}
		w.addWorkspace(root, parseView(saved.View), false)
	}
	if len(w.workspaces) > 0 {
		w.active = clamp(cfg.Active, 0, len(w.workspaces)-1)
		w.workspaces[w.active].sh.arrive()
		w.syncTitle()
	}
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// saveState writes the workspaces, the front one and the sidebar width.
func (w *Window) saveState() {
	saved := make([]settings.WorkspaceSettings, 0, len(w.workspaces))
	for _, ws := range w.workspaces {
		saved = append(saved, settings.WorkspaceSettings{Root: ws.sh.dir, View: string(ws.sh.currentView())})
	}
	active, width := max(w.active, 0), int(w.sidebarW)
	_, _ = w.mutateSettings(func(cfg *settings.Settings) error {
		cfg.Window.Workspaces, cfg.Window.Active, cfg.Window.SidebarWidth = saved, active, width
		return nil
	})
}

// updateSettings is the shells' way to change preferences: it returns the
// stored document and refreshes every workspace's snapshot of it.
func (w *Window) updateSettings(ctx context.Context, mutate func(*settings.Settings) error) (settings.Settings, error) {
	return w.mutateSettingsCtx(ctx, mutate)
}

func (w *Window) mutateSettings(mutate func(*settings.Settings) error) (settings.Settings, error) {
	return w.mutateSettingsCtx(context.Background(), mutate)
}

func (w *Window) mutateSettingsCtx(ctx context.Context, mutate func(*settings.Settings) error) (settings.Settings, error) {
	w.setMu.Lock()
	defer w.setMu.Unlock()
	next := w.settings
	if err := mutate(&next); err != nil {
		return w.settings, err
	}
	next = normalizeSettings(next)
	if err := w.store.Save(ctx, next); err != nil {
		slog.Warn("save settings failed", "error", err)
		return w.settings, err
	}
	w.settings = next
	highContrast.Store(next.General.HighContrast)
	for _, ws := range w.workspaces {
		ws.sh.settings = next
	}
	w.refreshMenu()
	return next, nil
}

// openSettings opens the preferences sheet (⌘,).
func (w *Window) openSettings() { w.settingsOpen = true }

// ---- the front workspace's per-frame sync ----------------------------------

// handOver gives the front workspace the window's sidebar state before its frame.
func (w *Window) handOver(sh *Shell) {
	sh.sidebarW = w.sidebarW
	sh.sidebarCollapsed = w.sidebarCollapsed
	sh.settings = w.settings
}

// themeSourceFor is the framework's theme source a Theme preference stands
// for: only an explicit Light or Dark overrides the system.
func themeSourceFor(appearance string) mygo.ThemeSource {
	switch appearance {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// syncThemeSource applies Settings → Theme to the framework's theme. The
// colours this package picks per frame are not enough: the window's sidebar
// material, the in-window menus and the Files preview's web page follow the
// framework's source, so a Light choice on a dark Mac drew light text over a
// dark sidebar (seen in a real window). Headless windows leave the framework
// alone.
func (w *Window) syncThemeSource() {
	if w.win == nil {
		return
	}
	mygo.Theme.SetSource(themeSourceFor(w.settings.General.Appearance))
}

// resolvedDark maps the appearance preference onto the system's scheme.
func (w *Window) resolvedDark(systemDark bool) bool {
	switch w.settings.General.Appearance {
	case "light":
		return false
	case "dark":
		return true
	}
	return systemDark
}
