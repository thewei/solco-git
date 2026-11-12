package nativeui

/**
 * [INPUT]: git_undo.go、window_view.go 的 ⌘Z / ⇧⌘Z、Repository 菜单、真实临时仓库
 * [OUTPUT]: 界面一侧 Undo / Redo 的合同：请求先出确认框并写明动作、确认后才动 HEAD、HEAD 在确认期间被别人动了就拒绝、没有可撤销时只给提示、⌘Z / ⇧⌘Z 走同一条路、文本输入框拿着焦点时 ⌘Z 不触发 Git 撤销
 * [POS]: 2026-11-12 决定第 (3) 条 Undo / Redo 的防线；底层合同在 solco-gitkit gitworkbench/reflog_undo_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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

func TestUndoKeysOpenTheSameConfirmation(t *testing.T) {
	root := gitFixture(t, "undo-keys")
	gitRun(t, root, "commit", "-q", "-am", "Keyed")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	tt.Key(ui.Super, ui.KeyZ)
	pump.until(t, "the confirmation from ⌘Z", func() bool { settle(tt, pump, 1); return sh.confirmOpen })
	if sh.confirmKind != confirmUndo {
		t.Fatalf("⌘Z opened %q", sh.confirmKind)
	}
	sh.confirmOpen = false
	// Take the commit back so a redo exists, then ⇧⌘Z must ask for it.
	sh.runGitConfirm(sh.confirmKind, sh.confirmTarget)
	pump.until(t, "the undo", func() bool { return !sh.git.opBusy && gitRun(t, root, "diff", "--cached", "--name-only") != "" })
	settle(tt, pump, 3)
	tt.Key(ui.Super|ui.Shift, ui.KeyZ)
	pump.until(t, "the confirmation from ⇧⌘Z", func() bool { settle(tt, pump, 1); return sh.confirmOpen })
	if sh.confirmKind != confirmRedo {
		t.Fatalf("⇧⌘Z opened %q", sh.confirmKind)
	}
}

// A text field that has the focus takes ⌘Z for its own undo: the commit
// search must not open a Git undo.
func TestUndoKeyBelongsToAFocusedTextField(t *testing.T) {
	root := gitFixture(t, "undo-field")
	gitRun(t, root, "commit", "-q", "-am", "Fielded")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "the commit list", func() bool { return len(sh.git.commits) > 0 })
	settle(tt, pump, 3)
	if err := tt.Click("Search commit list"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	tt.Type("Fiel")
	settle(tt, pump, 2)
	if sh.gitCommitFilter != "Fiel" {
		t.Fatalf("the search field did not take the text: %q", sh.gitCommitFilter)
	}
	tt.Key(ui.Super, ui.KeyZ)
	settle(tt, pump, 3)
	time.Sleep(600 * time.Millisecond) // the plan is read off the UI thread; give a wrongly fired one time to land
	pump.drain()
	if sh.confirmOpen {
		t.Fatalf("⌘Z in a focused text field opened %q", sh.confirmKind)
	}
}
