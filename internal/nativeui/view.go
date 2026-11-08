package nativeui

/**
 * [INPUT]: 依赖 shell.go 的 filesView/commitsMode、surface.go 的 Diff/Commit 状态机、right_panel.go 的 Files 根同步
 * [OUTPUT]: 对外提供 viewKind（Changes / Commits / Files）、Shell.currentView/showView（侧栏顶部三个标签的唯一切换点，Commit 草稿未收起时拦住离开）
 * [POS]: 取代 Solco 的 Router 路由：一个标签里只有这三个视图，没有页面栈
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

// viewKind is one of the three views a workspace offers, the sidebar's top
// tabs.
type viewKind string

const (
	viewChanges viewKind = "changes"
	viewCommits viewKind = "commits"
	viewFiles   viewKind = "files"
)

// parseView reads a persisted or URL view name; anything unknown is Changes.
func parseView(name string) viewKind {
	switch viewKind(name) {
	case viewCommits:
		return viewCommits
	case viewFiles:
		return viewFiles
	}
	return viewChanges
}

// currentView is the view the workspace shows now.
func (s *Shell) currentView() viewKind {
	switch {
	case s.filesView:
		return viewFiles
	case s.commitsMode:
		return viewCommits
	}
	return viewChanges
}

// showView is the user's choice of a view. A Commit draft that is still open
// holds the workspace where it is: it has to be cancelled deliberately first.
func (s *Shell) showView(v viewKind) {
	if s.holdCommitNavigation() {
		return
	}
	s.setView(v)
}

// setView puts the workspace on a view with no draft check: for a new
// workspace, a restored one and an external link, none of which can have a
// draft yet.
func (s *Shell) setView(v viewKind) {
	switch v {
	case viewFiles:
		s.syncRightPanelRoot()
		s.expandSidebar() // Files always exposes its tree on entry.
		s.filesView = true
	case viewCommits:
		s.filesView = false
		s.showGitCommits()
		s.ensureGitSnapshot(false)
	case viewChanges:
		s.filesView = false
		s.reviewLocalChanges()
		s.ensureGitSnapshot(false)
	}
}

// expandSidebar opens the sidebar (the window owns the fold; the workspace's copy
// follows at once so the rest of this frame agrees).
func (s *Shell) expandSidebar() {
	s.sidebarCollapsed = false
	if s.w != nil {
		s.w.sidebarCollapsed = false
	}
}
