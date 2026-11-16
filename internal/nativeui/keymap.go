package nativeui

/**
 * [INPUT]: 依赖 mygo/ui 的 Element.Shortcut（焦点在该元素之内才触发，输入框取走文本时不会触发）、Shell 上已有的 Git 动作（fetchRepo / pullRepo / pushRepo / requestReplay）与各列表的搜索开关
 * [OUTPUT]: 对外提供 keyContext（全局 / Changes / Commits / Refs / Files / 提交文件 / Diff）、keyAction 与 keymap（键位的单一来源：键、键帽、标签、动作）、Shell.keymapPass（把某个上下文的键绑到一个列表元素上）、keysIn（`?` 速查浮层读同一张表）
 * [POS]: LazyGit 式「按上下文的单字母键」的落点（2026-11-16 决定「Solco Git 的键盘交互」）：动作只调右键菜单与按钮已经在调的 Shell 方法，所以确认框、预览表与 Safety Journal 绕不过；字母键只在列表（或 diff）有焦点时触发，从不进菜单加速键，否则会在输入框里抢走打字；菜单加速键（shortcuts.go）与这里互不重叠
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// keyContext is the place a key answers in: the list or surface that holds the
// keyboard focus.
type keyContext uint8

const (
	keysAny         keyContext = iota // every list below
	keysChanges                       // the Unstaged / Staged lists
	keysCommits                       // the commit list
	keysRefs                          // the references tree
	keysFiles                         // the Files tree
	keysCommitFiles                   // the file tree of a reviewed commit
	keysDiff                          // the diff review
)

func (k keyContext) title() string {
	switch k {
	case keysChanges:
		return "Changes"
	case keysCommits:
		return "Commits"
	case keysRefs:
		return "References"
	case keysFiles:
		return "Files"
	case keysCommitFiles:
		return "Commit files"
	case keysDiff:
		return "Diff"
	}
	return "Anywhere"
}

// keyAction is one row of the keymap. A row with no run is documentation of a
// key the list itself answers (the arrows, Home, End): the keyboard sheet
// shows it, nothing here fires it.
type keyAction struct {
	ctx   keyContext
	mods  ui.Modifiers
	key   ui.Key
	caps  []string
	label string
	// when reports whether the key does anything now; nil means always.
	when func(s *Shell) bool
	run  func(s *Shell, ctx keyContext)
}

func repoReady(s *Shell) bool { return s.git != nil && s.git.root != "" }

// keymap is the one table the keys are bound from and the `?` sheet is read
// from. A new key is a new row here, and an action that already exists.
var keymap = slices.Concat(keymapBase, changesKeyRows, historyKeyRows)

var keymapBase = []keyAction{
	// Anywhere a list holds the keyboard.
	{ctx: keysAny, mods: 0, key: ui.KeyF, caps: []string{"F"}, label: "Fetch", when: repoReady,
		run: func(s *Shell, _ keyContext) { s.fetchRepo() }},
	{ctx: keysAny, mods: 0, key: ui.KeyP, caps: []string{"P"}, label: "Pull (fast-forward only)", when: repoReady,
		run: func(s *Shell, _ keyContext) { s.pullRepo() }},
	{ctx: keysAny, mods: ui.Shift, key: ui.KeyP, caps: []string{"⇧", "P"}, label: "Push", when: repoReady,
		run: func(s *Shell, _ keyContext) { s.pushRepo() }},
	{ctx: keysAny, mods: 0, key: ui.KeyZ, caps: []string{"Z"}, label: "Undo the last Git action…", when: repoReady,
		run: func(s *Shell, _ keyContext) { s.requestReplay(false) }},
	{ctx: keysAny, mods: ui.Shift, key: ui.KeyZ, caps: []string{"⇧", "Z"}, label: "Redo the undone Git action…", when: repoReady,
		run: func(s *Shell, _ keyContext) { s.requestReplay(true) }},
	{ctx: keysAny, mods: ui.Shift, key: ui.KeySlash, caps: []string{"?"}, label: "Keyboard shortcuts",
		run: func(s *Shell, ctx keyContext) { s.openKeyHelp(ctx) }},

	// The lists' own movement (answered by the list; shown here).
	{ctx: keysChanges, caps: []string{"↑", "↓"}, label: "Move between files (J K too)"},
	{ctx: keysChanges, caps: []string{"⇧", "↑", "↓"}, label: "Choose several files"},
	{ctx: keysChanges, caps: []string{"↩"}, label: "Open the diff"},
	{ctx: keysChanges, mods: 0, key: ui.KeySlash, caps: []string{"/"}, label: "Filter files",
		run: func(s *Shell, _ keyContext) { s.changesSearchOpen = true }},

	{ctx: keysCommits, caps: []string{"↑", "↓"}, label: "Move between commits (J K too)"},
	{ctx: keysCommits, caps: []string{"⇧", "↑", "↓"}, label: "Choose a run of commits"},
	{ctx: keysCommits, caps: []string{"Home", "End"}, label: "First / last commit"},
	{ctx: keysCommits, caps: []string{"↩"}, label: "Open the diff"},
	{ctx: keysCommits, mods: 0, key: ui.KeySlash, caps: []string{"/"}, label: "Search commits",
		run: func(s *Shell, _ keyContext) { s.gitCommitSearchOpen = true }},

	{ctx: keysRefs, caps: []string{"↑", "↓"}, label: "Move between references (J K too)"},
	{ctx: keysRefs, caps: []string{"←", "→"}, label: "Fold / unfold a group"},
	{ctx: keysRefs, caps: []string{"↩"}, label: "Act on the reference"},
	{ctx: keysRefs, mods: 0, key: ui.KeySlash, caps: []string{"/"}, label: "Filter references",
		run: func(s *Shell, _ keyContext) { s.gitRefsSearchOpen = true }},

	{ctx: keysFiles, caps: []string{"↑", "↓"}, label: "Move between files (J K too)"},
	{ctx: keysFiles, caps: []string{"←", "→"}, label: "Fold / unfold a folder"},
	{ctx: keysFiles, caps: []string{"↩"}, label: "Open the file"},
	{ctx: keysFiles, mods: 0, key: ui.KeySlash, caps: []string{"/"}, label: "Search files",
		run: func(s *Shell, _ keyContext) { s.filesSearch.open = true }},

	{ctx: keysCommitFiles, caps: []string{"↑", "↓"}, label: "Move between files (J K too)"},
	{ctx: keysCommitFiles, caps: []string{"←", "→"}, label: "Fold / unfold a folder"},
	{ctx: keysCommitFiles, mods: 0, key: ui.KeySlash, caps: []string{"/"}, label: "Search the commit's files",
		run: func(s *Shell, _ keyContext) { s.gitCommitFilesSearchOpen = true }},

	{ctx: keysDiff, caps: []string{"J", "K"}, label: "Next / previous hunk"},
	{ctx: keysDiff, caps: []string{"⌃", "↑", "↓"}, label: "Previous / next hunk"},
	{ctx: keysDiff, caps: []string{"⌥", "Z"}, label: "Toggle word wrap"},
	{ctx: keysDiff, caps: []string{"Esc"}, label: "Back to the list"},
}

// keysIn lists the rows a keyboard sheet shows for ctx: its own, then the
// ones that answer anywhere.
func keysIn(ctx keyContext) (own, anywhere []keyAction) {
	for _, a := range keymap {
		switch {
		case a.ctx == keysAny:
			anywhere = append(anywhere, a)
		case a.ctx == ctx:
			own = append(own, a)
		}
	}
	return own, anywhere
}

// keymapPass binds ctx's keys (and the ones that answer anywhere) to el: they
// fire only while the keyboard focus is inside el. Every row is asked on
// every pass — that is how a key stays claimed for the next press.
func (s *Shell) keymapPass(el ui.Element, ctx keyContext) {
	for _, a := range keymap {
		if a.run == nil || (a.ctx != ctx && a.ctx != keysAny) {
			continue
		}
		if el.Shortcut(a.mods, a.key) && (a.when == nil || a.when(s)) {
			a.run(s, ctx)
		}
	}
}
