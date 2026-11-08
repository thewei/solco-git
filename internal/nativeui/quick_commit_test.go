package nativeui

import "testing"

// A real click on Commit works with the composer idle. The press must not move
// the focus onto the button: focus in the form opens it and moves the buttons to
// the options row mid-click, and the release then lands on a new button — the
// first click only opened the form (real-machine run, 2026-11-08).
func TestQuickCommitButtonWorksOnTheFirstClick(t *testing.T) {
	root := gitFixture(t, "quick")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)

	button, ok := tt.Find("Commit")
	if !ok {
		t.Fatalf("no Commit button: %q", tt.Texts())
	}
	x, y := button.X+button.W/2, button.Y+button.H/2
	tt.Move(x, y)
	tt.Frame()
	tt.Press(x, y)
	tt.Frame() // the press would focus the form: it opens and the buttons move
	tt.Frame()
	tt.Release(x, y)
	settle(tt, pump, 2)
	if sh.surface.current() != WorkspaceSurfaceCommit || !sh.commitDialogOpen {
		t.Fatalf("one press and release on Commit left the surface at %v (dialog open %v)", sh.surface.current(), sh.commitDialogOpen)
	}
}
