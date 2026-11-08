package nativeui

/**
 * [INPUT]: window_nav.go、window_view.go（侧栏带上的 Back/Forward 一对）、window_menu.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: Back/Forward 的合同：历史栈记下（工作区，视图）、同一处不重复、回退后新去处丢掉前方、有上限、移除工作区时把它的位置折掉；每条导航路径（下拉切换、视图标签、菜单步进）都进历史且 Back/Forward 不自己再造一条；Back/Forward 跨工作区也跨视图；Commit 编辑器未收起时按钮不动、游标也不动；按钮与 Solco 同位（侧栏开着时在带的右缘，收起后落在侧栏钮旁，内容给三颗钮让位）、无处可去时是禁用脸；View 菜单的 Back ⌘[ / Forward ⌘] 与历史同步
 * [POS]: 2026-11-08「Solco Git 侧栏：前进后退」决定（用户："Solco Git 侧栏，像我们 Solco 一样增加前进后退按钮"）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"math"
	"testing"
)

func at(ws int, v viewKind) navLocation { return navLocation{workspace: ws, view: v} }

// Every place the window goes is recorded once; going to a new place from
// the middle of the stack drops what lay ahead, as a browser does.
func TestNavHistoryRecordsPlacesAndDropsTheForwardOnes(t *testing.T) {
	var h navHistory
	if h.canBack() || h.canForward() {
		t.Fatalf("an empty history can go somewhere: back=%v forward=%v", h.canBack(), h.canForward())
	}
	if !h.observe(at(1, viewChanges)) {
		t.Fatal("the first place was not recorded")
	}
	if h.observe(at(1, viewChanges)) {
		t.Error("the same place again was recorded as a new one")
	}
	h.observe(at(1, viewCommits))
	h.observe(at(2, viewCommits))
	if !h.canBack() || h.canForward() || len(h.entries) != 3 {
		t.Fatalf("after three places: back=%v forward=%v entries=%d", h.canBack(), h.canForward(), len(h.entries))
	}

	h.at-- // Back, to (1, commits)
	if !h.canBack() || !h.canForward() {
		t.Fatalf("in the middle both ways should be open: back=%v forward=%v", h.canBack(), h.canForward())
	}
	if next, ok := h.peek(1); !ok || next != at(2, viewCommits) {
		t.Errorf("Forward leads to %v (%v), want (2, commits)", next, ok)
	}
	h.observe(at(3, viewChanges)) // somewhere new: (2, commits) is no longer ahead
	if h.canForward() || len(h.entries) != 3 || h.entries[2] != at(3, viewChanges) {
		t.Errorf("a new place did not replace the forward ones: %v (forward %v)", h.entries, h.canForward())
	}
}

// The stack is a session of hopping, not a log: it keeps the newest places.
func TestNavHistoryIsBounded(t *testing.T) {
	var h navHistory
	for i := 0; i < navHistoryMax*2; i++ {
		h.observe(at(i, viewChanges))
	}
	if len(h.entries) != navHistoryMax {
		t.Fatalf("the history holds %d places, want the cap %d", len(h.entries), navHistoryMax)
	}
	if cur, _ := h.current(); cur != at(navHistoryMax*2-1, viewChanges) {
		t.Errorf("the current place is %v, want the newest", cur)
	}
	if h.entries[0] != at(navHistoryMax, viewChanges) {
		t.Errorf("the oldest kept place is %v, want the %dth", h.entries[0], navHistoryMax)
	}
}

// A workspace that left the list takes its places with it; neighbours that
// become the same place fold, and the cursor stays where it was.
func TestNavHistoryForgetsAWorkspaceAndFoldsNeighbours(t *testing.T) {
	build := func(cursor int) navHistory {
		h := navHistory{entries: []navLocation{at(1, viewChanges), at(2, viewChanges), at(1, viewChanges), at(3, viewFiles)}}
		h.at = cursor
		return h
	}

	h := build(3) // on (3, files)
	h.forget(2)
	if len(h.entries) != 2 || h.entries[0] != at(1, viewChanges) || h.entries[1] != at(3, viewFiles) || h.at != 1 {
		t.Errorf("forgetting a middle workspace: entries=%v at=%d, want [(1,changes) (3,files)] at 1", h.entries, h.at)
	}

	h = build(1) // on the workspace that goes away
	h.forget(2)
	if cur, ok := h.current(); !ok || cur != at(1, viewChanges) {
		t.Errorf("the cursor on a removed place lands on %v (%v), want the place before it", cur, ok)
	}

	h = build(0)
	h.forget(1)
	if cur, ok := h.current(); ok {
		t.Errorf("with nothing before the cursor it should point at nothing, got %v", cur)
	}
	if h.observe(at(2, viewChanges)); len(h.entries) != 1 {
		t.Errorf("the next observation should start the history again: %v", h.entries)
	}
}

// positions is a copy of the history's places, oldest first.
func positions(w *Window) []navLocation { return append([]navLocation(nil), w.nav.entries...) }

// Back and Forward go to the workspace and the view of the place, whatever
// way it was reached, and walking the history never writes to it.
func TestBackAndForwardWalkWorkspacesAndViews(t *testing.T) {
	w, pump, tt := openThree(t) // alpha, beta, gamma; gamma in front on Changes
	alpha, beta, gamma := w.workspaces[0], w.workspaces[1], w.workspaces[2]

	w.activeShell().showView(viewCommits) // (gamma, commits)
	settle(tt, pump, 3)
	w.chooseWorkspace(alpha.id) // (alpha, changes)
	settle(tt, pump, 3)
	w.activeShell().showView(viewFiles) // (alpha, files)
	settle(tt, pump, 3)
	w.stepWorkspace(1) // the next workspace in the list: beta, on its own view
	settle(tt, pump, 3)

	want := []navLocation{at(gamma.id, viewChanges), at(gamma.id, viewCommits), at(alpha.id, viewChanges), at(alpha.id, viewFiles), at(beta.id, viewChanges)}
	got := positions(w)
	if len(got) != len(want) {
		t.Fatalf("the history is %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history[%d] = %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}

	expect := func(label string, ws *workspace, view viewKind, back, forward bool) {
		t.Helper()
		settle(tt, pump, 3)
		if w.workspaces[w.active] != ws || ws.sh.currentView() != view {
			t.Fatalf("%s: front=%s view=%s, want %s on %s", label, pathBase(w.workspaces[w.active].sh.dir), w.workspaces[w.active].sh.currentView(), pathBase(ws.sh.dir), view)
		}
		if w.nav.canBack() != back || w.nav.canForward() != forward {
			t.Fatalf("%s: back=%v forward=%v, want %v %v", label, w.nav.canBack(), w.nav.canForward(), back, forward)
		}
		if len(w.nav.entries) != len(want) {
			t.Fatalf("%s: walking changed the history: %v", label, w.nav.entries)
		}
	}

	w.navBack()
	expect("Back to alpha's Files", alpha, viewFiles, true, true)
	w.navBack()
	expect("Back to alpha's Changes (the view changes with it)", alpha, viewChanges, true, true)
	w.navBack()
	expect("Back to gamma's Commits (another workspace)", gamma, viewCommits, true, true)
	w.navBack()
	expect("Back to gamma's Changes", gamma, viewChanges, false, true)
	w.navBack() // the end: nothing happens
	expect("Back at the start does nothing", gamma, viewChanges, false, true)
	for range want[1:] {
		w.navForward()
		settle(tt, pump, 2)
	}
	expect("Forward all the way to beta", beta, viewChanges, true, false)
	w.navForward()
	expect("Forward at the end does nothing", beta, viewChanges, true, false)

	// From the middle, somewhere new replaces what lay ahead.
	w.navBack()
	w.navBack()
	settle(tt, pump, 3)
	w.activeShell().showView(viewCommits) // alpha's Changes → alpha's Commits
	settle(tt, pump, 3)
	if w.nav.canForward() || len(w.nav.entries) != 4 || w.nav.entries[3] != at(alpha.id, viewCommits) {
		t.Errorf("a new place after Back kept the forward ones: %v", w.nav.entries)
	}
}

// The buttons do the same as the calls: a real click on Back, on Forward.
func TestTheHistoryButtonsClick(t *testing.T) {
	w, pump, tt := openThree(t)
	w.chooseWorkspace(w.workspaces[0].id) // gamma → alpha
	settle(tt, pump, 3)

	if err := tt.Click("Back"); err != nil {
		t.Fatalf("clicking Back: %v", err)
	}
	settle(tt, pump, 3)
	if w.active != 2 {
		t.Fatalf("Back from alpha should return to gamma: active=%d", w.active)
	}
	if err := tt.Click("Forward"); err != nil {
		t.Fatalf("clicking Forward: %v", err)
	}
	settle(tt, pump, 3)
	if w.active != 0 {
		t.Errorf("Forward should lead to alpha again: active=%d", w.active)
	}
}

// With nothing behind (or ahead) the button is the shared disabled face: it
// does nothing when pressed.
func TestHistoryButtonsWithNowhereToGoDoNothing(t *testing.T) {
	w, pump, tt := openThree(t)
	if w.nav.canBack() || w.nav.canForward() {
		t.Fatalf("a freshly opened window has history: %v", w.nav.entries)
	}
	_ = tt.Click("Back")
	_ = tt.Click("Forward")
	settle(tt, pump, 3)
	if w.active != 2 || len(w.nav.entries) != 1 {
		t.Errorf("a disabled button did something: active=%d entries=%v", w.active, w.nav.entries)
	}
}

// The Commit editor is a modal state of its workspace: while it is open (and
// has words in it) Back neither leaves nor moves the cursor, like every other
// way out.
func TestBackIsHeldByTheCommitEditor(t *testing.T) {
	w, pump, tt := openThree(t)
	w.chooseWorkspace(w.workspaces[0].id) // gamma → alpha: there is a Back
	settle(tt, pump, 3)
	sh := w.activeShell()
	sh.openCommitSurface()
	sh.surface.commit.Subject = "work in progress"
	settle(tt, pump, 2)
	cursor := w.nav.at

	w.navBack()
	settle(tt, pump, 2)
	if w.active != 0 || w.nav.at != cursor {
		t.Errorf("Back left a Commit draft: active=%d cursor %d→%d", w.active, cursor, w.nav.at)
	}
	if !sh.commitDiscardPrompt {
		t.Error("the held Back did not ask what to do with the draft")
	}
}

// A workspace removed from the list is nowhere Back can lead.
func TestRemovingAWorkspaceDropsItsPlaces(t *testing.T) {
	w, pump, tt := openThree(t)
	beta := w.workspaces[1]
	w.chooseWorkspace(beta.id) // (beta, changes)
	settle(tt, pump, 3)
	w.chooseWorkspace(w.workspaces[0].id) // (alpha, changes)
	settle(tt, pump, 3)

	w.removeWorkspace(w.indexOfID(beta.id))
	settle(tt, pump, 3)
	for _, place := range w.nav.entries {
		if place.workspace == beta.id {
			t.Fatalf("a removed workspace is still in the history: %v", w.nav.entries)
		}
	}
	for w.nav.canBack() {
		w.navBack()
		settle(tt, pump, 2)
		if w.active < 0 || w.active >= len(w.workspaces) {
			t.Fatalf("Back led to no workspace: active=%d", w.active)
		}
	}
}

// The pair holds the sidebar band's right edge while the sidebar is open, as
// Solco's does, then settles beside the toggle when the sidebar is shut, and
// the content clears all three buttons either way.
func TestHistoryButtonsHoldTheSidebarEdgeThenSettleBesideTheToggle(t *testing.T) {
	w, pump, tt := openThree(t)
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1 }

	back, okB := tt.Find("Back")
	forward, okF := tt.Find("Forward")
	if !okB || !okF {
		t.Fatalf("Back/Forward are not on the screen: %q", tt.Texts())
	}
	if !near(forward.X+forward.W, w.sidebarW-12) {
		t.Errorf("open sidebar: Forward ends at %.1f, want the band's right edge less 12 = %.1f", forward.X+forward.W, w.sidebarW-12)
	}
	if !near(forward.X-(back.X+back.W), sidebarNavGap) || back.Y != forward.Y {
		t.Errorf("Back %v and Forward %v are not one tight row", back, forward)
	}
	toggle, _ := tt.Find("Hide Sidebar")
	if back.X < toggle.X+toggle.W {
		t.Errorf("Back (%v) overlaps the sidebar toggle (%v)", back, toggle)
	}

	w.toggleSidebar()
	settle(tt, pump, 4)
	toggle, _ = tt.Find("Show Sidebar")
	back, _ = tt.Find("Back")
	forward, _ = tt.Find("Forward")
	if !near(back.X, toggle.X+toggle.W+sidebarNavGap) {
		t.Errorf("shut sidebar: Back at %.1f, want beside the toggle at %.1f", back.X, toggle.X+toggle.W+sidebarNavGap)
	}
	chip, ok := tt.Find("Workspace: gamma")
	if !ok {
		t.Fatalf("with the sidebar shut the band must carry the workspace chip: %q", tt.Texts())
	}
	if chip.X < forward.X+forward.W {
		t.Errorf("the band's content (%v) starts under the history buttons (%v)", chip, forward)
	}
}

// The View menu's Back and Forward are enabled exactly when the buttons are,
// and the menu is rebuilt when that changes.
func TestTheViewMenuFollowsTheHistory(t *testing.T) {
	w, pump, tt := openThree(t)
	view := func() map[string]bool { // label → disabled
		out := map[string]bool{}
		for _, menu := range w.MenuBar().Items() {
			if menu.Label != "View" {
				continue
			}
			for _, item := range menu.Submenu {
				switch item.Label {
				case "Back", "Forward":
					out[item.Label] = item.Disabled
				}
			}
		}
		return out
	}
	before := w.currentMenuState()
	if got := view(); !got["Back"] || !got["Forward"] {
		t.Fatalf("a window with no history enables Back/Forward: %v", got)
	}
	w.chooseWorkspace(w.workspaces[0].id)
	settle(tt, pump, 3)
	if got := view(); got["Back"] || !got["Forward"] {
		t.Errorf("after one move Back should be enabled and Forward not: disabled=%v", got)
	}
	if w.currentMenuState() == before {
		t.Error("the menu state did not change with the history: the menu bar would keep stale items")
	}
	w.navBack()
	settle(tt, pump, 3)
	if got := view(); !got["Back"] || got["Forward"] {
		t.Errorf("after Back, Forward should be enabled and Back not: disabled=%v", got)
	}
}
