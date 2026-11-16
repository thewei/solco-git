package nativeui

/**
 * [INPUT]: 依赖 keymap.go 的 keyAction、gd_find.go 的 j / k 所选的 gdSelFile / gdSelHunk、gd_stage_lines.go 的 gdStageSide / gdPickedIn / gdMoveLines（点行号挑出的行）、changes_panel.go 的 gdRowAction（整文件的暂存 / 取消暂存）
 * [OUTPUT]: 对外提供 diffKeyTarget（j / k 当前所选的那个 hunk 与它能走哪个方向）、diffKeyRows（Diff 上下文里会写 Git 的键：Space 暂存 / 取消暂存所选 hunk 或点出的行，A 暂存 / 取消暂存整个文件）
 * [POS]: LazyGit 暂存视图的键位落在 diff 上（2026-11-16 决定，第 4 期）：「所选的 hunk」就是 J / K 已经在维护的那一个，所以键盘走一遍 hunk 再按 Space 就是 LazyGit 的逐块暂存；动作与 hunk 行上的「Stage Hunk / Stage N Lines」按钮是同一个 gdMoveLines，已查看的提交里 gdStageSide 不放行，所以这些键在那里不作用
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import "github.com/egoist/mygo/ui"

// diffKeyTarget is the hunk J / K chose and the way it can move: unstage is
// true for the staged side. ok is false when nothing is chosen, the file
// cannot be staged by lines, or the diff is a commit's.
func (s *Shell) diffKeyTarget() (f *gdFile, hunk int, unstage, ok bool) {
	if s.git == nil {
		return nil, 0, false, false
	}
	fi, hi := s.git.gdSelFile, s.git.gdSelHunk
	if fi < 0 || fi >= len(s.git.gdFiles) || hi < 0 {
		return nil, 0, false, false
	}
	f = s.git.gdFiles[fi]
	if hi >= len(f.hunks()) {
		return nil, 0, false, false
	}
	unstage, ok = s.gdStageSide(f)
	return f, hi, unstage, ok
}

func diffHunkReady(s *Shell) bool {
	if !repoReady(s) || s.git.opBusy {
		return false
	}
	_, _, _, ok := s.diffKeyTarget()
	return ok
}

// diffFileReady: a file is chosen (a collapsed one counts) whose side has a
// Stage / Unstage action.
func diffFileReady(s *Shell) bool {
	if !repoReady(s) || s.git.opBusy || s.gdReviewingCommit() {
		return false
	}
	fi := s.git.gdSelFile
	if fi < 0 || fi >= len(s.git.gdFiles) {
		return false
	}
	f := s.git.gdFiles[fi]
	action, _, _ := s.gdRowAction(f.cf, f.view)
	return action != nil
}

// diffKeyRows are the Diff context's writing keys; keymap appends them.
var diffKeyRows = []keyAction{
	{ctx: keysDiff, mods: 0, key: ui.KeySpace, caps: []string{"Space"}, label: "Stage / unstage the hunk (or the picked lines)", when: diffHunkReady,
		run: func(s *Shell, _ keyContext) {
			f, hunk, unstage, _ := s.diffKeyTarget()
			lines := s.gdPickedIn(f, hunk)
			if len(lines) == 0 {
				lines = gdChangedLines(f.hunks()[hunk])
			}
			s.gdMoveLines(f, hunk, lines, unstage)
		}},
	{ctx: keysDiff, mods: 0, key: ui.KeyA, caps: []string{"A"}, label: "Stage / unstage the whole file", when: diffFileReady,
		run: func(s *Shell, _ keyContext) {
			f := s.git.gdFiles[s.git.gdSelFile]
			if action, _, _ := s.gdRowAction(f.cf, f.view); action != nil {
				action([]string{f.cf.Path})
			}
		}},
}
