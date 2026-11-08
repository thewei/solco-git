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
