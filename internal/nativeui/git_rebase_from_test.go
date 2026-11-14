package nativeui

/**
 * [INPUT]: sidebar.go 的提交右键菜单（Move Commits After This Onto）、git_operations.go 的 confirmRebaseFrom、真实临时仓库
 * [OUTPUT]: 三参 rebase 的界面合同：所点提交之后的提交挪到另一个本地分支上（该提交及之前不再属于当前分支，可由 Undo 找回），先确认再执行；没有其它本地分支时给出停用的说明项；冲突时中止、仓库原样
 * [POS]: 2026-11-14 LazyGit 对齐批次（“Edit commit + Rebase onto”里只做 Rebase onto 一半，Edit 由 Amend Into / Reword / Custom Patch 覆盖）；内核合同在 solco-gitkit gitworkbench/fork_more_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"slices"
	"strings"
	"testing"
)

func TestMoveCommitsAfterThisOntoAnotherBranch(t *testing.T) {
	root := historyRepo(t, "rebase-from", 4)
	c1 := hashOf(t, root, "c1")
	gitRun(t, root, "checkout", "-q", "-b", "other", c1)
	writeFile(t, root+"/x.txt", "x\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "x1")
	gitRun(t, root, "checkout", "-q", "work")
	sh, pump, tt := historyShell(t, root)

	if err := tt.RightClick("c2"); err != nil {
		t.Fatalf("no commit row: %v (%q)", err, tt.Texts())
	}
	if !slices.Contains(tt.Menu(), "Move Commits After This Onto") {
		t.Fatalf("commit menu = %q", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Move Commits After This Onto", "other…"); err != nil {
		t.Fatalf("submenu: %v", err)
	}
	settle(tt, pump, 2)
	if got := subjects(t, root); got != "c4 c3 c2 c1" {
		t.Fatalf("the rebase ran before the confirmation: %s", got)
	}
	if err := tt.Click("Rebase"); err != nil {
		t.Fatalf("no confirmation: %v (%q)", err, tt.Texts())
	}
	pump.until(t, "the rebase", func() bool { return !sh.git.opBusy && strings.HasPrefix(subjects(t, root), "c4 c3 x1") })
	// c3 and c4 now sit on other; c2 is left out of this branch.
	if got := subjects(t, root); got != "c4 c3 x1 c1" {
		t.Errorf("log = %q, want c4 c3 x1 c1", got)
	}
}
