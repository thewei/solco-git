package nativeui

/**
 * [INPUT]: 依赖 keymap_diff.go 的 diffKeyTarget / diffHunkReady、gd_stage_lines.go 的 gdPickRange 与 git.gdPick（点行号挑行用的同一份状态）、gd_find.go 的 gdSelFile / gdSelHunk（J / K 所选）、gd_rows.go 的行表（滚到那一行）
 * [OUTPUT]: 对外提供 diffLineCursor（行光标：落在 J / K 所选 hunk 的某个改动行上，可带一个范围）、Shell.diffLines、diffLineRows（↑ ↓ v）与 diffRangeCancel（Esc 先收范围）
 * [POS]: LazyGit 暂存视图的逐行挑选（2026-11-16 决定，第 4 期续）：光标只停在改动行（加 / 删）上，不停在上下文行；光标所在的行就是「点出的行」，所以 Space（keymap_diff.go）不用改——挑出的行优先、否则整个 hunk；光标走到 hunk 尽头会进入相邻的 hunk；范围（v 起头，↑ ↓ 延伸，Esc 收回）只在同一个 hunk 里延伸，与点行号时 ⇧ 的限制一致；⇧↑ ⇧↓ 不绑：窗口把它们当文本选区命令先吃掉，Shortcut 收不到；光标属于一张快照与一个 hunk，暂存之后快照换了，它就失效，J / K 所选的 hunk 还在，再按 ↓ 从该处第一个改动行重新开始
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// diffLineCursor is the keyboard's line in the chosen hunk. line is an index
// into the hunk's lines (-1 none); anchor is the other end of a range.
type diffLineCursor struct {
	snap         *gitworkbench.ChangesSnapshot
	file, hunk   int
	line, anchor int
	ranging      bool // v: ↑ ↓ extend the range from the anchor
}

// live reports whether the cursor still belongs to the hunk J / K chose in
// this snapshot.
func (s *Shell) diffCursorLive() bool {
	c := &s.diffLines
	return c.snap != nil && c.line >= 0 && c.snap == s.git.snapshot && c.file == s.git.gdSelFile && c.hunk == s.git.gdSelHunk
}

// changedIn lists the changed (added / deleted) line indices of a hunk, in order.
func changedIn(h gitworkbench.Hunk) []int {
	var out []int
	for i := range gdChangedLines(h) {
		out = append(out, i)
	}
	slices.Sort(out)
	return out
}

// diffLineMove is ↑ / ↓ (dir -1 / 1). The cursor starts
// at the hunk's first changed line going down, its last going up; at either
// end it goes on into the neighbouring hunk — unless a range is being drawn,
// which stays inside the hunk.
func (s *Shell) diffLineMove(dir int) {
	f, hunk, _, ok := s.diffKeyTarget()
	if !ok {
		return
	}
	cur := &s.diffLines
	changed := changedIn(f.hunks()[hunk])
	if len(changed) == 0 {
		return
	}
	if !s.diffCursorLive() {
		*cur = diffLineCursor{snap: s.git.snapshot, file: s.git.gdSelFile, hunk: hunk, line: changed[0]}
		if dir < 0 {
			cur.line = changed[len(changed)-1]
		}
		cur.anchor = cur.line
		s.diffLinePick(f)
		return
	}
	pos := slices.Index(changed, cur.line)
	next := pos + dir
	staying := cur.ranging // the anchor holds still while a range is drawn
	switch {
	case next >= 0 && next < len(changed):
		cur.line = changed[next]
	case staying:
		return // a range stays in its hunk
	default:
		nh := hunk + dir
		if nh < 0 || nh >= len(f.hunks()) {
			return
		}
		other := changedIn(f.hunks()[nh])
		if len(other) == 0 {
			return
		}
		s.git.gdSelHunk = nh
		cur.hunk, cur.line = nh, other[0]
		if dir < 0 {
			cur.line = other[len(other)-1]
		}
	}
	if !staying {
		cur.anchor = cur.line
	}
	s.diffLinePick(f)
}

// diffLinePick makes the picked lines the range between the anchor and the
// cursor (just the cursor's line when they are one) and brings it into view.
func (s *Shell) diffLinePick(f *gdFile) {
	cur := &s.diffLines
	s.gdPickRange(f, cur.hunk, cur.anchor, cur.line)
	p := &s.git.gdPick
	p.anchor, p.anchored = [2]int{cur.hunk, cur.anchor}, true
	s.git.gdCurrent = cur.file
	s.gdScrollToLine(cur.file, cur.hunk, cur.line)
}

// gdScrollToLine brings one hunk line into view.
func (s *Shell) gdScrollToLine(file, hunk, line int) {
	if s.git.gdRowsDirty {
		s.buildGdRows()
	}
	for i := range s.git.gdRows {
		r := &s.git.gdRows[i]
		if int(r.file) == file && r.kind == gdRowLine && int(r.hunk) == hunk && (int(r.a) == line || int(r.b) == line) {
			s.git.gdList.ScrollTo(i, ui.Center)
			return
		}
	}
}

// diffRangeToggle is v: start a range at the cursor's line (the cursor starts
// at the hunk's first changed line when it is not on one), or end the range
// and keep what it picked.
func (s *Shell) diffRangeToggle() {
	f, hunk, _, ok := s.diffKeyTarget()
	if !ok {
		return
	}
	cur := &s.diffLines
	if !s.diffCursorLive() {
		changed := changedIn(f.hunks()[hunk])
		if len(changed) == 0 {
			return
		}
		*cur = diffLineCursor{snap: s.git.snapshot, file: s.git.gdSelFile, hunk: hunk, line: changed[0]}
	}
	cur.ranging = !cur.ranging
	if cur.ranging {
		cur.anchor = cur.line
	}
	s.diffLinePick(f)
}

// diffRangeCancel is Esc while a range is being drawn: the range collapses to
// the cursor's line and the keyboard stays in the diff. It reports whether it
// used the key.
func (s *Shell) diffRangeCancel() bool {
	cur := &s.diffLines
	if !s.diffCursorLive() || !cur.ranging {
		return false
	}
	f, _, _, ok := s.diffKeyTarget()
	if !ok {
		return false
	}
	cur.ranging, cur.anchor = false, cur.line
	s.diffLinePick(f)
	return true
}

// diffLineRows are the Diff context's line keys; keymap appends them.
var diffLineRows = []keyAction{
	{ctx: keysDiff, mods: 0, key: ui.KeyDown, caps: []string{"↓"}, label: "Next changed line (picks it)", when: diffHunkReady,
		run: func(s *Shell, _ keyContext) { s.diffLineMove(1) }},
	{ctx: keysDiff, mods: 0, key: ui.KeyUp, caps: []string{"↑"}, label: "Previous changed line (picks it)", when: diffHunkReady,
		run: func(s *Shell, _ keyContext) { s.diffLineMove(-1) }},
	{ctx: keysDiff, mods: 0, key: ui.KeyV, caps: []string{"V"}, label: "Pick a range of lines (↑ ↓ extend it; Esc ends it)", when: diffHunkReady,
		run: func(s *Shell, _ keyContext) { s.diffRangeToggle() }},
}
