package nativeui

/**
 * [INPUT]: 依赖 Shell 的 Git 编排（openChanges/revealDiffFile/dirtyWorktree/openConfirm）、Window.chooseRepository
 * [OUTPUT]: 对外提供 changesClickSide/openChangesSide（侧栏树行点击）、confirmSwitchBranch、agentAgo（「3h」式相对时间）、webviewBackground、chooseFolderAction（非仓库空态的出路）
 * [POS]: 复制来的 Git/Files 呈现代码在 Solco 里原本跨出去调用的几个小边界，在这里用本应用的语义落地；没有第二份实现的可能，所以一处集中
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// webviewBackground is the WebView pages' paint-before-load colour: the
// window's own background in either appearance, so a loading page never
// flashes the wrong shade.
const webviewBackground = "light-dark(#fcfcfc, #0a0a0a)"

// changesClickSide opens a tree row from one of the sidebar's split panes:
// files open the center review on that pane's side of the change.
func (s *Shell) changesClickSide(row gitworkbench.FlatRow, view gdViewSide) {
	if row.Node.Dir {
		s.toggleChangesDir(row.Node.Path)
		return
	}
	s.openChangesSide(row.Node.Path, view)
	s.revealDiffFile(row.Node.Path)
}

// openChangesSide opens the center review for path with the given view side.
func (s *Shell) openChangesSide(path string, view gdViewSide) {
	s.openChanges(path)
	if f, _ := s.gdFileForPath(path); f != nil {
		f.setView(view)
		s.git.gdRowsDirty = true
	}
}

// confirmSwitchBranch switches at once on a clean worktree and asks first
// when uncommitted changes would come along.
func (s *Shell) confirmSwitchBranch(name string) {
	if !s.dirtyWorktree() {
		s.switchBranch(name)
		return
	}
	s.openConfirm("git-switch-branch", name, "Switch Branch With Local Changes?",
		"Your uncommitted changes come along to "+name+". Git refuses if they conflict.")
}

// agentAgo is the short "last worked" tag: "<1m", "7m", "3h", "2d"; empty
// when the instant is unknown.
func agentAgo(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return itoa(int(d/time.Minute)) + "m"
	case d < 48*time.Hour:
		return itoa(int(d/time.Hour)) + "h"
	default:
		return itoa(int(d/(24*time.Hour))) + "d"
	}
}

// chooseFolderAction is the empty state's way out when the workspace's directory is
// not a repository: pick another folder. nil (no action) without a window.
func (s *Shell) chooseFolderAction(c *ui.Context) func() {
	if s.w == nil {
		return nil
	}
	return func() {
		if compactPrimaryButton(c, "Choose Folder…").Clicked() {
			s.w.chooseRepository()
		}
	}
}

// pathBase is the last element of a path, "" for an empty one.
func pathBase(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

// shortHash is the seven-character form of a commit hash.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
