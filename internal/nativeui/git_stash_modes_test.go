package nativeui

/**
 * [INPUT]: git_workbench_actions.go 的 Actions 菜单（Stash Staged Changes / Stash Changes, Keep Staged）、git_operations.go 的 stashMode、真实临时仓库
 * [OUTPUT]: stash 变体的界面合同：只存已暂存的（未暂存的改动留在工作区）、全存但保留已暂存的；没有暂存时两项停用
 * [POS]: 2026-11-14 LazyGit 对齐批次；内核合同在 solco-gitkit gitworkbench/stash_modes_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStashStagedLeavesTheRestAndKeepStagedKeepsTheIndex(t *testing.T) {
	root := gitFixture(t, "stash-modes")
	readme, util := filepath.Join(root, "README.md"), filepath.Join(root, "src", "lib", "util.go")
	writeFile(t, readme, "# demo\n\nunstaged edit\n")
	writeFile(t, util, "package lib\n\n// staged edit\n")
	gitRun(t, root, "add", "src/lib/util.go")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { return sh.hasStagedChanges() })
	settle(tt, pump, 3)
	stashes := func() int { return len(strings.Fields(gitRun(t, root, "stash", "list"))) }

	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stash Staged Changes"); err != nil {
		t.Fatalf("menu %q: %v", tt.Menu(), err)
	}
	framesUntil(t, tt, pump, "the stash", func() bool { return !sh.git.opBusy && stashes() > 0 })
	if got := strings.TrimSpace(gitRun(t, root, "diff", "--cached", "--name-only")); got != "" {
		t.Errorf("the index is empty after Stash Staged: %q", got)
	}
	if data, _ := os.ReadFile(readme); !strings.Contains(string(data), "unstaged edit") {
		t.Errorf("the unstaged edit stays in the work tree: %q", data)
	}

	// With nothing staged both items are dead.
	settle(tt, pump, 3)
	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stash Changes, Keep Staged"); err == nil {
		t.Error("Keep Staged needs something staged")
	}
}

func TestStashKeepStagedFromTheMenu(t *testing.T) {
	root := gitFixture(t, "stash-keep")
	util := filepath.Join(root, "src", "lib", "util.go")
	writeFile(t, util, "package lib\n\n// staged edit\n")
	gitRun(t, root, "add", "src/lib/util.go")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { return sh.hasStagedChanges() })
	settle(tt, pump, 3)
	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stash Changes, Keep Staged"); err != nil {
		t.Fatalf("menu %q: %v", tt.Menu(), err)
	}
	framesUntil(t, tt, pump, "the stash", func() bool { return !sh.git.opBusy && strings.TrimSpace(gitRun(t, root, "stash", "list")) != "" })
	if got := strings.TrimSpace(gitRun(t, root, "diff", "--cached", "--name-only")); got != "src/lib/util.go" {
		t.Errorf("the staged change stays in the index: %q", got)
	}
}
