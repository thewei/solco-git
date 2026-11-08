package nativeui

import (
	"context"
	"strings"
)

// checkoutRemoteBranch checks out a remote-tracking branch (a tracking local
// branch is created when none of that name exists). With local changes it asks
// first, as switching a branch does.
func (s *Shell) checkoutRemoteBranch(ref string) {
	if s.git == nil || s.git.root == "" || s.git.opBusy {
		return
	}
	if s.dirtyWorktree() {
		s.openConfirm("git-checkout-remote", ref, "Check Out With Local Changes?",
			"Your uncommitted changes come along to "+ref+". Git refuses if they conflict.")
		return
	}
	s.runCheckoutRemote(ref)
}

func (s *Shell) runCheckoutRemote(ref string) {
	s.runGitOp("Check Out", func(ctx context.Context, root string) (string, error) {
		res, err := s.git.runner.CheckoutRemoteBranch(ctx, root, ref)
		if err != nil {
			return "", err
		}
		if res.Created {
			return "Checked out " + res.Branch + " (tracking " + ref + ")", nil
		}
		return "Switched to " + res.Branch, nil
	})
}

// openRenameBranch asks for a local branch's new name.
func (s *Shell) openRenameBranch(name string) {
	if s.git == nil || s.git.root == "" || s.git.opBusy {
		return
	}
	s.openTextDialog("git-rename-branch", s.git.root+"\x00"+name, "Rename Branch", "New name for "+name, name)
	s.dialogAction = "Rename"
}

// openCreateAt asks for a branch name to create at one exact commit.
func (s *Shell) openCreateAt(kind, hash string) {
	if s.git == nil || s.git.root == "" || s.git.opBusy || !gitworkbenchFullHash(hash) {
		return
	}
	s.openTextDialog(kind, s.git.root+"\x00"+hash, "New Branch Here ("+hash[:8]+")", "Branch name", "")
	s.dialogAction = "Create"
}

// submitRefDialog completes the name dialogs above; the target carries the
// repository root it was opened for, so a submit after the context moved on is
// refused rather than applied elsewhere.
func (s *Shell) submitRefDialog(kind, target, value string) {
	root, subject, _ := strings.Cut(target, "\x00")
	if s.git == nil || s.git.root == "" || s.git.root != root {
		s.pendingToast = "Repository changed. Reopen the dialog to continue."
		return
	}
	switch kind {
	case "git-rename-branch":
		s.runGitOp("Rename Branch", func(ctx context.Context, root string) (string, error) {
			if err := s.git.runner.RenameBranch(ctx, root, subject, value); err != nil {
				return "", err
			}
			return "Renamed " + subject + " to " + value, nil
		})
	case "git-rename-stash":
		s.renameStash(subject, value)
	case "git-create-branch-at":
		s.runGitOp("New Branch", func(ctx context.Context, root string) (string, error) {
			if _, err := s.git.runner.CreateBranchAt(ctx, root, value, subject); err != nil {
				return "", err
			}
			return "Created and switched to " + value, nil
		})
	}
}

// pushTag publishes one tag to the branch's remote (plain Push never sends tags).
func (s *Shell) pushTag(name string) {
	s.runGitNetworkOp("Push Tag", func(ctx context.Context, root string) (string, error) {
		remote := s.git.runner.DefaultRemote(ctx, root)
		if up := s.git.upstream; up.OK && up.Remote != "" {
			remote = up.Remote
		}
		if err := s.git.runner.PushTag(ctx, root, remote, name); err != nil {
			return "", err
		}
		return "Pushed tag " + name + " to " + remote, nil
	})
}

// applyStash re-applies one stash and keeps it (Pop applies and drops).
func (s *Shell) applyStash(ref string) {
	s.runGitOp("Stash Apply", func(ctx context.Context, root string) (string, error) {
		if err := s.git.runner.StashApply(ctx, root, ref); err != nil {
			return "", err
		}
		return "Stash applied (kept)", nil
	})
}

// stashTitle is a stash row's label: its message without Git's "On <branch>:"
// framing and the stash@{n} ref (the ref stays in the tooltip), as Fork shows it.
func stashTitle(subject string) string {
	text := strings.TrimSpace(subject)
	for _, prefix := range []string{"On ", "WIP on "} {
		if rest, ok := strings.CutPrefix(text, prefix); ok {
			if _, after, found := strings.Cut(rest, ": "); found {
				return strings.TrimSpace(after)
			}
		}
	}
	return text
}
