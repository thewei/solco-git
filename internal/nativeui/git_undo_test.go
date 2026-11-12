package nativeui

/**
 * [INPUT]: git_undo.go、window_view.go 的 ⌘Z / ⇧⌘Z、Repository 菜单、真实临时仓库
 * [OUTPUT]: 界面一侧 Undo / Redo 的合同：请求先出确认框并写明动作、确认后才动 HEAD、HEAD 在确认期间被别人动了就拒绝、没有可撤销时只给提示、Repository 菜单两项带 ⌥⌘Z 系加速键（不占 Edit 菜单的 ⌘Z）且各开对应确认
 * [POS]: 2026-11-12 决定第 (3) 条 Undo / Redo 的防线；底层合同在 solco-gitkit gitworkbench/reflog_undo_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"

	mygo "github.com/egoist/mygo"
)

func gitHead(t *testing.T, root string) string {
	t.Helper()
	return gitRun(t, root, "rev-parse", "HEAD")
}

func TestUndoAsksThenMovesHead(t *testing.T) {
	root := gitFixture(t, "undo")
	before := gitHead(t, root)
	gitRun(t, root, "commit", "-q", "-am", "One more")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	sh.requestReplay(false)
	pump.until(t, "the undo confirmation", func() bool { return sh.confirmOpen })
	if sh.confirmKind != confirmUndo || !strings.Contains(sh.confirmMessage, "commit: One more") {
		t.Fatalf("confirm = %q %q", sh.confirmKind, sh.confirmMessage)
	}
	if gitHead(t, root) == before {
		t.Fatal("HEAD moved before the user accepted")
	}
	sh.confirmOpen = false
	sh.runGitConfirm(sh.confirmKind, sh.confirmTarget)
	pump.until(t, "the undo", func() bool { return !sh.git.opBusy && gitHead(t, root) == before })
	if !strings.Contains(gitRun(t, root, "diff", "--cached", "--name-only"), "README.md") {
		t.Error("the undone commit's changes are not staged")
	}

	sh.requestReplay(true)
	pump.until(t, "the redo confirmation", func() bool { return sh.confirmOpen && sh.confirmKind == confirmRedo })
}

func TestUndoConfirmationRefusesAfterHeadMoved(t *testing.T) {
	root := gitFixture(t, "undo-moved")
	gitRun(t, root, "commit", "-q", "-am", "First")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	sh.requestReplay(false)
	pump.until(t, "the undo confirmation", func() bool { return sh.confirmOpen })
	target := sh.confirmTarget
	sh.confirmOpen = false
	writeFile(t, root+"/late.txt", "x\n")
	gitRun(t, root, "add", "late.txt")
	gitRun(t, root, "commit", "-q", "-m", "Someone else")
	moved := gitHead(t, root)

	sh.runGitConfirm(confirmUndo, target)
	pump.until(t, "the refused undo", func() bool { return !sh.git.opBusy })
	settle(tt, pump, 2)
	if gitHead(t, root) != moved {
		t.Fatal("an undo the user never saw ran")
	}
}

func TestUndoWithNothingToUndoOnlySaysSo(t *testing.T) {
	root := gitFixture(t, "undo-none")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.requestReplay(true) // nothing was undone, so nothing can be redone
	pump.until(t, "the notice", func() bool { return sh.pendingToast == "Nothing to redo." })
	if sh.confirmOpen {
		t.Fatal("a redo with nothing undone opened a confirmation")
	}
}

// The Repository menu carries both, with accelerators that are not the Edit
// menu's ⌘Z / ⇧⌘Z (those reach a focused text field as its own undo, so a view
// never sees them), and choosing them asks the same confirmations.
func TestRepositoryMenuUndoAndRedo(t *testing.T) {
	root := gitFixture(t, "undo-menu")
	gitRun(t, root, "commit", "-q", "-am", "Menued")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	find := func(label string) *mygo.MenuItem {
		for _, menu := range w.MenuBar().Items() {
			for _, item := range menu.Submenu {
				if item.Label == label {
					return item
				}
			}
		}
		t.Fatalf("no menu item %q", label)
		return nil
	}
	undo, redo := find("Undo Git Action…"), find("Redo Git Action…")
	if undo.Accelerator != accelUndoGit || redo.Accelerator != accelRedoGit {
		t.Fatalf("accelerators = %q, %q", undo.Accelerator, redo.Accelerator)
	}
	for _, edit := range []string{"CmdOrCtrl+Z", "Shift+CmdOrCtrl+Z"} {
		if undo.Accelerator == edit || redo.Accelerator == edit {
			t.Fatalf("a Git undo item took the Edit menu's %s", edit)
		}
	}
	if undo.Disabled || redo.Disabled {
		t.Fatal("the items are disabled with a repository open")
	}

	undo.Click(undo, nil)
	pump.until(t, "the undo confirmation", func() bool { return sh.confirmOpen })
	if sh.confirmKind != confirmUndo {
		t.Fatalf("Undo opened %q", sh.confirmKind)
	}
	sh.confirmOpen = false
	sh.runGitConfirm(sh.confirmKind, sh.confirmTarget)
	pump.until(t, "the undo", func() bool { return !sh.git.opBusy && gitRun(t, root, "diff", "--cached", "--name-only") != "" })
	settle(tt, pump, 3)
	redo.Click(redo, nil)
	pump.until(t, "the redo confirmation", func() bool { return sh.confirmOpen && sh.confirmKind == confirmRedo })
}
