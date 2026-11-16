package nativeui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

/**
 * [INPUT]: sidebar.go 的 Commits 列表、changes_panel.go 的 Changes 列表、真实临时仓库、headless ui.Tester 的按键
 * [OUTPUT]: 2026-11-16 键盘 P1 的合同：方向键在 Commits 与 Changes 列表里走行，右侧的审阅跟着光标走
 * [POS]: 表征测试起步（当时 Commits 列表没有 Selected，方向键不动任何东西），随后成为行为的守卫
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

func reviewedHash(sh *Shell) string {
	if !sh.gdReviewingCommit() {
		return ""
	}
	return sh.git.commitHash
}

func TestArrowKeysWalkTheCommitList(t *testing.T) {
	root := gitFixture(t, "arrows-commits")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "commits", func() bool { return len(sh.git.commits) >= 3 })
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)

	if err := tt.Click("Add main"); err != nil {
		t.Fatalf("click a commit: %v", err)
	}
	settle(tt, pump, 3)
	first := reviewedHash(sh)
	if first == "" {
		t.Fatal("a click on a commit reviews it")
	}
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	if sh.commitKbd.cursor == "" {
		t.Fatal("↓ did not reach the commit list: the keyboard cursor never left the review")
	}
	pump.until(t, "the review to follow ↓", func() bool { return reviewedHash(sh) != first && reviewedHash(sh) != "" })
	tt.Key(0, ui.KeyUp)
	settle(tt, pump, 3)
	pump.until(t, "the review to follow ↑", func() bool { return reviewedHash(sh) == first })
}

func TestArrowKeysWalkTheChangesList(t *testing.T) {
	root := gitFixture(t, "arrows-changes")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	pump.until(t, "the changes", func() bool { settle(tt, pump, 1); return len(sh.git.gdFiles) > 0 })

	if err := tt.Click("README.md"); err != nil {
		t.Fatalf("click a changed file: %v", err)
	}
	settle(tt, pump, 3)
	first := sh.surface.diff.SelectedPath
	if first == "" {
		t.Fatal("a click on a changed file selects it")
	}
	// README.md is the last of the two unstaged rows: ↑ leaves it, ↓ comes back.
	tt.Key(0, ui.KeyUp)
	settle(tt, pump, 3)
	if got := sh.surface.diff.SelectedPath; got == first || got == "" {
		t.Fatalf("↑ left the selection on %q; it must move to the previous file", got)
	}
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	if got := sh.surface.diff.SelectedPath; got != first {
		t.Fatalf("↓ did not come back: selection is %q, want %q", got, first)
	}
}
