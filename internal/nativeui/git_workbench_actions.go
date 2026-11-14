package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-git/internal/settings"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// gitHeaderToolsShown reports whether the title bar carries the Git tools:
// the local worktree's Changes view only. A historical commit diff is
// read-only and shows no mutation UI.
func (s *Shell) gitHeaderToolsShown() bool {
	return s.git != nil && !s.filesView &&
		s.surface.current() == WorkspaceSurfaceDiff && !s.gdReviewingCommit() && s.gitSnapshot() != nil
}

// gitHeaderTools is the Changes view's share of the title bar's trailing
// tools (2026-10-30; it was a toolbar row above the diff): the file list
// toggle, Fetch/Pull/Push with the ahead/behind counts, and the Split/Unified
// switch. It sits ahead of the page Actions, so Actions keeps its place from
// the right edge on every route. The less common actions are in that Actions
// menu (gitActionItems). The branch selector lives in the repo bar, and the
// Commit entry is the bottom composer.
func (s *Shell) gitHeaderTools(c *ui.Context) {
	sp := Spacing()
	ui.Row(c).Gap(sp.XS).AlignItems(ui.Center).Shrink(0).Children(func() {
		s.gitFilePaneToggle(c)
		s.gdSyncButtons(c, gdPaletteFor(c.Theme()))
		// Split/Unified must remain reachable even when actions compact.
		s.gdLayoutControl(c)
	})
}

// gitActionItems adds the Git actions to the page Actions menu of the
// Changes view: batch stage/unstage, stash, undo, and the way to Commits and
// Git settings.
func (s *Shell) gitActionItems(m *ui.Menu) {
	snap := s.gitSnapshot()
	if snap == nil {
		return
	}
	stage, unstage := gitActionPaths(snap)
	busySequencer := s.git.sequencerChecked && s.git.sequencer.Active()
	canStage := !s.git.opBusy && len(stage) > 0 && (!busySequencer || s.git.sequencer.Unmerged == 0)
	canUnstage := !s.git.opBusy && len(unstage) > 0 && !busySequencer
	if m.Item("Stage All").Disabled(!canStage).Chosen() {
		s.stageFiles(stage)
	}
	if m.Item("Unstage All").Disabled(!canUnstage).Chosen() {
		s.unstageFiles(unstage)
	}
	if m.Item("Hide Untracked Files").Checked(s.changes.hideUntracked).Chosen() {
		s.changes.hideUntracked = !s.changes.hideUntracked
		s.changesTree = nil
		s.git.gdRowsDirty = true
	}
	var unstaged, untracked []string
	for _, f := range snap.Files {
		switch {
		case f.Untracked:
			untracked = append(untracked, f.Path)
		case f.Unstaged && f.Status != gitworkbench.StatusConflicted:
			unstaged = append(unstaged, f.Path)
		}
	}
	idle := s.git.opBusy || busySequencer
	if m.Item("Discard All Unstaged Changes…").Disabled(idle || len(unstaged) == 0).Chosen() {
		s.discardChanges(unstaged)
	}
	if m.Item("Delete All Untracked Files…").Disabled(idle || len(untracked) == 0).Chosen() {
		s.discardChanges(untracked)
	}
	m.Separator()
	canSync := !s.git.opBusy && !busySequencer && snap.Branch != ""
	// The four rarer remote variants share one submenu; plain Fetch / Pull /
	// Push are the toolbar icons.
	m.Submenu("Fetch & Pull Options", func(sub *ui.Menu) {
		if sub.Item("Fetch All Remotes").Disabled(s.git.opBusy || len(s.git.remotes) == 0).Chosen() {
			s.fetchAll(false)
		}
		if sub.Item("Fetch All and Prune").Disabled(s.git.opBusy || len(s.git.remotes) == 0).Chosen() {
			s.fetchAll(true)
		}
		sub.Separator()
		if sub.Item("Pull and Merge").Disabled(!canSync || len(s.git.remotes) == 0).Chosen() {
			s.pullWith(gitworkbench.PullMerge, "Pull and Merge")
		}
		if sub.Item("Pull and Rebase").Disabled(!canSync || len(s.git.remotes) == 0).Chosen() {
			s.pullWith(gitworkbench.PullRebase, "Pull and Rebase")
		}
		sub.Separator()
		if sub.Item("Push Multiple…").Disabled(s.git.opBusy || len(s.git.remotes) == 0).Chosen() {
			s.openMultiPush()
		}
	})
	m.Separator()
	if m.Item("Stash All Changes").Disabled(s.git.opBusy || busySequencer || len(snap.Files) == 0).Chosen() {
		s.stashChanges()
	}
	if m.Item("Stash Staged Changes").Disabled(s.git.opBusy || busySequencer || !s.hasStagedChanges()).Chosen() {
		s.stashMode(gitworkbench.StashStaged)
	}
	if m.Item("Stash Changes, Keep Staged").Disabled(s.git.opBusy || busySequencer || !s.hasStagedChanges() || len(snap.Files) == 0).Chosen() {
		s.stashMode(gitworkbench.StashKeepIndex)
	}
	if m.Item("Save Stash…").Disabled(s.git.opBusy || busySequencer || len(snap.Files) == 0).Chosen() {
		s.openSaveStashDialog()
	}
	if len(s.git.stashes) > 0 {
		m.Submenu("Recent Stashes", func(sub *ui.Menu) {
			s.recentStashItems(sub)
		})
	}
	if m.Item("Save Changes as Patch…").Disabled(s.git.opBusy || len(snap.Files) == 0).Chosen() {
		s.saveWorkingPatch()
	}
	if m.Item("Apply Patch…").Disabled(s.git.opBusy || busySequencer).Chosen() {
		s.applyPatchFile()
	}
	if m.Item("Undo Last Commit…").Disabled(s.git.opBusy || busySequencer || snap.NoHead || snap.Branch == "").Chosen() {
		s.undoLastCommit()
	}
	m.Separator()
	if m.Item("View Commits").Chosen() {
		s.showGitCommits()
	}
	if m.Item("Hide Stashes in Commit List").Checked(s.settings.Workbench.HideStashes).Chosen() {
		hide := !s.settings.Workbench.HideStashes
		s.applySettings(func(st *settings.Settings) error {
			st.Workbench.HideStashes = hide
			return nil
		})
	}
	if m.Item("Reflog…").Chosen() {
		s.openReflogSheet()
	}
	if m.Item("Recovery…").Chosen() {
		s.openRecoverySheet()
	}
	if m.Item("Command Log…").Chosen() {
		s.openCommandLog()
	}
	if m.Item("Add Submodule…").Disabled(s.git.opBusy || busySequencer).Chosen() {
		s.openAddSubmoduleDialog()
	}
	m.Separator()
	if m.Item("Git Settings…").Chosen() && s.w != nil {
		s.w.openSettings()
	}
}

func gitActionPaths(snap *gitworkbench.ChangesSnapshot) (stage, unstage []string) {
	if snap == nil {
		return nil, nil
	}
	for _, file := range snap.Files {
		if file.Untracked || file.Unstaged {
			stage = append(stage, file.Path)
		}
		if file.Staged {
			unstage = append(unstage, file.Path)
		}
	}
	return stage, unstage
}
