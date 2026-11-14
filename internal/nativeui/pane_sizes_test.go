package nativeui

/**
 * [INPUT]: pane_sizes.go、sidebar.go 的 Changes 分隔条、settings.Window.Panes
 * [OUTPUT]: 尺寸记忆合同：拖完分隔条才写偏好（拖的途中不写）、新开的窗口按存下的尺寸站、双击复位清掉存的值、越界的存值被夹住
 * [POS]: 拖拽缝尺寸记忆（2026-11-14）的测试
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"testing"

	"github.com/thewei/solco-git/internal/settings"
)

func TestDraggedPaneSizeIsRememberedAndTakenByTheNextWindow(t *testing.T) {
	root := stagedPaneFixture(t, 8, 3)
	store := settings.NewMemoryStore(settings.Default())
	w := NewWindow(store)
	pump := &uiPump{}
	w.uiApply = pump.post
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)

	if got := w.settings.Window.Panes.StagedHeight; got != 0 {
		t.Fatalf("nothing is stored before a drag: %d", got)
	}
	div, _ := tt.Find("Resize panes")
	y := div.Y + div.H/2
	tt.Press(div.X+40, y)
	tt.Move(div.X+40, y-60)
	settle(tt, pump, 2)
	if got := w.settings.Window.Panes.StagedHeight; got != 0 {
		t.Errorf("a drag in progress is not written: %d", got)
	}
	tt.Release(div.X+40, y-60)
	settle(tt, pump, 4)
	stored := w.settings.Window.Panes.StagedHeight
	if stored == 0 || float32(stored) != float32(int(sh.stagedPaneHeight)) {
		t.Fatalf("the finished drag is stored: stored %d, pane %v", stored, sh.stagedPaneHeight)
	}

	cfg, _ := store.Load(t.Context())
	if cfg.Window.Panes.StagedHeight != stored {
		t.Errorf("the store holds %d, want %d", cfg.Window.Panes.StagedHeight, stored)
	}
	next := NewWindow(store)
	next.uiApply = pump.post
	sh2 := NewShell(next, root)
	if !sh2.stagedPaneSized || int(sh2.stagedPaneHeight) != stored {
		t.Errorf("a new workspace stands at the stored %d: sized %v height %v", stored, sh2.stagedPaneSized, sh2.stagedPaneHeight)
	}

	// A double click on the line is the way back to the default, stored as 0.
	div, _ = tt.Find("Resize panes")
	tt.ClickAt(div.X+40, div.Y+div.H/2)
	tt.ClickAt(div.X+40, div.Y+div.H/2)
	settle(tt, pump, 4)
	if got := w.settings.Window.Panes.StagedHeight; got != 0 {
		t.Errorf("a reset clears the stored height: %d", got)
	}
}

func TestStoredPaneSizesAreClamped(t *testing.T) {
	cfg := settings.Default()
	cfg.Window.Panes = settings.PaneSizes{StagedHeight: -5, HistoryHeight: 999999, FileNavWidth: 300, TerminalHeight: 1 << 30}
	got := normalizeSettings(cfg).Window.Panes
	want := settings.PaneSizes{StagedHeight: 0, HistoryHeight: paneSizeMax, FileNavWidth: 300, TerminalHeight: paneSizeMax}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// A short history gets a short commit list; the review below takes the room.
// Once the user drags the line, their height stands even for a short history.
func TestShortHistoryGetsAShortCommitList(t *testing.T) {
	root := gitFixture(t, "short-history") // two or three commits
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)
	if err := tt.Click("Commits"); err != nil {
		t.Fatal(err)
	}
	framesUntil(t, tt, pump, "the commits", func() bool { return len(sh.git.commits) > 0 })
	settle(tt, pump, 4)

	line, ok := tt.Find("Resize Git history and diff panes")
	if !ok {
		t.Fatalf("no history seam: %q", tt.Texts())
	}
	if line.Y > gitSplitDefaultTop-100 {
		t.Errorf("the list of %d commits stands %.0f high, expected well under the %.0f default", len(sh.git.commits), line.Y, gitSplitDefaultTop)
	}

	sh.gitUpperHeight, sh.gitUpperSized = gitSplitDefaultTop, true
	settle(tt, pump, 4)
	line, _ = tt.Find("Resize Git history and diff panes")
	if line.Y < gitSplitDefaultTop-80 {
		t.Errorf("a chosen height is kept for a short history: line at %.0f", line.Y)
	}
}
