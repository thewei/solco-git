package nativeui

/**
 * [INPUT]: 依赖 Window（标签条宿主：窗口句柄、偏好存储、主题）、gitkit 的 gitworkbench/filesview（Git 与 Files 的唯一数据层）、MyGo ui/Window
 * [OUTPUT]: 对外提供 Shell（一个仓库标签的完整工作台：Git 状态机 gitService、Diff/Commit 表面、Files 缓存与预览、各列表/对话框的呈现状态）、NewShell、窗口车道（onUI/applyOnWindow/callOnWindow）与偏好写入（applySettings）
 * [POS]: 从 Solco 的 Shell 里只带走 Git + Files 用到的那部分成员（2026-11-07 决定「Solco Git」第 1/4 条）；没有 Workspace/Pane/终端/Herdr；一个标签 = 一个 Shell，Window 负责把它们排成标签条
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"log/slog"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"

	"github.com/thewei/solco-git/internal/preview"
	"github.com/thewei/solco-git/internal/settings"
)

// Shell is one workspace's whole workbench (a workspace is one repository in
// the window's list): the Git orchestration, the Diff / Commit surfaces, the
// Files explorer and every list and dialog's own presentation state. State
// that belongs to the window (the workspace list, the preferences store, the
// theme) lives on Window and is reached through w.
type Shell struct {
	// w is the window that hosts this workspace; nil in headless tests.
	w *Window
	// win is the window handle every UI-lane hop goes through; nil without a
	// window (headless tests), which then run inline.
	win *mygo.Window
	// dir is the directory the workspace was opened on. The repository root is
	// resolved from it (gitService.root); a directory that is not inside a
	// repository stays an honest empty state.
	dir string
	// filesView is the sidebar's Files choice; off, the workspace shows Changes or
	// Commits (commitsMode picks which).
	filesView bool

	// status is the one line the shell toasts (flushed in View); errText is
	// the last operation error a surface shows itself.
	status  string
	errText string

	// filesGit is Solco Git's layer over the Files view (files_git.go);
	// commitTree the Tree mode of the commit review (commit_tree.go).
	filesGit     filesGitState
	commitTree   commitTreeState
	imageCompare imageCompareState

	// statusShown is the status line already toasted, so a line that stays
	// set is shown once.
	statusShown string

	// dialog* is the shared single-field dialog (new branch, rename, ...);
	// confirm* the shared confirmation. Both are bound to the repository they
	// were opened for (confirmRepoRoot) so a workspace switch cannot apply one to
	// another repository.
	dialogOpen      bool
	dialogKind      string
	dialogTarget    string
	dialogTitle     string
	dialogLabel     string
	dialogValue     string
	dialogAction    string // the primary button's title; "Save" when empty
	confirmOpen     bool
	confirmKind     string
	confirmTarget   string
	confirmTitle    string
	confirmMessage  string
	confirmRepoRoot string // binds any pending Git confirmation to its original repository

	// Commit shares the one verified Commit state (surface.commit); the
	// dialog is presentation-only and never creates a second Git draft.
	commitDialogOpen    bool
	commitDiscardPrompt bool

	mergeDialogOpen bool
	mergeTarget     string
	mergeRepoRoot   string
	mergeStrategy   int // 0: FF-only, 1: create merge commit, 2: squash

	gitMore   gitMoreState
	gitNotice gitNoticeState

	// settings is the presentation snapshot of the app's preferences,
	// refreshed whenever any workspace (or the settings page) changes them.
	settings settings.Settings

	// recentFiles are the files opened in Files, newest first.
	recentFiles    []string
	filesStartInfo filesStartCache
	// rightPanel holds the Files explorer's cache and preview tiers.
	rightPanel rightPanelState
	// filesSearch is the Files sidebar's one search (names | content).
	filesSearch filesSearchState
	// staticPreview serves the project root to HTML previews, started by the
	// first Open Preview and closed with the root (files_static_preview.go).
	staticPreview     *preview.StaticServer
	staticPreviewRoot string

	// nav is the keyboard model of the sidebar lists and trees (list_nav.go).
	nav listNavs
	// sidebarCollapsed hides the sidebar: session-only presentation state.
	sidebarCollapsed bool
	// sidebarW is the sidebar's width: sidebarWidth until the user drags the
	// trailing edge.
	sidebarW float32

	// git owns the Git Workbench orchestration (single cache).
	git *gitService
	// surface owns the Diff / Commit state machine.
	surface workspaceSurfaceState
	// changes panel state.
	changes         changesPanelState
	changesList     ui.ListState
	changesSelected int
	changesTree     *gitworkbench.TreeNode
	// The sidebar's Unstaged/Staged panes: Fork-style split lists, each with
	// its own list state over the shared tree data. The choices hold the rows
	// picked with Cmd/Shift-click for the pane's Stage or Unstage action,
	// keyed by path.
	unstagedList     ui.ListState
	unstagedSelected int
	unstagedChoice   ui.Selection[string]
	stagedList       ui.ListState
	stagedSelected   int
	stagedChoice     ui.Selection[string]
	stagedPaneHeight float32
	unstagedOpen     bool
	stagedOpen       bool

	// The Changes center is split between commits/files and the Diff viewer.
	gitUpperHeight       float32
	gitFilePaneWidth     float32
	gitCommitBodyOpen    bool // the commit card shows the whole body
	gitQuickSubject      string
	gitQuickBody         string
	gitQuickAmend        bool
	gitCommitFilesFilter string
	// The file navigator closes by itself in a narrow center; the user's say
	// over it is kept per width class (see gitFilePaneOpen).
	gitFilePaneOpenNarrow bool
	gitFilePaneClosedWide bool

	// The Git reference tree is a presentation projection, scoped to the repo.
	gitTreeClosed   map[string]bool
	gitRefsFilter   string
	gitCommitFilter string
	// gitRefsSearchOpen shows the sidebar head's reference search field;
	// gitRefsSelected is the ref row a single click selected (kind:name),
	// which a double click or Enter then acts on; gitRefsHinted is whether the
	// "double-click to act" hint has already been shown this run.
	gitRefsSearchOpen bool
	gitRefsSelected   string
	gitRefsHinted     bool
	// gitRefsReveal is the sidebar ref row a commit chip asked to scroll to.
	gitRefsReveal string
	// The three Git lists' search buttons: Changes files, Commits, commit files.
	changesSearchOpen, gitCommitSearchOpen, gitCommitFilesSearchOpen bool
	gitConflictFilesExpanded                                         bool
	// commitsMode shows the All Commits pane instead of the split panes.
	commitsMode     bool
	commitsList     ui.ListState
	commitsSelected int
	branchMenuOpen  bool
	branchFilter    string

	// pendingToast is flushed as a toast on the next frame (background lanes
	// cannot toast).
	pendingToast string

	// filesWebview is the AVIF/ICO preview page's embedded WebView (2026-11-08);
	// it exists only while one of those two formats is the open file, and
	// filesWebviewBroken latches a failed creation so the center shows a
	// truthful empty state.
	filesWebview       *mygo.WebView
	filesWebviewBroken bool

	// previews are the Local Preview windows that open an HTML file's served
	// page (files_static_preview.go).
	previews *previewManager
	// viewFrame counts the workspace's View passes: the list keyboard model uses it
	// to tell "the same mount" from "a new one" (list_nav.go).
	viewFrame uint64

	// uiApplyOverride lets a test observe the real apply lane of background
	// work. Production never sets it; nil keeps the drop-in-headless rule.
	uiApplyOverride func(fn func())
}

// NewShell makes the workbench of one workspace opened on dir.
func NewShell(w *Window, dir string) *Shell {
	s := &Shell{
		w:        w,
		dir:      dir,
		settings: settings.Default(),

		rightPanel: defaultRightPanelState(),
	}
	if w != nil {
		s.win = w.win
		s.settings = w.settings
		s.uiApplyOverride = w.uiApply
	}
	s.git = newGitService()
	s.previews = newPreviewManager()
	s.changesSelected = -1
	s.changesList.Selected = &s.changesSelected
	s.unstagedSelected = -1
	s.unstagedList.Selected = &s.unstagedSelected
	s.stagedSelected = -1
	s.stagedList.Selected = &s.stagedSelected
	s.stagedPaneHeight = 220
	s.sidebarW = sidebarWidth
	s.gitUpperHeight = gitSplitDefaultTop
	s.gitFilePaneWidth = gitFilePaneDefaultWidth
	s.gitTreeClosed = make(map[string]bool)
	s.unstagedOpen, s.stagedOpen = true, true
	s.git.splitLayout = s.settings.Workbench.SplitDiff
	s.surface.start(WorkspaceContextKey{})
	return s
}

// selectedTabCWD is the directory the workspace was opened on: the Git context and
// the Files explorer both start from it.
func (s *Shell) selectedTabCWD() string { return s.dir }

// applyOnWindow marshals a state mutation onto the window update lane, or
// runs it inline when no window exists (headless tests).
func (s *Shell) applyOnWindow(apply func()) {
	if s.win == nil {
		apply()
		return
	}
	s.win.Update(apply)
}

// callOnWindow runs fn on the window UI lane and waits for it; inline when no
// window exists (headless tests). Shell-bound methods run on their own
// goroutines, so this is the only path back to Shell state.
func (s *Shell) callOnWindow(fn func()) {
	if s.win == nil {
		fn()
		return
	}
	done := make(chan struct{})
	s.applyOnWindow(func() {
		fn()
		close(done)
	})
	<-done
}

// webTheme is the appearance the embedded WebView page follows: the
// framework call blocks on the main thread, so it only runs with a window
// attached and always on the UI lane; headless sessions report light without
// touching the framework.
func (s *Shell) webTheme() string {
	if s.win != nil && mygo.Theme.IsDark() {
		return "dark"
	}
	return "light"
}

// applySettings routes one preference mutation through the window's store and
// refreshes every workspace's presentation snapshot.
func (s *Shell) applySettings(mutate func(*settings.Settings) error) {
	if s.w == nil {
		// Headless: apply on the local snapshot so tests can observe it.
		if err := mutate(&s.settings); err != nil {
			slog.Warn("apply settings update failed", "error", err)
		}
		return
	}
	updated, err := s.w.updateSettings(context.Background(), mutate)
	if err != nil {
		slog.Warn("apply settings update failed", "error", err)
		return
	}
	s.settings = updated
}
