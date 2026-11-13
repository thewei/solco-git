package nativeui

import (
	"context"
	"slices"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

/**
 * [INPUT]: gd_stage_lines.go 的行选择（gdPick）、gitworkbench.StashLines、共用的文本对话框（dialogs.go）
 * [OUTPUT]: 行级 stash 的界面：hunk 行上的 “Stash N Lines…”（N 是该文件里选中的全部行）→ 问一句 stash 说明（预填默认值）→ 执行；已暂存的文件、提交审阅里不可用并说明原因
 * [POS]: 2026-11-12 决定第 (14) 条的界面，内核 gitkit/gitworkbench/stash_lines.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

// stashLinesState is what the Stash Lines prompt will stash: the lines as
// they were picked, kept apart from the pick so a pick made meanwhile does not
// change what the prompt was opened for.
type stashLinesState struct {
	root string
	src  gitworkbench.CourierSource
	n    int
}

// stashLinesWhy says why the picked lines of f cannot be stashed, "" when they
// can: the kernel cuts them against HEAD, so only an unstaged change of a
// tracked file qualifies.
func (s *Shell) stashLinesWhy(f *gdFile) string {
	if s.git == nil || s.gdReviewingCommit() {
		return "Lines are stashed from the working tree"
	}
	if unstage, ok := s.gdStageSide(f); !ok || unstage {
		return "Only unstaged changes of a tracked file can be stashed by line"
	}
	if f.cf.Staged {
		return "This file also has staged changes; unstage them first to stash lines"
	}
	return ""
}

// pickedInFile counts the lines picked in f, over every hunk.
func (s *Shell) pickedInFile(f *gdFile) int {
	p := &s.git.gdPick
	if !p.holds(s.git.snapshot, f) {
		return 0
	}
	return len(p.lines)
}

// openStashLines asks for the stash's message; the lines are fixed now.
func (s *Shell) openStashLines(f *gdFile) {
	n := s.pickedInFile(f)
	if n == 0 || s.stashLinesWhy(f) != "" || s.git.opBusy {
		return
	}
	picks := map[[2]int]bool{}
	for k := range s.git.gdPick.lines {
		picks[k] = true
	}
	s.gitMore.stashPick = stashLinesState{
		root: s.git.root, n: n,
		src: gitworkbench.CourierSource{Path: f.cf.Path, Hunks: slices.Clone(f.hunks()), Picks: picks},
	}
	s.openTextDialog("git-stash-lines", s.git.root, "Stash "+gdCountOf(n, "Line"), "Stash message", "Stashed "+gdCountOf(n, "line")+" of "+f.cf.Path)
	s.dialogAction = "Stash"
}

// submitStashLines completes the prompt: refused when the repository changed
// since it was opened.
func (s *Shell) submitStashLines(root, message string) {
	st := s.gitMore.stashPick
	s.gitMore.stashPick = stashLinesState{}
	if s.git == nil || s.git.root == "" || s.git.root != root || st.root != root || st.n == 0 {
		s.pendingToast = "Repository changed. Reopen the dialog to continue."
		return
	}
	s.git.gdPick = gdLinePick{}
	runner := s.git.runner
	s.runGitOp("Stash Lines", func(ctx context.Context, root string) (string, error) {
		if err := runner.StashLines(ctx, root, st.src, message); err != nil {
			return "", err
		}
		return "Stashed " + gdCountOf(st.n, "line") + " of " + st.src.Path, nil
	})
}
