package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// changesKeyShell opens the fixture (README.md modified, notes.txt untracked)
// and leaves the keyboard on the Unstaged list with README.md under the cursor.
func changesKeyShell(t *testing.T, name string) (root string, tt *ui.Tester, pump *uiPump, sh *Shell) {
	t.Helper()
	root = gitFixture(t, name)
	w, pump := newTestWindow(t)
	sh = openAndLoad(t, w, pump, root)
	tt = newTester(w, 1280, 800, true)
	pump.until(t, "the changes", func() bool { settle(tt, pump, 1); return len(sh.git.gdFiles) > 0 })
	if err := tt.Click("README.md"); err != nil {
		t.Fatalf("click README.md: %v", err)
	}
	settle(tt, pump, 3)
	return root, tt, pump, sh
}

func gitIdle(t *testing.T, tt *ui.Tester, pump *uiPump, sh *Shell) {
	t.Helper()
	pump.until(t, "the operation to finish", func() bool { settle(tt, pump, 1); return !sh.git.opBusy })
}

func staged(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(gitRun(t, root, "diff", "--cached", "--name-only"))
}

func TestSpaceStagesTheCursorFile(t *testing.T) {
	root, tt, pump, _ := changesKeyShell(t, "keys-space")
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to stage", func() bool { settle(tt, pump, 1); return staged(t, root) == "README.md" })
}

func TestSpaceUnstagesInTheStagedList(t *testing.T) {
	root := gitFixture(t, "keys-unstage")
	gitRun(t, root, "add", "notes.txt")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	pump.until(t, "the changes", func() bool { settle(tt, pump, 1); return len(sh.git.gdFiles) > 0 })
	if err := tt.Click("notes.txt"); err != nil {
		t.Fatalf("click the staged notes.txt: %v", err)
	}
	settle(tt, pump, 3)
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to unstage", func() bool { settle(tt, pump, 1); return staged(t, root) == "" })
}

func TestSpaceActsOnEveryChosenFile(t *testing.T) {
	root, tt, pump, _ := changesKeyShell(t, "keys-space-many")
	tt.Key(ui.Shift, ui.KeyUp) // README.md and the row above it
	settle(tt, pump, 3)
	tt.Key(0, ui.KeySpace)
	pump.until(t, "both files staged", func() bool {
		settle(tt, pump, 1)
		return strings.Contains(staged(t, root), "README.md") && strings.Contains(staged(t, root), "notes.txt")
	})
}

func TestAStagesTheWholeList(t *testing.T) {
	root, tt, pump, _ := changesKeyShell(t, "keys-stage-all")
	tt.Key(0, ui.KeyA)
	pump.until(t, "A to stage everything", func() bool {
		settle(tt, pump, 1)
		out := staged(t, root)
		return strings.Contains(out, "README.md") && strings.Contains(out, "notes.txt")
	})
}

func TestDAsksBeforeItDiscards(t *testing.T) {
	root, tt, pump, sh := changesKeyShell(t, "keys-discard")
	if err := tt.Click("notes.txt"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyD)
	settle(tt, pump, 3)
	if !sh.confirmOpen || sh.confirmKind != "git-delete-untracked" {
		t.Fatalf("D must open the delete confirmation first: open=%v kind=%q", sh.confirmOpen, sh.confirmKind)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatalf("D deleted the file before it was confirmed: %v", err)
	}
}

func TestIIgnoresAnUntrackedFileAndOnlyThat(t *testing.T) {
	root, tt, pump, sh := changesKeyShell(t, "keys-ignore")
	// a tracked file has nothing to ignore: the key does nothing
	tt.Key(0, ui.KeyI)
	settle(tt, pump, 3)
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err == nil {
		t.Fatal("I wrote a rule for a tracked file")
	}
	if err := tt.Click("notes.txt"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyI)
	pump.until(t, "I to write the rule", func() bool {
		settle(tt, pump, 1)
		b, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
		return strings.Contains(string(b), "notes.txt")
	})
	gitIdle(t, tt, pump, sh)
}

func TestSOpensTheStashSheet(t *testing.T) {
	_, tt, pump, sh := changesKeyShell(t, "keys-stash")
	tt.Key(0, ui.KeyS)
	settle(tt, pump, 3)
	if !sh.gitMore.stash.open || sh.gitMore.stash.kind != "save" {
		t.Fatalf("S must open the save-stash sheet, got %+v", sh.gitMore.stash)
	}
}

func TestCAndShiftAGiveTheKeyboardToTheCommitBar(t *testing.T) {
	_, tt, pump, sh := changesKeyShell(t, "keys-commit")
	tt.Key(0, ui.KeyC)
	settle(tt, pump, 3)
	tt.TypeKey(0, ui.KeyX, "x")
	settle(tt, pump, 2)
	if sh.gitQuickSubject != "x" {
		t.Fatalf("after C the next letters belong to the subject, got %q", sh.gitQuickSubject)
	}
	if sh.gitQuickAmend {
		t.Fatal("C must not tick Amend")
	}

	if err := tt.Click("README.md"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	tt.Key(ui.Shift, ui.KeyA)
	settle(tt, pump, 3)
	if !sh.gitQuickAmend {
		t.Fatal("⇧A must tick Amend")
	}
}

func TestChangesKeysStayQuietInAReviewedCommit(t *testing.T) {
	root, tt, pump, sh := changesKeyShell(t, "keys-quiet")
	sh.showGitCommits()
	pump.until(t, "the commit's files", func() bool {
		settle(tt, pump, 1)
		_, shown := tt.Find("util.go")
		return sh.gdReviewingCommit() && shown
	})
	if err := tt.Click("util.go"); err != nil { // a file of the reviewed commit
		t.Fatalf("click a file of the reviewed commit: %v", err)
	}
	settle(tt, pump, 3)
	for _, k := range []ui.Key{ui.KeySpace, ui.KeyA, ui.KeyD, ui.KeyS, ui.KeyI} {
		tt.Key(0, k)
	}
	settle(tt, pump, 3)
	if out := staged(t, root); out != "" || sh.confirmOpen || sh.gitMore.stash.open {
		t.Fatalf("a Changes key acted on a reviewed commit: staged=%q confirm=%v stash=%v", out, sh.confirmOpen, sh.gitMore.stash.open)
	}
}
