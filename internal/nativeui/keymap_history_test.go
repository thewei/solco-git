package nativeui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

/**
 * [INPUT]: keymap_history.go、真实临时仓库（gitFixture：main 有三个提交，feature 指向第二个，工作区有未提交改动）、headless ui.Tester 的按键
 * [OUTPUT]: 2026-11-16 键盘 P3 的合同：Commits 列表的键开出与右键菜单同一批确认框 / 预览表，多选时只有批量键作用；引用树的键作用于光标所在的那一行
 * [POS]: 每个键只验证「开出了什么」，不验证 git 结果（那是菜单动作自己的测试）；Space 在提交上是唯一真正动 HEAD 的键，单独验证
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

func historyOpen(sh *Shell, title string) bool {
	return sh.gitMore.history.open && sh.gitMore.history.title == title
}

func TestCommitKeysOpenTheMenusSheets(t *testing.T) {
	root := gitFixture(t, "keys-commits")
	tt, pump, sh := commitListShell(t, root)
	head := sh.git.commits[0]

	cases := []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
		ok   func() bool
	}{
		{"T reverts", 0, ui.KeyT, func() bool {
			return sh.confirmOpen && sh.confirmKind == "git-revert-commit" && sh.confirmTarget == head.Hash
		}},
		{"C cherry-picks", 0, ui.KeyC, func() bool {
			return sh.confirmOpen && sh.confirmKind == "git-cherry-pick" && sh.confirmTarget == head.Hash
		}},
		{"N branches here", 0, ui.KeyN, func() bool { return sh.dialogOpen && sh.dialogKind == "git-create-branch-at" }},
		{"⇧T tags here", ui.Shift, ui.KeyT, func() bool { return sh.gitMore.tag.open && sh.gitMore.tag.hash == head.Hash }},
		{"D drops", 0, ui.KeyD, func() bool { return historyOpen(sh, "Drop Commits?") }},
		{"S squashes", 0, ui.KeyS, func() bool { return historyOpen(sh, "Squash Commits?") }},
		{"⇧F fixes up", ui.Shift, ui.KeyF, func() bool { return historyOpen(sh, "Fixup Commits?") }},
		{"⌥↓ moves down", ui.Alt, ui.KeyDown, func() bool { return historyOpen(sh, "Move Commits?") }},
		{"R rewords", 0, ui.KeyR, func() bool { return historyOpen(sh, "Reword Commit?") }},
	}
	for _, tc := range cases {
		// each key starts from a clean slate and the commit list holding the keyboard
		sh.confirmOpen, sh.dialogOpen, sh.gitMore.tag.open, sh.gitMore.history.open = false, false, false, false
		settle(tt, pump, 3) // the sheet leaves before the click
		if err := tt.Click(head.Subject); err != nil {
			t.Fatalf("%s: click: %v", tc.name, err)
		}
		settle(tt, pump, 3)
		tt.Key(tc.mods, tc.key)
		pump.until(t, tc.name, func() bool { settle(tt, pump, 1); return tc.ok() })
	}
}

func TestSpaceChecksOutTheCommitUnderTheCursor(t *testing.T) {
	root := gitFixture(t, "keys-checkout")
	tt, pump, _ := commitListShell(t, root)
	tt.Key(0, ui.KeyJ) // "Add main", where the feature branch points
	settle(tt, pump, 2)
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to check out", func() bool {
		settle(tt, pump, 1)
		return strings.TrimSpace(gitRun(t, root, "rev-parse", "--abbrev-ref", "HEAD")) == "feature"
	})
}

func TestOnlyBatchKeysActOnAChosenRun(t *testing.T) {
	root := gitFixture(t, "keys-batch")
	tt, pump, sh := commitListShell(t, root)
	tt.Key(ui.Shift, ui.KeyDown) // the newest two commits
	settle(tt, pump, 3)
	if n := len(sh.commitChoiceHashes(sh.git.commits)); n != 2 {
		t.Fatalf("⇧↓ chose %d commits, want 2", n)
	}
	// a one-commit key does nothing on a run
	tt.Key(0, ui.KeyT)
	tt.Key(0, ui.KeyR)
	tt.Key(0, ui.KeyN)
	settle(tt, pump, 3)
	if sh.confirmOpen || sh.dialogOpen || sh.gitMore.history.open {
		t.Fatalf("a single-commit key acted on a chosen run: confirm=%v dialog=%v history=%v", sh.confirmOpen, sh.dialogOpen, sh.gitMore.history.open)
	}
	tt.Key(0, ui.KeyD)
	pump.until(t, "D to drop the run", func() bool {
		settle(tt, pump, 1)
		return historyOpen(sh, "Drop Commits?") && len(sh.gitMore.history.lines) >= 2
	})
}

// refsRowShell opens the fixture and leaves the keyboard on the references row
// with the given label.
func refsRowShell(t *testing.T, label string) (*ui.Tester, *uiPump, *Shell) {
	t.Helper()
	w, pump, sh := focusShell(t)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 3)
	if err := tt.Click(label); err != nil {
		t.Fatalf("click the %q reference: %v", label, err)
	}
	settle(tt, pump, 3)
	if got, ok := sh.refCursor(); !ok || got.name != label {
		t.Fatalf("the cursor is on %+v, want the %q row", got, label)
	}
	return tt, pump, sh
}

func TestReferenceKeysActOnTheCursorRow(t *testing.T) {
	tt, pump, sh := refsRowShell(t, "feature")
	again := func() { // a sheet takes the keyboard when it opens; the row gets it back by a click
		sh.confirmOpen, sh.mergeDialogOpen, sh.dialogOpen = false, false, false
		settle(tt, pump, 3)
		if err := tt.Click("feature"); err != nil {
			t.Fatal(err)
		}
		settle(tt, pump, 3)
	}

	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to switch", func() bool {
		settle(tt, pump, 1)
		return sh.confirmOpen && sh.confirmKind == "git-switch-branch" && sh.confirmTarget == "feature"
	})
	again()
	tt.Key(0, ui.KeyD)
	pump.until(t, "D to delete", func() bool {
		settle(tt, pump, 1)
		return sh.confirmOpen && sh.confirmKind == "git-delete-branch" && sh.confirmTarget == "feature"
	})
	again()
	tt.Key(0, ui.KeyR)
	pump.until(t, "R to rebase", func() bool {
		settle(tt, pump, 1)
		return sh.confirmOpen && sh.confirmKind == "git-rebase" && sh.confirmTarget == "feature"
	})
	again()
	tt.Key(ui.Shift, ui.KeyM)
	pump.until(t, "⇧M to merge", func() bool { settle(tt, pump, 1); return sh.mergeDialogOpen && sh.mergeTarget == "feature" })

	again()
	tt.Key(ui.Shift, ui.KeyR)
	pump.until(t, "⇧R to rename", func() bool { settle(tt, pump, 1); return sh.dialogOpen && sh.dialogKind == "git-rename-branch" })
}

func TestReferenceKeysLeaveTheCurrentBranchAlone(t *testing.T) {
	tt, pump, sh := refsRowShell(t, "main")
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
	}{{0, ui.KeySpace}, {0, ui.KeyD}, {0, ui.KeyR}, {ui.Shift, ui.KeyM}} {
		tt.Key(k.mods, k.key)
	}
	settle(tt, pump, 3)
	if sh.confirmOpen || sh.mergeDialogOpen {
		t.Fatalf("a key acted on the checked-out branch: confirm=%v (%s) merge=%v", sh.confirmOpen, sh.confirmKind, sh.mergeDialogOpen)
	}
}
