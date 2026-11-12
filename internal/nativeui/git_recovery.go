package nativeui

/**
 * [INPUT]: shell.go（git 状态、applyOnUI）、git_operations.go（runGitOp）、gitworkbench.Runner.Operations / UndoOperation（Safety Journal）
 * [OUTPUT]: openRecoverySheet（Repository ▸ Recovery…：列出可找回的破坏性操作，新的在前）、recoverySheet、每行的 Recover 按钮（找回前由内核重新校验，文件又变了就拒绝并说明）
 * [POS]: 2026-11-12 决定第 (3) 条 Safety Journal 的界面一侧：丢弃改动与删除未跟踪文件的确认框不再写「无法撤销」，而是指向这里；列表只显示名字与时间，内容不进界面也不进日志
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

type recoverySheetState struct {
	open    bool
	loading bool
	err     string
	ops     []gitworkbench.Operation
	list    ui.ListState
	gen     int
}

func (s *Shell) openRecoverySheet() {
	if s.git == nil || s.git.root == "" {
		return
	}
	r := &s.gitMore.recovery
	r.gen++
	gen := r.gen
	*r = recoverySheetState{open: true, loading: true, gen: gen}
	root := s.git.root
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		ops, err := s.git.runner.Operations(ctx, root)
		s.applyOnUI(func() {
			r := &s.gitMore.recovery
			if r.gen != gen || s.git == nil || s.git.root != root {
				return
			}
			r.loading = false
			if err != nil {
				r.err = gitFailureText(err)
				return
			}
			r.ops = ops
		})
	}()
}

func (s *Shell) recoverySheet(c *ui.Context) {
	r := &s.gitMore.recovery
	if !r.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &r.open, func() {
		ui.Column(c).Width(640).MaxWidth(900).Height(480).MinHeight(240).MinWidth(0).Gap(Spacing().M).Children(func() {
			modalTitle(c, "Recovery")
			ui.Text(c, "Edits you discarded, files, branches and stashes you deleted here, newest first. Recover puts them back unless they changed since.").
				FontSize(Typography().BodySmall).TextColor(t.TextMuted)
			switch {
			case r.loading:
				busyNote(c, "Reading…")
			case r.err != "":
				ui.Text(c, r.err).FontSize(Typography().Body).TextColor(t.Warning)
			case len(r.ops) == 0:
				ui.Text(c, "Nothing to recover.").FontSize(Typography().Body).TextColor(t.TextMuted)
			default:
				ui.List(c, &r.list, len(r.ops), func(i int) {
					if i < 0 || i >= len(r.ops) {
						return
					}
					s.recoveryRow(c, r.ops[i])
				}).Grow(1).MinHeight(0)
			}
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if regularPrimaryButton(c, "Close").Clicked() {
					r.open = false
				}
			})
		})
	})
}

func (s *Shell) recoveryRow(c *ui.Context, op gitworkbench.Operation) {
	t := c.Theme()
	kind, summary := recoveryText(op)
	row := ui.Row(c).Gap(8).Padding(4, 8).Radius(treeRowRadius).AlignItems(ui.Center).MinWidth(0)
	tipped(c, row, summary+"\n"+strings.Join(op.Paths, "\n"))
	row.Children(func() {
		ui.Row(c).Size(70, 18).Shrink(0).Center().Radius(treeRowRadius).
			Background(t.TextMuted.Alpha(0.14)).Children(func() {
			ui.Text(c, kind).FontSize(Typography().Caption).FontWeight(WeightMedium).TextColor(t.TextMuted).SingleLine()
		})
		ui.Text(c, summary).FontSize(Typography().Body).Grow(1).MinWidth(0).SingleLine()
		ui.Text(c, gdRelativeTime(op.Created())).FontSize(Typography().BodySmall).TextColor(t.TextMuted).
			Width(72).TextAlign(ui.End).Shrink(0).SingleLine()
		label := "Recover"
		if op.UndoneAt != 0 {
			label = "Recovered"
		}
		if regularButton(c, label).Disabled(!op.CanUndo() || s.git.opBusy).Clicked() {
			s.gitMore.recovery.open = false
			s.recoverOperation(op)
		}
	})
}

// recoveryText names an entry by kind and by what it touched: the first file's
// name and how many more.
func recoveryText(op gitworkbench.Operation) (kind, summary string) {
	switch op.Kind {
	case gitworkbench.OpKindDiscard:
		kind = "discard"
	case gitworkbench.OpKindDeleteUntracked:
		kind = "delete"
	case gitworkbench.OpKindBranchDelete:
		kind = "branch"
	case gitworkbench.OpKindStashDrop, gitworkbench.OpKindStashClear:
		kind = "stash"
	case gitworkbench.OpKindApplyPatch:
		kind = "patch"
	case gitworkbench.OpKindResolveConflict:
		kind = "resolve"
	default:
		kind = op.Kind
	}
	switch len(op.Paths) {
	case 0:
		summary = op.Name
	case 1:
		summary = op.Paths[0]
	default:
		summary = op.Paths[0] + " and " + itoa(len(op.Paths)-1) + " more"
	}
	if op.Kind == gitworkbench.OpKindApplyPatch {
		summary = op.Name + " · " + pluralFiles(len(op.Paths))
	}
	if op.Status == gitworkbench.OpStatusFailed {
		summary += " (stopped part-way)"
	}
	return kind, summary
}

func (s *Shell) recoverOperation(op gitworkbench.Operation) {
	s.runGitOp("Recover", func(ctx context.Context, root string) (string, error) {
		if err := s.git.runner.UndoOperation(ctx, root, op.ID); err != nil {
			return "", err
		}
		switch op.Kind {
		case gitworkbench.OpKindBranchDelete:
			return "Recovered branch " + op.Name, nil
		case gitworkbench.OpKindStashDrop:
			return "Recovered the stash", nil
		case gitworkbench.OpKindStashClear:
			return "Recovered " + op.Name, nil
		case gitworkbench.OpKindApplyPatch:
			return "Took back " + op.Name, nil
		case gitworkbench.OpKindResolveConflict:
			return "Reopened the conflict in " + op.Paths[0], nil
		}
		return "Recovered " + pluralFiles(len(op.Paths)), nil
	})
}
