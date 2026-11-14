package nativeui

/**
 * [INPUT]: git_history_ui.go 的 Rewrite History 子菜单（Create Fixup Commit… / Squash Fixups From Here…）、git_operations.go 的确认分支、真实临时仓库
 * [OUTPUT]: fixup 工作流的界面合同：有暂存才能建 fixup（先确认，主题是 “fixup! 目标”）、之后 Squash Fixups From Here 把它折进目标；没有暂存时菜单项停用
 * [POS]: 2026-11-14 LazyGit 对齐批次；内核合同在 solco-gitkit gitworkbench/fixup_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"strings"
	"testing"
)

func TestCreateFixupThenSquashFixups(t *testing.T) {
	root := historyRepo(t, "fixup-flow", 3)
	sh, pump, tt := historyShell(t, root)

	writeFile(t, root+"/f2.txt", "line 2\nfixed\n")
	gitRun(t, root, "add", "f2.txt")
	sh.git.cache.Invalidate(root)
	sh.ensureGitSnapshot(true)
	pump.until(t, "the staged change", func() bool { return sh.hasStagedChanges() })
	settle(tt, pump, 2)
	if err := tt.RightClick("c2"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Rewrite History", "Create Fixup Commit…"); err != nil {
		t.Fatalf("menu: %v", err)
	}
	settle(tt, pump, 2)
	if got := subjects(t, root); got != "c3 c2 c1" {
		t.Fatalf("the fixup was committed before the confirmation: %s", got)
	}
	settle(tt, pump, 3)
	if err := tt.Click("Create Fixup"); err != nil {
		t.Fatalf("no confirmation: %v (%q)", err, tt.Texts())
	}
	pump.until(t, "the fixup commit", func() bool { return !sh.git.opBusy && strings.HasPrefix(subjects(t, root), "fixup! c2") })

	settle(tt, pump, 3)
	if err := tt.RightClick("c2"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Rewrite History", "Squash Fixups From Here…"); err != nil {
		t.Fatalf("menu: %v", err)
	}
	settle(tt, pump, 2)
	if err := tt.Click("Squash Fixups"); err != nil {
		t.Fatalf("no confirmation: %v (%q)", err, tt.Texts())
	}
	pump.until(t, "the squash", func() bool { return !sh.git.opBusy && subjects(t, root) == "c3 c2 c1" })
	if data, _ := os.ReadFile(root + "/f2.txt"); string(data) != "line 2\nfixed\n" {
		t.Errorf("the fixup's change is part of c2: %q", data)
	}
}

func TestFixupItemIsDeadWithoutStagedChanges(t *testing.T) {
	root := historyRepo(t, "fixup-dead", 3)
	_, _, tt := historyShell(t, root)
	if err := tt.RightClick("c2"); err != nil {
		t.Fatalf("no commit row: %v (%q)", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("Rewrite History", "Create Fixup Commit…"); err == nil {
		t.Fatal("a fixup needs staged changes: the item must be disabled")
	}
}
