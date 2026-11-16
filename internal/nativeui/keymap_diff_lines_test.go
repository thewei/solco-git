package nativeui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// blockShell opens a repository whose rows.txt changed in one hunk of three
// deleted and three added lines (rows 10–12), with the first hunk of rows.txt
// chosen and the keyboard in the diff.
func blockShell(t *testing.T, name string) (root string, tt *ui.Tester, pump *uiPump, sh *Shell) {
	t.Helper()
	root = gitFixture(t, name)
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, "row-"+itoa(i))
	}
	writeFile(t, filepath.Join(root, "rows.txt"), strings.Join(lines, "\n")+"\n")
	gitRun(t, root, "add", "rows.txt")
	gitRun(t, root, "commit", "-q", "-m", "rows")
	for i := 9; i <= 11; i++ {
		lines[i] = "x" + itoa(i+1)
	}
	writeFile(t, filepath.Join(root, "rows.txt"), strings.Join(lines, "\n")+"\n")
	w, pump := newTestWindow(t)
	sh = openAndLoad(t, w, pump, root)
	tt = newTester(w, 1280, 800, true)
	pump.until(t, "the diff", func() bool { settle(tt, pump, 1); return len(sh.git.gdRows) > 0 })
	if err := tt.Click("x10"); err != nil {
		t.Skipf("no diff line to click: %v", err)
	}
	settle(tt, pump, 3)
	chooseHunkOf(t, tt, pump, sh, "rows.txt")
	return root, tt, pump, sh
}

func pickedCount(sh *Shell) int { return len(sh.git.gdPick.lines) }

func TestDownPicksTheNextChangedLineAndSpaceStagesJustIt(t *testing.T) {
	root, tt, pump, sh := blockShell(t, "keys-line")
	tt.Key(0, ui.KeyDown) // the first changed line: -row-10
	settle(tt, pump, 3)
	if pickedCount(sh) != 1 {
		t.Fatalf("↓ must pick one line, picked %d", pickedCount(sh))
	}
	tt.Key(0, ui.KeyDown) // -row-11
	settle(tt, pump, 3)
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to stage the line", func() bool { settle(tt, pump, 1); return cachedDiff(t, root) != "" })
	c := cachedDiff(t, root)
	if !strings.Contains(c, "-row-11") || strings.Contains(c, "-row-10") || strings.Contains(c, "-row-12") || strings.Contains(c, "+x") {
		t.Fatalf("only the second changed line may be staged, got:\n%s", c)
	}
}

func TestVDrawsARangeAndSpaceStagesIt(t *testing.T) {
	root, tt, pump, sh := blockShell(t, "keys-range")
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyV)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	if pickedCount(sh) != 3 {
		t.Fatalf("v then ↓ ↓ must pick three lines, picked %d", pickedCount(sh))
	}
	tt.Key(0, ui.KeyUp) // the range shrinks back
	settle(tt, pump, 3)
	if pickedCount(sh) != 2 {
		t.Fatalf("↑ in a range must give a line back, picked %d", pickedCount(sh))
	}
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to stage the range", func() bool { settle(tt, pump, 1); return cachedDiff(t, root) != "" })
	c := cachedDiff(t, root)
	for _, want := range []string{"-row-10", "-row-11", "-row-12"} {
		if !strings.Contains(c, want) {
			t.Fatalf("the range must stage %s, got:\n%s", want, c)
		}
	}
	if strings.Contains(c, "+x") {
		t.Fatalf("the added lines were not in the range:\n%s", c)
	}
}

func TestEscEndsARangeBeforeItLeavesTheDiff(t *testing.T) {
	_, tt, pump, sh := blockShell(t, "keys-range-esc")
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyV)
	settle(tt, pump, 3)
	tt.Key(0, ui.KeyDown)
	settle(tt, pump, 3)
	sh.keyBack = &ui.ListState{} // a list is waiting to take the keyboard back
	tt.Key(0, ui.KeyEscape)
	settle(tt, pump, 3)
	if sh.diffLines.ranging || pickedCount(sh) != 1 {
		t.Fatalf("Esc must collapse the range (ranging %v, picked %d)", sh.diffLines.ranging, pickedCount(sh))
	}
	if sh.keyBackNow || sh.keyBack == nil {
		t.Fatal("the first Esc ended the range; it must not also leave the diff")
	}
	tt.Key(0, ui.KeyEscape) // nothing left to end: this one leaves the diff
	settle(tt, pump, 3)
	if !sh.keyBackNow {
		t.Fatal("the second Esc must ask for the keyboard back to the list")
	}
}

func TestTheLineCursorGoesOnIntoTheNextHunk(t *testing.T) {
	_, tt, pump, sh := twoHunkShell(t, "keys-cross")
	chooseHunkOf(t, tt, pump, sh, "rows.txt")
	for i := 0; i < 3; i++ { // -row-3, +changed-3, then the next hunk's first line
		tt.Key(0, ui.KeyDown)
		settle(tt, pump, 3)
	}
	if sh.git.gdSelHunk != 1 {
		t.Fatalf("↓ past the last changed line must go on into the next hunk, at hunk %d", sh.git.gdSelHunk)
	}
	tt.Key(0, ui.KeyUp)
	settle(tt, pump, 3)
	if sh.git.gdSelHunk != 0 {
		t.Fatalf("↑ from a hunk's first line must go back into the previous hunk, at hunk %d", sh.git.gdSelHunk)
	}
}

func TestTheLineKeysAreInTheDiffSheet(t *testing.T) {
	own, _ := keysIn(keysDiff)
	var seen = map[ui.Key]bool{}
	for _, a := range own {
		seen[a.key] = true
	}
	for _, k := range []ui.Key{ui.KeyDown, ui.KeyUp, ui.KeyV} {
		if !seen[k] {
			t.Errorf("the diff sheet lacks key %v", k)
		}
	}
}
