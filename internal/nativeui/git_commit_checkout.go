package nativeui

import (
	"context"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

// commitCheckoutPlan says what a double-click on a commit does.
type commitCheckoutPlan struct {
	kind   string // "here", "branch", "remote", "detach"
	target string
}

// planCommitCheckout reads the commit's own refs the way Fork does: the
// checked-out branch means nothing to do, else a local branch is switched to,
// else a remote branch becomes (or reuses) a local one, else HEAD detaches.
func planCommitCheckout(cm gitworkbench.CommitInfo) commitCheckoutPlan {
	var local, remote string
	for _, r := range cm.Refs {
		switch r.Kind {
		case gitworkbench.RefBranch:
			if r.Head {
				return commitCheckoutPlan{kind: "here", target: r.Name}
			}
			if local == "" {
				local = r.Name
			}
		case gitworkbench.RefRemote:
			if remote == "" {
				remote = r.Name
			}
		}
	}
	switch {
	case local != "":
		return commitCheckoutPlan{kind: "branch", target: local}
	case remote != "":
		return commitCheckoutPlan{kind: "remote", target: remote}
	}
	return commitCheckoutPlan{kind: "detach", target: cm.Hash}
}

// checkoutCommitRow checks out what a commit row stands for.
func (s *Shell) checkoutCommitRow(cm gitworkbench.CommitInfo) {
	if s.git == nil || s.git.root == "" || s.git.opBusy || !gitworkbenchFullHash(cm.Hash) {
		return
	}
	plan := planCommitCheckout(cm)
	switch plan.kind {
	case "here":
		s.pendingToast = "Already on " + plan.target
	case "branch":
		s.runGitOp("Check Out Branch", func(ctx context.Context, root string) (string, error) {
			if _, err := s.git.runner.SwitchBranch(ctx, root, plan.target); err != nil {
				return "", err
			}
			return "Switched to " + plan.target, nil
		})
	case "remote":
		s.runCheckoutRemote(plan.target)
	default:
		s.runGitOp("Check Out Commit", func(ctx context.Context, root string) (string, error) {
			if err := s.git.runner.CheckoutDetached(ctx, root, plan.target); err != nil {
				return "", err
			}
			return "HEAD is detached at " + plan.target[:8], nil
		})
	}
}
