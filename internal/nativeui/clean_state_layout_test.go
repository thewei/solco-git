package nativeui

import (
	"strings"
	"testing"
)

// commitAll leaves the fixture's worktree clean.
func commitAll(t *testing.T, root string) {
	t.Helper()
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "Everything")
}

// A clean worktree is the Changes view with nothing in it, not a page of its
// own (2026-11-16): the file list column stays where it is with both of its
// sections, and the center says so in the banner every other empty diff uses.
func TestCleanChangesKeepTheLayoutOfAChangedTree(t *testing.T) {
	dirtyRoot := gitFixture(t, "dirty")
	w, pump := newTestWindow(t)
	openAndLoad(t, w, pump, dirtyRoot)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)
	dirtyUnstaged, ok := tt.Find("Unstaged Files")
	if !ok {
		t.Fatalf("no Unstaged Files header on a changed tree: %q", tt.Texts())
	}
	dirtyStaged, _ := tt.Find("Staged Files")

	cleanRoot := gitFixture(t, "clean")
	commitAll(t, cleanRoot)
	w2, pump2 := newTestWindow(t)
	openAndLoad(t, w2, pump2, cleanRoot)
	tc := newTester(w2, 1280, 800, true)
	settle(tc, pump2, 4)

	un, ok := tc.Find("Unstaged Files")
	if !ok {
		t.Fatalf("a clean tree lost its file list column: %q", tc.Texts())
	}
	st, ok := tc.Find("Staged Files")
	if !ok {
		t.Fatalf("a clean tree lost its Staged section: %q", tc.Texts())
	}
	if un.X != dirtyUnstaged.X || st.X != dirtyStaged.X {
		t.Errorf("the file list moved: Unstaged x %.0f→%.0f, Staged x %.0f→%.0f", dirtyUnstaged.X, un.X, dirtyStaged.X, st.X)
	}
	banner, ok := tc.Find("Working tree clean")
	if !ok {
		t.Fatalf("no clean-tree message: %q", tc.Texts())
	}
	if banner.X <= un.X+200 {
		t.Errorf("the message (x %.0f) must sit in the center, right of the file list (x %.0f)", banner.X, un.X)
	}
	saveEvidence(t, "clean-changes-dark", tc)
	for _, gone := range []string{"Recent Commits", "View All Commits"} {
		if tc.HasText(gone) {
			t.Errorf("%q is a page of its own; a clean tree is the Changes view with nothing in it", gone)
		}
	}
}

// The one action that follows a clean tree stays reachable from the center:
// Push when ahead, Merge when diverged, else Fetch.
func TestCleanChangesOfferTheNextAction(t *testing.T) {
	w, pump, _, _ := pushShell(t)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)
	if !tt.HasText("Push 1 commit") {
		t.Fatalf("an ahead branch offers its push in the center: %q", tt.Texts())
	}
	saveEvidence(t, "clean-ahead-dark", tt)
	var detail string
	for _, s := range tt.Texts() {
		if strings.Contains(s, "ahead of origin/main") {
			detail = s
		}
	}
	if detail == "" {
		t.Errorf("the banner should say where the branch stands: %q", tt.Texts())
	}
}
