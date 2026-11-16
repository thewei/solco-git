package nativeui

/**
 * [INPUT]: 依赖 keymap.go 的 keyAction、keymap_nav.go 的 commitCursorHash、git_history_ui.go 的 commitChoiceHashes / openHistoryRewrite / openReword / openCherryPickMany、git_operations.go 与 git_fork_more*.go 里右键菜单已经在调的提交与引用动作、git_ref_tree.go 的 gitTreeRow（把每一行的身份登记进 Shell.refRows）、list_nav.go 的 listNav.cursor
 * [OUTPUT]: 对外提供 refTarget 与 Shell.refCursor（引用树里光标所在的那一行是什么）、commitKeyTargets（光标提交 / 多选）、historyKeyRows（Commits 与 References 上下文的键位行）
 * [POS]: LazyGit 的 Commits / Branches 面板键位落在 Commits 列表与引用树上（2026-11-16 决定，第 3 期）：每个键只调右键菜单已经在调的方法，所以重写历史的键先出预览表、丢弃与删除的键先弹确认；多选时只有对一批提交有意义的键（s f d c ⌥↑ ⌥↓）作用，其余单提交键不动；LazyGit 里 c / V 的「复制再粘贴」在这里就是一个 c（Cherry-pick…）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// ── commits ─────────────────────────────────────────────────────────────────

// commitKeyTargets is what a Commits key acts on: several chosen commits (the
// cursor one among them), else the one commit under the cursor. one is the
// cursor commit when the choice is a single commit.
func (s *Shell) commitKeyTargets() (chosen []string, one *gitworkbench.CommitInfo) {
	if s.git == nil {
		return nil, nil
	}
	cursor := s.commitCursorHash()
	if hashes := s.commitChoiceHashes(s.git.commits); len(hashes) > 1 && (cursor == "" || s.commitChosen(cursor)) {
		return hashes, nil
	}
	for i := range s.git.commits {
		if s.git.commits[i].Hash == cursor {
			return []string{cursor}, &s.git.commits[i]
		}
	}
	return nil, nil
}

func commitsIdle(s *Shell) bool { return repoReady(s) && !s.git.opBusy }

// aCommit: the keys that act on one commit.
func aCommit(s *Shell) bool {
	if !commitsIdle(s) {
		return false
	}
	_, one := s.commitKeyTargets()
	return one != nil
}

// someCommits: the keys that act on one commit or a chosen run.
func someCommits(s *Shell) bool {
	if !commitsIdle(s) {
		return false
	}
	chosen, _ := s.commitKeyTargets()
	return len(chosen) > 0
}

func historyKey(act gitworkbench.HistoryAction, offset int) func(*Shell, keyContext) {
	return func(s *Shell, _ keyContext) {
		chosen, _ := s.commitKeyTargets()
		s.openHistoryRewrite(gitworkbench.HistoryOp{Action: act, Hashes: chosen, Offset: offset})
	}
}

// ── references ──────────────────────────────────────────────────────────────

// refTarget is what a row of the references tree stands for, recorded as the
// row is drawn so a key can ask what the keyboard's cursor is on.
type refTarget struct {
	kind    string // "Branch", "Remote branch", "Tag", "Stash", "Worktree", "Folder"
	name    string // the branch, remote-tracking ref, tag name or stash ref
	current bool
}

// refCursor is the row the keyboard is on in the references tree.
func (s *Shell) refCursor() (refTarget, bool) {
	t, ok := s.refRows[s.nav.gitRefs.cursor]
	return t, ok && t.name != ""
}

func refIs(kinds ...string) func(*Shell) bool {
	return func(s *Shell) bool {
		if !repoReady(s) || s.git.opBusy {
			return false
		}
		t, ok := s.refCursor()
		if !ok {
			return false
		}
		for _, k := range kinds {
			if t.kind == k {
				return true
			}
		}
		return false
	}
}

// refNotCurrent is refIs for a branch that is not the checked-out one.
func refNotCurrent(kinds ...string) func(*Shell) bool {
	is := refIs(kinds...)
	return func(s *Shell) bool {
		t, _ := s.refCursor()
		return is(s) && !t.current
	}
}

func (s *Shell) refStash(ref string) (gitworkbench.Stash, bool) {
	for _, st := range s.git.stashes {
		if st.Ref == ref {
			return st, true
		}
	}
	return gitworkbench.Stash{}, false
}

// historyKeyRows are the Commits and References contexts' keys; keymap appends them.
var historyKeyRows = []keyAction{
	// Commits.
	{ctx: keysCommits, mods: 0, key: ui.KeySpace, caps: []string{"Space"}, label: "Check out the commit", when: aCommit,
		run: func(s *Shell, _ keyContext) { _, one := s.commitKeyTargets(); s.checkoutCommitRow(*one) }},
	{ctx: keysCommits, mods: 0, key: ui.KeyN, caps: []string{"N"}, label: "New branch here…", when: aCommit,
		run: func(s *Shell, _ keyContext) {
			_, one := s.commitKeyTargets()
			s.openCreateAt("git-create-branch-at", one.Hash)
		}},
	{ctx: keysCommits, mods: ui.Shift, key: ui.KeyT, caps: []string{"⇧", "T"}, label: "New tag here…", when: aCommit,
		run: func(s *Shell, _ keyContext) { _, one := s.commitKeyTargets(); s.openTagDialog(one.Hash) }},
	{ctx: keysCommits, mods: 0, key: ui.KeyT, caps: []string{"T"}, label: "Revert the commit…", when: aCommit,
		run: func(s *Shell, _ keyContext) {
			_, one := s.commitKeyTargets()
			s.confirmRevertCommit(one.Hash, one.Subject)
		}},
	{ctx: keysCommits, mods: 0, key: ui.KeyC, caps: []string{"C"}, label: "Cherry-pick…", when: someCommits,
		run: func(s *Shell, _ keyContext) {
			if chosen, one := s.commitKeyTargets(); one != nil {
				s.confirmCherryPickCommit(one.Hash, one.Subject)
			} else {
				s.openCherryPickMany(chosen)
			}
		}},
	{ctx: keysCommits, mods: 0, key: ui.KeyR, caps: []string{"R"}, label: "Reword…", when: aCommit,
		run: func(s *Shell, _ keyContext) { _, one := s.commitKeyTargets(); s.openReword(*one) }},
	{ctx: keysCommits, mods: 0, key: ui.KeyS, caps: []string{"S"}, label: "Squash into the commit below…", when: someCommits,
		run: historyKey(gitworkbench.HistorySquash, 0)},
	{ctx: keysCommits, mods: ui.Shift, key: ui.KeyF, caps: []string{"⇧", "F"}, label: "Fixup into the commit below…", when: someCommits,
		run: historyKey(gitworkbench.HistoryFixup, 0)},
	{ctx: keysCommits, mods: 0, key: ui.KeyD, caps: []string{"D"}, label: "Drop…", when: someCommits,
		run: historyKey(gitworkbench.HistoryDrop, 0)},
	{ctx: keysCommits, mods: ui.Alt, key: ui.KeyUp, caps: []string{"⌥", "↑"}, label: "Move the commit up…", when: someCommits,
		run: historyKey(gitworkbench.HistoryMove, -1)},
	{ctx: keysCommits, mods: ui.Alt, key: ui.KeyDown, caps: []string{"⌥", "↓"}, label: "Move the commit down…", when: someCommits,
		run: historyKey(gitworkbench.HistoryMove, 1)},
	{ctx: keysCommits, mods: 0, key: ui.KeyY, caps: []string{"Y"}, label: "Copy the commit hash", when: aCommit,
		run: func(s *Shell, _ keyContext) { _, one := s.commitKeyTargets(); s.copyToClipboard(one.Hash) }},

	// References.
	{ctx: keysRefs, mods: 0, key: ui.KeySpace, caps: []string{"Space"}, label: "Check out / apply",
		when: func(s *Shell) bool { return refNotCurrent("Branch", "Remote branch")(s) || refIs("Stash")(s) },
		run: func(s *Shell, _ keyContext) {
			t, _ := s.refCursor()
			switch t.kind {
			case "Branch":
				s.confirmSwitchBranch(t.name)
			case "Remote branch":
				s.checkoutRemoteBranch(t.name)
			case "Stash":
				if st, ok := s.refStash(t.name); ok {
					s.openApplyStashDialog(st)
				}
			}
		}},
	{ctx: keysRefs, mods: 0, key: ui.KeyN, caps: []string{"N"}, label: "New branch… (a tag on a tag)",
		when: func(s *Shell) bool { return repoReady(s) && !s.git.opBusy },
		run: func(s *Shell, _ keyContext) {
			if t, ok := s.refCursor(); ok && t.kind == "Tag" {
				s.openTagDialog("")
				return
			}
			s.openNewBranchDialog()
		}},
	{ctx: keysRefs, mods: ui.Shift, key: ui.KeyM, caps: []string{"⇧", "M"}, label: "Merge into the current branch…",
		when: refNotCurrent("Branch", "Remote branch"),
		run:  func(s *Shell, _ keyContext) { t, _ := s.refCursor(); s.openMergeDialog(t.name) }},
	{ctx: keysRefs, mods: 0, key: ui.KeyR, caps: []string{"R"}, label: "Rebase the current branch onto it…",
		when: refNotCurrent("Branch", "Remote branch"),
		run:  func(s *Shell, _ keyContext) { t, _ := s.refCursor(); s.confirmRebaseOnto(t.name) }},
	{ctx: keysRefs, mods: ui.Shift, key: ui.KeyR, caps: []string{"⇧", "R"}, label: "Rename the branch / stash…", when: refIs("Branch", "Stash"),
		run: func(s *Shell, _ keyContext) {
			t, _ := s.refCursor()
			if t.kind == "Branch" {
				s.openRenameBranch(t.name)
			} else if st, ok := s.refStash(t.name); ok {
				s.openRenameStash(st)
			}
		}},
	{ctx: keysRefs, mods: 0, key: ui.KeyD, caps: []string{"D"}, label: "Delete… (asks first)",
		when: func(s *Shell) bool { return refNotCurrent("Branch", "Remote branch")(s) || refIs("Tag", "Stash")(s) },
		run: func(s *Shell, _ keyContext) {
			t, _ := s.refCursor()
			switch t.kind {
			case "Branch":
				s.deleteBranch(t.name)
			case "Remote branch":
				s.confirmDeleteRemoteBranch(t.name)
			case "Tag":
				s.deleteTagConfirm(t.name)
			case "Stash":
				s.dropStash(t.name)
			}
		}},
	{ctx: keysRefs, mods: 0, key: ui.KeyY, caps: []string{"Y"}, label: "Copy the name", when: refIs("Branch", "Remote branch", "Tag", "Stash"),
		run: func(s *Shell, _ keyContext) { t, _ := s.refCursor(); s.copyToClipboard(t.name) }},
}
