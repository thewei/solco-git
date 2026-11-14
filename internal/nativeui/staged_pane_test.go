package nativeui

import (
	"fmt"
	"path/filepath"
	"testing"
)

// stagedPaneFixture is a repository with unstaged files (the fixture's
// modified README.md and untracked notes.txt, plus extra) and staged ones.
func stagedPaneFixture(t *testing.T, unstagedExtra, staged int) string {
	t.Helper()
	root := gitFixture(t, "panes")
	for i := 0; i < unstagedExtra; i++ {
		writeFile(t, filepath.Join(root, fmt.Sprintf("u%d.txt", i)), "a\n")
	}
	for i := 0; i < staged; i++ {
		writeFile(t, filepath.Join(root, "staged", fmt.Sprintf("s%d.txt", i)), "a\n")
	}
	if staged > 0 {
		gitRun(t, root, "add", "staged")
	}
	return root
}

// The divider is live: the Staged list stands at the dragged height, so a
// taller Staged pane starts higher. Height on a list that Grow(1) had given a
// 0% basis did nothing — both lists shared the room equally and the divider
// moved nothing (real-machine run, 2026-11-08).
func TestStagedPaneFollowsTheDivider(t *testing.T) {
	root := stagedPaneFixture(t, 3, 10)
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)

	headerAt := func(dragged float32) float32 {
		sh.stagedPaneHeight = dragged
		settle(tt, pump, 2)
		r, ok := tt.Find("Staged Files")
		if !ok {
			t.Fatalf("no Staged Files header at %v: %q", dragged, tt.Texts())
		}
		return r.Y
	}
	short, tall := headerAt(60), headerAt(4000)
	if tall > short-30 {
		t.Errorf("a taller Staged pane must start higher: header at y %.0f for 60 points, y %.0f for the most", short, tall)
	}
	// A drag past the cap is held there, not stored: dragging back moves at once.
	if sh.stagedPaneHeight >= 4000 {
		t.Errorf("the dragged height %v is kept past the cap", sh.stagedPaneHeight)
	}
}

// A one-file Staged list stands no taller than its row, so the Unstaged rows
// above it all show: the lists used to split the room equally and cut the
// Unstaged list to half of it.
func TestStagedPaneStaysAsTallAsItsRows(t *testing.T) {
	root := stagedPaneFixture(t, 12, 1)
	w, pump := newTestWindow(t)
	openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)

	staged, ok := tt.Find("Staged Files")
	if !ok {
		t.Fatalf("no Staged Files header: %q", tt.Texts())
	}
	// Rows sort as text: u9.txt is the last of the fourteen.
	for _, name := range []string{"notes.txt", "u0.txt", "u8.txt", "u9.txt"} {
		row, ok := tt.Find(name)
		if !ok {
			t.Fatalf("no %s row: %q", name, tt.Texts())
		}
		if row.Y+row.H > staged.Y+1 {
			t.Errorf("%s (y %.0f–%.0f) sits behind the Staged Files header at y %.0f", name, row.Y, row.Y+row.H, staged.Y)
		}
	}
}

// The divider follows the pointer from the first pixel and in both directions,
// also when the Staged list's rows are fewer than its stored height (the case
// where it used to move nothing: it started from a height the list never had).
func TestPaneDividerFollowsThePointerBothWays(t *testing.T) {
	root := stagedPaneFixture(t, 8, 3)
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)

	for _, dy := range []float32{-90, -60, 150, 40} {
		before, ok := tt.Find("Resize panes")
		if !ok {
			t.Fatalf("no divider: %q", tt.Texts())
		}
		y := before.Y + before.H/2
		tt.Press(before.X+40, y)
		tt.Move(before.X+40, y+dy/2)
		tt.Move(before.X+40, y+dy)
		tt.Release(before.X+40, y+dy)
		settle(tt, pump, 4)
		after, _ := tt.Find("Resize panes")
		if got := after.Y - before.Y; got < dy-2 || got > dy+2 {
			t.Errorf("a drag of %.0f moved the divider by %.0f", dy, got)
		}
	}
	if !sh.stagedPaneSized {
		t.Error("a dragged divider is remembered as the user's size")
	}
}

// A double click on the divider gives the panes their default split back.
func TestPaneDividerDoubleClickResets(t *testing.T) {
	root := stagedPaneFixture(t, 8, 3)
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)

	home, _ := tt.Find("Resize panes")
	y := home.Y + home.H/2
	tt.Press(home.X+40, y)
	tt.Move(home.X+40, y-80)
	tt.Release(home.X+40, y-80)
	settle(tt, pump, 4)
	moved, _ := tt.Find("Resize panes")
	if moved.Y > home.Y-70 || !sh.stagedPaneSized {
		t.Fatalf("the drag did not move the divider: %.0f → %.0f", home.Y, moved.Y)
	}

	tt.ClickAt(moved.X+40, moved.Y+moved.H/2)
	tt.ClickAt(moved.X+40, moved.Y+moved.H/2)
	settle(tt, pump, 4)
	back, _ := tt.Find("Resize panes")
	if back.Y != home.Y || sh.stagedPaneSized {
		t.Errorf("a double click should restore the divider to y %.0f, it is at y %.0f (sized %v)", home.Y, back.Y, sh.stagedPaneSized)
	}
}
