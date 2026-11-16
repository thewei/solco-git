package nativeui

/**
 * [INPUT]: 依赖 keymap.go 的 keyAction 与 keymapPass、changes_panel.go 的 gdChangedTreeList（在绑键前交出这一次的列表范围）、git_operations.go 里右键菜单与按钮已经在调的 stageFiles / unstageFiles / discardChanges / ignorePaths、git_stash_dialogs.go 的 openSaveStashDialog、openFileInEditor / openExternalPathParent
 * [OUTPUT]: 对外提供 changesScope（Unstaged / Staged 列表这一帧的窗格、行、光标与多选）、Shell.keyScope 的取用（keyTargets / keyPaneFiles / keyChangesReady）、changesKeyRows（Changes 上下文的键位行：Space a d s i ⇧I e o c A）
 * [POS]: LazyGit 的 Files 面板键位落在 Changes 列表上（2026-11-16 决定，第 2 期）：目标 = 多选，没有多选就是光标行（文件夹就是它下面的文件）；动作只调菜单已经在调的方法，所以删除仍先弹确认并留恢复点；提交类的键只把焦点交给快速提交条的主题输入框，真正的提交仍走它的按钮
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// changesScope is what the Changes keys act on for the list being built: the
// pane, its rows, the cursor and the multi-choice. It is set around one
// keymapPass and nil outside it.
type changesScope struct {
	pane     gdViewSide
	rows     []gitworkbench.FlatRow
	selected int
	choice   *ui.Selection[string]
}

// keyChangesReady reports a working-tree pane (not a reviewed commit's files)
// with an idle repository: the state the writing keys need.
func keyChangesReady(s *Shell) bool {
	sc := s.keyScope
	return sc != nil && sc.choice != nil && !s.gdReviewingCommit() && repoReady(s) && !s.git.opBusy
}

// keyTargets lists the files a key acts on, in snapshot order: the multi-choice
// when there is one, else the cursor row (a folder stands for the files under
// it).
func (s *Shell) keyTargets() []string {
	sc, snap := s.keyScope, s.gitSnapshot()
	if sc == nil || snap == nil {
		return nil
	}
	if sc.choice != nil && sc.choice.Len() > 0 {
		var paths []string
		for _, cf := range snap.Files {
			if sc.choice.Has(cf.Path) {
				paths = append(paths, cf.Path)
			}
		}
		return paths
	}
	if sc.selected < 0 || sc.selected >= len(sc.rows) {
		return nil
	}
	node := sc.rows[sc.selected].Node
	if !node.Dir {
		return []string{node.Path}
	}
	return gitDirectoryPaths(snap, node.Path, sc.pane)
}

// keyPaneFiles lists every file the pane shows (its filter applied).
func (s *Shell) keyPaneFiles() []string {
	var paths []string
	for _, r := range s.keyScope.rows {
		if !r.Node.Dir {
			paths = append(paths, r.Node.Path)
		}
	}
	return paths
}

// keyUntracked keeps the targets that are untracked: the only files an ignore
// rule is for.
func (s *Shell) keyUntracked(paths []string) []string {
	snap := s.gitSnapshot()
	if snap == nil {
		return nil
	}
	var out []string
	for _, p := range paths {
		for _, cf := range snap.Files {
			if cf.Path == p && cf.Untracked {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// keyCommitReady reports the quick commit bar being on screen, so a key can
// hand it the keyboard.
func keyCommitReady(s *Shell) bool {
	snap := s.gitSnapshot()
	return repoReady(s) && snap != nil && len(snap.Files) > 0 && !s.gdReviewingCommit() &&
		!(s.git.sequencerChecked && s.git.sequencer.Active())
}

// changesKeyRows are the Changes context's keys; keymap appends them.
var changesKeyRows = []keyAction{
	{ctx: keysChanges, mods: 0, key: ui.KeySpace, caps: []string{"Space"}, label: "Stage / unstage", when: keyChangesReady,
		run: func(s *Shell, _ keyContext) {
			paths := s.keyTargets()
			switch s.keyScope.pane {
			case gdViewUnstaged:
				s.stageFiles(paths)
			case gdViewStaged:
				s.unstageFiles(paths)
			}
		}},
	{ctx: keysChanges, mods: 0, key: ui.KeyA, caps: []string{"A"}, label: "Stage / unstage every file of the list", when: keyChangesReady,
		run: func(s *Shell, _ keyContext) {
			paths := s.keyPaneFiles()
			switch s.keyScope.pane {
			case gdViewUnstaged:
				s.stageFiles(paths)
			case gdViewStaged:
				s.unstageFiles(paths)
			}
		}},
	{ctx: keysChanges, mods: 0, key: ui.KeyD, caps: []string{"D"}, label: "Discard changes… (asks first)",
		when: func(s *Shell) bool { return keyChangesReady(s) && s.keyScope.pane == gdViewUnstaged },
		run:  func(s *Shell, _ keyContext) { s.discardChanges(s.keyTargets()) }},
	{ctx: keysChanges, mods: 0, key: ui.KeyS, caps: []string{"S"}, label: "Stash changes…", when: keyChangesReady,
		run: func(s *Shell, _ keyContext) { s.openSaveStashDialog() }},
	{ctx: keysChanges, mods: 0, key: ui.KeyI, caps: []string{"I"}, label: "Ignore (.gitignore)",
		when: func(s *Shell) bool { return keyChangesReady(s) && len(s.keyUntracked(s.keyTargets())) > 0 },
		run: func(s *Shell, _ keyContext) {
			s.ignorePaths(s.keyUntracked(s.keyTargets()), gitworkbench.IgnoreInRepository)
		}},
	{ctx: keysChanges, mods: ui.Shift, key: ui.KeyI, caps: []string{"⇧", "I"}, label: "Ignore on this clone only (info/exclude)",
		when: func(s *Shell) bool { return keyChangesReady(s) && len(s.keyUntracked(s.keyTargets())) > 0 },
		run: func(s *Shell, _ keyContext) {
			s.ignorePaths(s.keyUntracked(s.keyTargets()), gitworkbench.IgnoreLocally)
		}},
	{ctx: keysChanges, mods: 0, key: ui.KeyE, caps: []string{"E"}, label: "Open in the editor",
		when: func(s *Shell) bool { return keyChangesReady(s) && len(s.keyTargets()) > 0 },
		run:  func(s *Shell, _ keyContext) { s.openFileInEditor(s.keyTargets()[0]) }},
	{ctx: keysChanges, mods: 0, key: ui.KeyO, caps: []string{"O"}, label: "Show in Finder",
		when: func(s *Shell) bool { return keyChangesReady(s) && len(s.keyTargets()) > 0 },
		run: func(s *Shell, _ keyContext) {
			if full := joinRepoPath(s.gitRoot(), s.keyTargets()[0]); full != "" {
				s.openExternalPathParent(full)
			}
		}},
	{ctx: keysChanges, mods: 0, key: ui.KeyC, caps: []string{"C"}, label: "Write the commit message", when: keyCommitReady,
		run: func(s *Shell, _ keyContext) { s.keyFocusSubject = true }},
	{ctx: keysChanges, mods: ui.Shift, key: ui.KeyA, caps: []string{"⇧", "A"}, label: "Amend the last commit", when: keyCommitReady,
		run: func(s *Shell, _ keyContext) {
			s.gitQuickAmend = true
			s.keyFocusSubject = true
		}},
}
