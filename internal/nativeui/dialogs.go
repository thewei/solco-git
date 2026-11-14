package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (s *Shell) openTextDialog(kind, target, title, label, value string) {
	s.dialogKind, s.dialogTarget = kind, target
	s.dialogTitle, s.dialogLabel, s.dialogValue = title, label, value
	s.dialogAction = ""
	s.dialogOpen = true
}

// openCreateDialog asks for the name of something new in the current
// repository. The dialog is bound to that repository: a submit after the
// context moved on is refused rather than applied elsewhere.
func (s *Shell) openCreateDialog(kind, title, label string) {
	if s.git == nil || s.git.root == "" {
		return
	}
	s.openTextDialog(kind, s.git.root, title, label, "")
	s.dialogAction = "Create"
}

// openNewBranchDialog starts a new local branch from HEAD.
func (s *Shell) openNewBranchDialog() {
	s.openCreateDialog("git-create-branch", "New Branch", "Branch name")
}

func (s *Shell) openConfirm(kind, target, title, message string) {
	s.confirmKind, s.confirmTarget = kind, target
	s.confirmRepoRoot = ""
	if strings.HasPrefix(kind, "git-") && s.git != nil {
		s.confirmRepoRoot = s.git.root
	}
	s.confirmTitle, s.confirmMessage = title, message
	s.confirmOpen = true
}

// modalTitle is the heading every dialog opens with. One size and weight, so
// Merge, Commit and the text prompts read as the same kind of surface — and
// not as pages: the Title size belongs to the page title in the title bar.
func modalTitle(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(Typography().Heading).FontWeight(WeightSemibold)
}

// gitConfirmActions names what a Git confirm's button does, so it reads
// "Remove" or "Discard" instead of a bare "Confirm".
var gitConfirmActions = map[string]string{
	"git-discard": "Discard", "git-delete-untracked": "Delete", "git-drop-stash": "Delete", "git-stash-clear": "Drop All",
	"git-delete-branch": "Delete", "git-delete-remote-branch": "Delete", "git-delete-tag": "Delete", "git-delete-remote-tag": "Delete",
	"git-merge-ff": "Merge", "git-merge-no-ff": "Merge", "git-merge-squash": "Squash", "git-rebase": "Rebase", "git-rebase-from": "Rebase",
	"git-cherry-pick": "Cherry-pick", "git-revert-commit": "Revert", "git-undo-commit": "Undo", "git-sequencer-abort": "Abort",
	"git-replay-undo": "Undo", "git-replay-redo": "Redo",
	"git-checkout-detached": "Check Out", "git-checkout-remote": "Check Out", "git-restore-file": "Restore",
	"git-remove-worktree": "Remove", "git-remove-submodule": "Remove", "git-remove-remote": "Remove",
}

// confirmActionText is the confirm button's label for a confirm kind.
func confirmActionText(kind string) string {
	if text, ok := gitConfirmActions[kind]; ok {
		return text
	}
	if strings.HasPrefix(kind, "git-reset-") {
		return "Reset"
	}
	return "Confirm"
}

