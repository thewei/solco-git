package nativeui

/**
 * [INPUT]: 依赖 mygo/ui 的 Element.Shortcut（列表有焦点才触发）、git_history_ui.go 的 pickCommit、git_operations.go 的 reviewCommit、sidebar.go 的 Commits 列表
 * [OUTPUT]: 对外提供 commitKeys（Commits 列表的键盘光标：↑↓ / j k 走行、Home / End 到首尾、⇧ 加这些键是范围多选；审阅在光标停下 commitFollowWait 后才读，按住不放不会每行起一个 git 进程）、commitCursorHash（当前该画选中色的提交：光标，否则正在审阅的那个）
 * [POS]: 键盘 P1（2026-11-16 决定「Solco Git 的键盘交互」）：Commits 列表原先没有 Selected，方向键什么也不动；这里不借 MyGo 列表自己的选择模型，因为点击已经走 pickCommit（⌘ / ⇧ 多选、普通点击审阅），两套选择会互相打架
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// commitFollowWait is how long the keyboard cursor rests on a commit before
// the review reads it: a held arrow key walks rows at key-repeat speed, and
// each stop would otherwise start a git process.
var commitFollowWait = 120 * time.Millisecond

// commitKeyState is the Commits list's keyboard cursor.
type commitKeyState struct {
	// cursor is the commit the keys stand on; "" means the one under review.
	cursor string
	timer  *time.Timer
}

// commitCursorHash is the commit that wears the selected face: the keyboard
// cursor, else the commit under review.
func (s *Shell) commitCursorHash() string {
	if s.commitKbd.cursor != "" {
		return s.commitKbd.cursor
	}
	if s.gdReviewingCommit() {
		return s.git.commitHash
	}
	return ""
}

// commitCursorRelease hands the highlight back to the review: a click, or a
// review that moved by some other road, ends the keyboard's visit.
func (s *Shell) commitCursorRelease() {
	if s.commitKbd.timer != nil {
		s.commitKbd.timer.Stop()
		s.commitKbd.timer = nil
	}
	s.commitKbd.cursor = ""
}

// commitKeys answers the arrow keys while the commit list holds the focus.
// list is the built list element; entries are the rows it shows.
func (s *Shell) commitKeys(list ui.Element, entries []gitworkbench.CommitInfo) {
	if len(entries) == 0 {
		return
	}
	// A cursor with nothing to stand for (no range chosen, no pending read)
	// is the review again: it follows a review changed elsewhere.
	if s.commitKbd.cursor != "" && s.commitKbd.timer == nil && s.commitChoice == nil {
		s.commitKbd.cursor = ""
	}
	for _, mods := range []ui.Modifiers{0, ui.Shift} {
		step := 0
		jump := ""
		switch {
		case list.Shortcut(mods, ui.KeyDown) || (mods == 0 && list.Shortcut(0, ui.KeyJ)):
			step = 1
		case list.Shortcut(mods, ui.KeyUp) || (mods == 0 && list.Shortcut(0, ui.KeyK)):
			step = -1
		case list.Shortcut(mods, ui.KeyHome):
			jump = "first"
		case list.Shortcut(mods, ui.KeyEnd):
			jump = "last"
		}
		if step == 0 && jump == "" {
			continue
		}
		at := slices.IndexFunc(entries, func(e gitworkbench.CommitInfo) bool { return e.Hash == s.commitCursorHash() })
		to := at + step
		switch {
		case jump == "first":
			to = 0
		case jump == "last":
			to = len(entries) - 1
		case at < 0:
			to = 0
		}
		to = min(max(to, 0), len(entries)-1)
		s.commitMoveTo(entries, to, mods)
		return
	}
}

// commitMoveTo puts the cursor on entries[to]. A plain move clears any
// chosen range and lets the review follow once the cursor rests; a ⇧ move
// grows the range from its anchor and leaves the review where it is.
func (s *Shell) commitMoveTo(entries []gitworkbench.CommitInfo, to int, mods ui.Modifiers) {
	hash := entries[to].Hash
	s.commitsList.ScrollIntoView(to)
	if !s.pickCommit(entries, hash, mods) {
		s.commitKbd.cursor = hash
		return
	}
	s.commitKbd.cursor = hash
	if s.commitKbd.timer != nil {
		s.commitKbd.timer.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(commitFollowWait, func() {
		s.applyOnUI(func() {
			if s.commitKbd.timer != timer {
				return // a later move owns the read
			}
			s.commitKbd.timer = nil
			if s.commitKbd.cursor != hash {
				return
			}
			s.reviewCommit(hash)
		})
	})
	s.commitKbd.timer = timer
}

// diffFocusWindow is how long a request for the diff to take the keyboard
// stays good: a commit's diff reads for a moment before it is there to focus.
const diffFocusWindow = 1500 * time.Millisecond

// enterDiffFrom is Enter on a list that opens a diff (Changes, Commits): the
// keyboard moves into the diff, and Esc there gives it back to this list.
func (s *Shell) enterDiffFrom(list ui.Element, state *ui.ListState) {
	if list.Shortcut(0, ui.KeyEnter) {
		s.keyBack = state
		s.diffFocusAt = timeNow()
	}
}

// takeDiffFocus gives the keyboard to the diff host once, when an Enter asked
// for it and the diff is on screen; Esc in the diff asks the list back.
func (s *Shell) takeDiffFocus(host ui.Element) {
	if !s.diffFocusAt.IsZero() {
		if timeSince(s.diffFocusAt) < diffFocusWindow {
			host.Focus()
		}
		s.diffFocusAt = time.Time{}
	}
	if host.Shortcut(0, ui.KeyEscape) && !s.diffRangeCancel() && s.keyBack != nil {
		s.keyBackNow = true
	}
}

// restoreListFocus returns the keyboard to state's list after Esc in the diff.
func (s *Shell) restoreListFocus(c *ui.Context, state *ui.ListState) {
	if s.keyBackNow && s.keyBack == state {
		state.Focus(c)
		s.keyBack, s.keyBackNow = nil, false
	}
}
