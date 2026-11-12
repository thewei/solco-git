package nativeui

/**
 * [INPUT]: shell.go（git 状态、applyOnUI、pendingToast）、dialogs.go（openConfirm / runGitConfirm）、gitworkbench.Runner.PlanReplay / Replay
 * [OUTPUT]: requestReplay（预览 → 确认 → 执行的撤销 / 重做）、replayConfirmText（确认框的标题与说明）、runReplay
 * [POS]: 2026-11-12 决定第 (3) 条 Undo / Redo 的界面一侧：Repository 菜单的 Undo / Redo 与窗口内 ⌘Z / ⇧⌘Z（文本输入框拿着焦点时输入框先吃键）；确认框写明将被撤销的那一个动作，确认的是它的身份串，期间别的 Git 客户端动了 HEAD 就拒绝，不撤销用户没看过的东西
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

const (
	confirmUndo = "git-replay-undo"
	confirmRedo = "git-replay-redo"
)

// requestReplay reads what an undo (or redo) would do and asks first. The plan
// is read off the UI thread; nothing is changed until the user accepts.
func (s *Shell) requestReplay(redo bool) {
	if s.git == nil || s.git.root == "" {
		return
	}
	if s.git.opBusy {
		s.pendingToast = "Another Git operation is still running."
		return
	}
	root, verb := s.git.root, "undo"
	if redo {
		verb = "redo"
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		plan, ok, err := s.git.runner.PlanReplay(ctx, root, redo)
		s.applyOnUI(func() {
			if s.git == nil || s.git.root != root {
				return
			}
			switch {
			case err != nil:
				s.openGitNotice(verb, err, nil, false)
			case !ok:
				s.pendingToast = "Nothing to " + verb + "."
			default:
				title, message := replayConfirmText(plan)
				kind := confirmUndo
				if redo {
					kind = confirmRedo
				}
				s.openConfirm(kind, plan.Identity, title, message)
			}
		})
	}()
}

// replayConfirmText says which action the confirmation is about and what
// accepting it does to the branch and the working tree.
func replayConfirmText(plan gitworkbench.ReplayPlan) (title, message string) {
	short := func(hash string) string {
		if len(hash) > 7 {
			return hash[:7]
		}
		return hash
	}
	what := plan.Description
	if what == "" {
		what = string(plan.Kind)
	}
	switch {
	case !plan.Redo && plan.Kind == gitworkbench.ReplayCheckout:
		return "Undo Checkout?", what + "\n\nSwitches back to " + plan.From + "."
	case plan.Redo && plan.Kind == gitworkbench.ReplayCheckout:
		return "Redo Checkout?", what + "\n\nSwitches to " + plan.To + " again."
	case !plan.Redo && plan.Kind == gitworkbench.ReplayCommit:
		return "Undo Commit, Reset or Pull?", what + "\n\nMoves the branch back to " + short(plan.From) + ". The changes it brought in stay staged."
	case plan.Redo && plan.Kind == gitworkbench.ReplayCommit:
		return "Redo Commit, Reset or Pull?", what + "\n\nResets the branch to " + short(plan.To) + ". Edits to tracked files are set aside and put back."
	case !plan.Redo:
		return "Undo Rebase?", "Resets the branch to " + short(plan.From) + ", where it was before the rebase. Edits to tracked files are set aside and put back."
	default:
		return "Redo Rebase?", "Resets the branch to " + short(plan.To) + ", the result of the rebase. Edits to tracked files are set aside and put back."
	}
}

// runReplay is the accepted confirmation: it replays exactly the action the
// dialog named, or refuses when HEAD has moved since.
func (s *Shell) runReplay(redo bool, identity string) {
	name := "Undo"
	if redo {
		name = "Redo"
	}
	s.runGitOp(name, func(ctx context.Context, root string) (string, error) {
		if err := s.git.runner.Replay(ctx, root, redo, identity); err != nil {
			return "", err
		}
		return name + " done", nil
	})
}