// dialogs renders every modal of the workspace: Merge, the one Commit editor, the
// shared single-field prompt, the Git notice and extras, and the shared
// confirmation. Each is bound to the repository it was opened for.
func (s *Shell) dialogs(c *ui.Context) {
	if s.mergeDialogOpen && (s.git == nil || s.mergeRepoRoot == "" || s.mergeRepoRoot != s.git.root) {
		s.mergeDialogOpen = false
		s.pendingToast = "Repository changed. Reopen Merge for the current repository."
	}
	if s.mergeDialogOpen {
		ui.Modal(c, &s.mergeDialogOpen, func() {
			ui.Column(c).Width(430).MaxWidth(480).Gap(Spacing().L).Children(func() {
				modalTitle(c, "Merge Branch")
				tipped(c, ui.Text(c, s.mergeTarget).FontSize(Typography().Body).FontWeight(WeightSemibold).SingleLine(), s.mergeTarget)
				segmentedControl(c, &s.mergeStrategy, "Fast-forward only", "Create merge commit", "Squash").FillWidth()
				hint := "Moves the branch forward if it has not diverged. No merge commit."
				switch s.mergeStrategy {
				case 1:
					hint = "Creates a merge commit. You may need to resolve conflicts."
				case 2:
					hint = "Brings every change in as staged changes without a merge commit; you commit them yourself."
				}
				ui.Text(c, hint).FontSize(Typography().Caption).TextColor(c.Theme().TextMuted)
				ui.Row(c).FillWidth().Gap(Spacing().M).Justify(ui.End).Children(func() {
					if regularButton(c, "Cancel Merge").Clicked() {
						s.mergeDialogOpen = false
					}
					if regularPrimaryButton(c, "Review Merge").Clicked() {
						target, strategy := s.mergeTarget, s.mergeStrategy
						s.mergeDialogOpen = false
						switch strategy {
						case 1:
							s.confirmMergeNoFFBranch(target)
						case 2:
							s.confirmMergeSquash(target)
						default:
							s.confirmMergeBranch(target)
						}
					}
				})
			})
		})
	}
	// Commit always owns one modal, regardless of which Git entry opened it.
	// MyGo dismisses Modal on backdrop/Escape; we turn such dismissals into
	// the same explicit guarded cancel flow on the next frame.
	if s.surface.current() == WorkspaceSurfaceCommit {
		if !s.commitDialogOpen {
			s.commitDialogOpen = true
			s.requestCancelCommit()
		}
		if s.surface.current() == WorkspaceSurfaceCommit {
			ui.Modal(c, &s.commitDialogOpen, func() {
				_, height := c.Size()
				ui.Column(c).Width(680).MaxWidth(760).Height(min(height-100, 600)).
					MinHeight(240).MinWidth(0).Gap(Spacing().L).Children(func() {
					modalTitle(c, "Commit Changes")
					s.commitSurface(c)
				})
			})
		}
	}
	if s.dialogOpen {
		ui.Modal(c, &s.dialogOpen, func() {
			modalTitle(c, s.dialogTitle)
			submit := false
			captionedField(c, s.dialogLabel, "", func() {
				submit = textField(c, &s.dialogValue).Label(s.dialogLabel).AutoFocus().Width(360).Submitted()
			})
			ui.Row(c).Gap(Spacing().M).Justify(ui.End).Children(func() {
				if regularButton(c, "Cancel").Clicked() {
					s.dialogOpen = false
				}
				action := s.dialogAction
				if action == "" {
					action = "Save"
				}
				if regularPrimaryButton(c, action).Clicked() {
					submit = true
				}
			})
			if submit && strings.TrimSpace(s.dialogValue) != "" {
				kind, target, value := s.dialogKind, s.dialogTarget, strings.TrimSpace(s.dialogValue)
				s.dialogOpen = false
				s.submitTextDialog(kind, target, value)
			}
		})
	}
	s.gitNoticeDialog(c)
	s.gitMoreDialogs(c)
	s.dirtySheet(c)
	s.lspRenameSheet(c)
	if s.confirmOpen {
		// Destructive confirms must not default-focus the destructive
		// button: AlertDialog focuses and Enter-clicks its LAST button, so
		// Cancel goes last (Enter dismisses safely; Escape still cancels by
		// label) and Confirm is index 0 (2026-10-06 F69).
		if ui.AlertDialog(c, &s.confirmOpen, s.confirmTitle, s.confirmMessage, confirmActionText(s.confirmKind), "Cancel") == 0 {
			kind, target := s.confirmKind, s.confirmTarget
			s.confirmOpen = false
			s.submitConfirm(kind, target)
		}
	}
}

// submitTextDialog applies the shared prompt's answer to the action that
// opened it.
func (s *Shell) submitTextDialog(kind, target, value string) {
	switch kind {
	case "git-add-worktree":
		s.addWorktree(value)
	case "git-create-branch":
		if s.git == nil || s.git.root == "" || s.git.root != target {
			s.pendingToast = "Repository changed. Reopen the dialog to continue."
			return
		}
		s.createBranch(value)
	case "git-stash-lines":
		s.submitStashLines(target, value)
	case "git-rename-branch", "git-create-branch-at", "git-rename-stash":
		s.submitRefDialog(kind, target, value)
	case "lsp-rename":
		s.lspRename(target, value)
	case "files-new-file", "files-new-dir", "files-rename":
		s.runFilesMutation(kind, target, value)
	}
}

// submitConfirm applies a confirmed action. A Git confirmation is refused when
// the repository changed since it was asked.
func (s *Shell) submitConfirm(kind, target string) {
	if strings.HasPrefix(kind, "git-") && (s.git == nil || s.confirmRepoRoot == "" || s.git.root != s.confirmRepoRoot) {
		s.pendingToast = "Repository changed. Reopen the dialog to continue."
		return
	}
	switch kind {
	case "files-delete":
		s.runFilesDelete(target)
	default:
		s.runGitConfirm(kind, target)
	}
}
