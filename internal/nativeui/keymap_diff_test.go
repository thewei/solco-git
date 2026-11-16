package nativeui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// twoHunkShell opens a repository whose rows.txt changed in two far-apart
// hunks (rows 3 and 55), the diff holding the keyboard on the first.
func twoHunkShell(t *testing.T, name string) (root string, tt *ui.Tester, pump *uiPump, sh *Shell) {
	t.Helper()
	root = gitFixture(t, name)
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, "row-"+itoa(i))
	}
	writeFile(t, filepath.Join(root, "rows.txt"), strings.Join(lines, "\n")+"\n")
	gitRun(t, root, "add", "rows.txt")
	gitRun(t, root, "commit", "-q", "-m", "rows")
	lines[2], lines[54] = "changed-3", "changed-55"
	writeFile(t, filepath.Join(root, "rows.txt"), strings.Join(lines, "\n")+"\n")
	w, pump := newTestWindow(t)
	sh = openAndLoad(t, w, pump, root)
	tt = newTester(w, 1280, 800, true)
	pump.until(t, "the diff", func() bool { settle(tt, pump, 1); return len(sh.git.gdRows) > 0 })
	if err := tt.Click("changed-3"); err != nil {
		t.Skipf("no diff line to click: %v", err)
	}
	settle(tt, pump, 3)
	return root, tt, pump, sh
}

// chooseHunkOf walks J until the chosen hunk is the first one of path.
func chooseHunkOf(t *testing.T, tt *ui.Tester, pump *uiPump, sh *Shell, path string) {
	t.Helper()
	for i := 0; i < 8; i++ {
		tt.Key(0, ui.KeyJ)
		settle(tt, pump, 3)
		if f, hunk, _, ok := sh.diffKeyTarget(); ok && f.cf.Path == path && hunk == 0 {
			return
		}
	}
	t.Fatalf("J never chose the first hunk of %s (at file %d hunk %d)", path, sh.git.gdSelFile, sh.git.gdSelHunk)
}

func cachedDiff(t *testing.T, root string) string {
	return gitRun(t, root, "diff", "--cached", "-U0", "rows.txt")
}

func TestSpaceStagesOnlyTheChosenHunk(t *testing.T) {
	root, tt, pump, sh := twoHunkShell(t, "keys-hunk")
	tt.Key(0, ui.KeySpace) // no hunk chosen yet: nothing to stage
	settle(tt, pump, 3)
	if out := gitRun(t, root, "diff", "--cached", "--stat"); out != "" {
		t.Fatalf("Space staged %q before any hunk was chosen", out)
	}
	chooseHunkOf(t, tt, pump, sh, "rows.txt")
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to stage the hunk", func() bool { settle(tt, pump, 1); return cachedDiff(t, root) != "" })
	cached := cachedDiff(t, root)
	if !strings.Contains(cached, "changed-3") || strings.Contains(cached, "changed-55") {
		t.Fatalf("Space must stage exactly the chosen hunk, staged:\n%s", cached)
	}
	rest := gitRun(t, root, "diff", "-U0", "rows.txt")
	if !strings.Contains(rest, "changed-") {
		t.Fatalf("the other hunk must stay unstaged, got:\n%s", rest)
	}
}

func TestAStagesTheWholeFileFromTheDiff(t *testing.T) {
	root, tt, pump, sh := twoHunkShell(t, "keys-hunk-file")
	chooseHunkOf(t, tt, pump, sh, "rows.txt")
	tt.Key(0, ui.KeyA)
	pump.until(t, "A to stage the file", func() bool {
		settle(tt, pump, 1)
		c := cachedDiff(t, root)
		return strings.Contains(c, "changed-3") && strings.Contains(c, "changed-55")
	})
}

func TestTheDiffSheetListsOnlyWhatTheDiffAnswers(t *testing.T) {
	own, anywhere := keysIn(keysDiff)
	if len(own) == 0 {
		t.Fatal("the diff has keys of its own")
	}
	for _, a := range anywhere {
		if a.key != ui.KeySlash {
			t.Errorf("the diff sheet lists %q, which the diff does not answer", a.label)
		}
	}
}

func TestSpaceStagesJustThePickedLines(t *testing.T) {
	root, tt, pump, sh := twoHunkShell(t, "keys-hunk-lines")
	chooseHunkOf(t, tt, pump, sh, "rows.txt")
	f, hunk, _, _ := sh.diffKeyTarget()
	added := -1
	for i, l := range f.hunks()[hunk].Lines {
		if l.Kind == gitworkbench.KindAdd {
			added = i
		}
	}
	if added < 0 {
		t.Fatal("the first hunk must hold an added line")
	}
	sh.gdPickLine(f, hunk, added, 0) // what a click on the line's number does
	tt.Key(0, ui.KeySpace)
	pump.until(t, "Space to stage the picked line", func() bool { settle(tt, pump, 1); return cachedDiff(t, root) != "" })
	if c := cachedDiff(t, root); !strings.Contains(c, "+changed-3") || strings.Contains(c, "-row-3") {
		t.Fatalf("only the picked line may be staged, got:\n%s", c)
	}
}
