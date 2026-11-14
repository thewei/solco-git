package nativeui

/**
 * [INPUT]: sidebar.go 的提交右键菜单（Reset Current Branch to Here）、git_fork_more.go 的 confirmResetTo / runReset、真实临时仓库
 * [OUTPUT]: Reset 菜单的界面合同：Soft / Mixed / Hard 各自先确认、再把分支挪到所点的提交，并留下 Git 规定的那种工作区（Soft 保持暂存、Mixed 退回未暂存、Hard 连文件一起回去）
 * [POS]: 2026-11-14 LazyGit 对齐批次；内核合同在 solco-gitkit gitworkbench/fork_more_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"strings"
	"testing"
)

func TestResetMenuMovesTheBranchAndLeavesGitsWorkTree(t *testing.T) {
	for _, tc := range []struct {
		item   string
		status string // porcelain of the work tree afterwards, for the dropped commit's file
		onDisk bool
	}{
		{"Soft…", "A  f4.txt", true},
		{"Mixed…", "?? f4.txt", true},
		{"Hard…", "", false},
	} {
		t.Run(tc.item, func(t *testing.T) {
			root := historyRepo(t, "reset-"+strings.TrimSuffix(tc.item, "…"), 4)
			c3 := hashOf(t, root, "c3")
			sh, pump, tt := historyShell(t, root)
			if err := tt.RightClick("c3"); err != nil {
				t.Fatalf("no commit row: %v (%q)", err, tt.Texts())
			}
			if err := tt.ChooseMenuItem("Reset Current Branch to Here", tc.item); err != nil {
				t.Fatalf("menu %q: %v", tt.Menu(), err)
			}
			settle(tt, pump, 2)
			if head := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD")); head == c3 {
				t.Fatal("the branch moved before the confirmation")
			}
			if err := tt.Click("Reset"); err != nil {
				t.Fatalf("no confirmation: %v (%q)", err, tt.Texts())
			}
			pump.until(t, "the reset", func() bool { return !sh.git.opBusy && strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD")) == c3 })
			if got := strings.TrimSpace(gitRun(t, root, "status", "--porcelain")); got != tc.status {
				t.Errorf("status after %s = %q, want %q", tc.item, got, tc.status)
			}
			if _, err := os.Stat(root + "/f4.txt"); (err == nil) != tc.onDisk {
				t.Errorf("f4.txt on disk = %v, want %v", err == nil, tc.onDisk)
			}
		})
	}
}
