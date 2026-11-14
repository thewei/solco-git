package nativeui

/**
 * [INPUT]: git_workbench_actions.go 的 Hide Untracked Files、sidebar.go 的 splitSections、gd_rows.go 的 gdHasContent
 * [OUTPUT]: 变更列表过滤的界面合同：Hide Untracked Files 同时从列表和 diff 卡片里去掉未跟踪文件（只剩已跟踪的改动），再点一次恢复；全是未跟踪时中心给出一键恢复
 * [POS]: 2026-11-14 LazyGit 对齐批次；文件名过滤早已有（Search files），这是“只看已跟踪”的那一半
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"testing"
)

func TestHideUntrackedFilesTakesThemOutOfListAndCards(t *testing.T) {
	root := gitFixture(t, "hide-untracked")
	writeFile(t, filepath.Join(root, "scratch.log"), "noise\n")
	writeFile(t, filepath.Join(root, "README.md"), "# demo\n\nedited\n")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { return len(sh.git.snapshot.Files) >= 2 })
	settle(tt, pump, 4)
	if !tt.HasText("scratch.log") {
		t.Fatalf("the untracked file is listed to begin with: %q", tt.Texts())
	}

	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Hide Untracked Files"); err != nil {
		t.Fatalf("menu %q: %v", tt.Menu(), err)
	}
	settle(tt, pump, 4)
	if tt.HasText("scratch.log") {
		t.Errorf("a hidden untracked file stays out of the list and the cards: %q", tt.Texts())
	}
	if !tt.HasText("README.md") {
		t.Errorf("tracked changes stay: %q", tt.Texts())
	}

	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Hide Untracked Files"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 4)
	if !tt.HasText("scratch.log") {
		t.Errorf("choosing it again brings them back: %q", tt.Texts())
	}
}
