package nativeui

/**
 * [INPUT]: git_workbench_actions.go 的 Actions 菜单（Discard All Unstaged Changes… / Delete All Untracked Files…）、git_operations.go 的 discardChanges、真实临时仓库
 * [OUTPUT]: 批量丢弃的界面合同：只丢未暂存的改动（已暂存的、未跟踪的原样）、只删未跟踪的文件；各自先确认；没有可丢的时菜单项停用
 * [POS]: 2026-11-14 LazyGit 对齐批次；确认与恢复点沿用单文件丢弃的路径（Safety Journal）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscardAllUnstagedAndDeleteAllUntracked(t *testing.T) {
	root := gitFixture(t, "discard-all")
	readme, util, scratch := filepath.Join(root, "README.md"), filepath.Join(root, "src", "lib", "util.go"), filepath.Join(root, "scratch.log")
	// what a discard restores: the index version, not the fixture's work tree
	orig := gitRun(t, root, "show", ":README.md")
	writeFile(t, readme, "# demo\n\nunstaged edit\n")
	writeFile(t, util, "package lib\n\n// staged edit\n")
	gitRun(t, root, "add", "src/lib/util.go")
	writeFile(t, scratch, "noise\n")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { return len(sh.git.snapshot.Files) >= 3 })
	settle(tt, pump, 3)

	if err := tt.Click("Actions"); err != nil {
		t.Fatalf("no Actions button: %v", err)
	}
	if err := tt.ChooseMenuItem("Discard All Unstaged Changes…"); err != nil {
		t.Fatalf("menu %q: %v", tt.Menu(), err)
	}
	settle(tt, pump, 3)
	if data, _ := os.ReadFile(readme); strings.TrimSpace(string(data)) == strings.TrimSpace(orig) {
		t.Fatal("the edit was discarded before the confirmation")
	}
	if err := tt.Click("Discard"); err != nil {
		t.Fatalf("no confirmation: %v (%q)", err, tt.Texts())
	}
	framesUntil(t, tt, pump, "the discard", func() bool {
		data, _ := os.ReadFile(readme)
		return !sh.git.opBusy && strings.TrimSpace(string(data)) == strings.TrimSpace(orig)
	})
	if data, _ := os.ReadFile(util); string(data) != "package lib\n\n// staged edit\n" {
		t.Errorf("a staged edit is not an unstaged one: %q", data)
	}
	if _, err := os.Stat(scratch); err != nil {
		t.Error("an untracked file is not touched by Discard All Unstaged")
	}

	settle(tt, pump, 3)
	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete All Untracked Files…"); err != nil {
		t.Fatalf("menu %q: %v", tt.Menu(), err)
	}
	settle(tt, pump, 3)
	if err := tt.Click("Delete"); err != nil {
		t.Fatalf("no confirmation: %v (%q)", err, tt.Texts())
	}
	framesUntil(t, tt, pump, "the delete", func() bool {
		_, err := os.Stat(scratch)
		return !sh.git.opBusy && err != nil
	})
	if data, _ := os.ReadFile(util); string(data) != "package lib\n\n// staged edit\n" {
		t.Errorf("the staged edit survived the delete: %q", data)
	}
}
