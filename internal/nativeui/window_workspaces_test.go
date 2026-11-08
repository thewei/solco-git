package nativeui

/**
 * [INPUT]: window.go / window_workspaces.go / quick_launch.go / external_open.go、真实临时仓库、headless ui.Tester、文件型设置存储
 * [OUTPUT]: 工作区下拉与管理的合同：标题带的 chip 始终是当前工作区（其余只在列表里）；点开有搜索（过滤、↑↓ 移动、↵ 切换并刷新、无命中时 ↵ 什么也不做）、列表有最大高度并可滚动、行右键移除；选中已在前台的工作区就地刷新、选中另一个读到磁盘上的新改动；设置里的管理（上移/下移/移除，有草稿先确认）；添加只入清单不抢前台（空窗口时除外）、已在清单与非仓库各有提示；每一种打开方式（链接、系统传入的路径、拖入、最近、Quick Launch）都加入清单并显示；旧版的 tabs 键读成工作区、保存只写新键；清单有上限
 * [POS]: 2026-11-08「Solco Git：工作区下拉」决定（用户："改成使用下拉列表来选择…设置里面增加一个工作区管理…下拉搜索…通过我们的 APP 直接打开的情况，就直接增加一个工作区进去"）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/thewei/solco-git/internal/settings"
)

// openThree opens alpha, beta and gamma (gamma ends in front) and returns a
// settled tester.
func openThree(t *testing.T) (*Window, *uiPump, *ui.Tester) {
	t.Helper()
	w, pump := newTestWindow(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		openAndLoad(t, w, pump, gitFixture(t, name))
	}
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	return w, pump, tt
}

func workspaceNames(w *Window) string {
	var names []string
	for _, ws := range w.workspaces {
		names = append(names, filepath.Base(ws.sh.dir))
	}
	return strings.Join(names, ",")
}

// The band names the workspace in front and nothing else; the others are in
// the list the chip opens, which filters as you type and switches on ↵.
func TestTheWorkspaceChipListsSearchesAndSwitches(t *testing.T) {
	w, pump, tt := openThree(t)
	if _, ok := tt.Find("Workspace: gamma"); !ok {
		t.Fatalf("the band has no chip for the workspace in front; texts: %q", tt.Texts())
	}
	if tt.HasText("alpha") || tt.HasText("beta") {
		t.Fatalf("the other workspaces belong in the list, not on the band; texts: %q", tt.Texts())
	}

	if err := tt.Click("Workspace: gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	for _, name := range []string{"alpha", "beta"} {
		if !tt.HasText(name) {
			t.Errorf("the pull-down does not list %s; texts: %q", name, tt.Texts())
		}
	}

	tt.Type("bet")
	settle(tt, pump, 2)
	if tt.HasText("alpha") || !tt.HasText("beta") {
		t.Errorf("typing bet should leave beta only; texts: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	settle(tt, pump, 2)
	if w.active != 1 || w.picker.open {
		t.Fatalf("↵ on the first match: active=%d open=%v, want beta (1) and the list closed", w.active, w.picker.open)
	}
	if _, ok := tt.Find("Workspace: beta"); !ok {
		t.Errorf("the chip does not name beta after the switch; texts: %q", tt.Texts())
	}

	// A query nobody matches says so, and ↵ does nothing.
	if err := tt.Click("Workspace: beta"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	tt.Type("zzz")
	settle(tt, pump, 2)
	if !tt.HasText("No matching workspaces") {
		t.Errorf("an empty result is not explained; texts: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	settle(tt, pump, 2)
	if w.active != 1 || !w.picker.open {
		t.Errorf("↵ with no match changed something: active=%d open=%v", w.active, w.picker.open)
	}
}

// ↑ ↓ move the highlight from the workspace in front, ↵ switches, and a click
// on a row does the same.
func TestTheWorkspaceListIsDrivenByKeysAndClicks(t *testing.T) {
	w, pump, tt := openThree(t)
	if err := tt.Click("Workspace: gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if w.picker.sel != 2 {
		t.Fatalf("the list opens on the workspace in front: sel=%d, want 2", w.picker.sel)
	}
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyUp)
	settle(tt, pump, 1)
	tt.Key(0, ui.KeyEnter)
	settle(tt, pump, 2)
	if w.active != 0 {
		t.Fatalf("↑↑↵ from gamma should land on alpha: active=%d", w.active)
	}

	if err := tt.Click("Workspace: alpha"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if err := tt.Click("gamma"); err != nil {
		t.Fatalf("clicking a row: %v", err)
	}
	settle(tt, pump, 2)
	if w.active != 2 || w.picker.open {
		t.Errorf("a click on gamma: active=%d open=%v", w.active, w.picker.open)
	}
}

// The list is as tall as switcherListMaxHeight and scrolls beyond it, so a
// hundred workspaces do not push the footer off the screen. It opens with the
// workspace in front in view — here the last of 31 — and, having done that
// once, leaves the scrolling to the user.
func TestTheWorkspaceListHasAMaxHeightAndScrolls(t *testing.T) {
	w, pump := newTestWindow(t)
	for i := 0; i < 30; i++ {
		w.addWorkspace(filepath.Join(t.TempDir(), fmt.Sprintf("repo-%02d", i)), "", false)
	}
	openAndLoad(t, w, pump, gitFixture(t, "front")) // the last of 31, and in front
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	if err := tt.Click("Workspace: front"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)

	field, okField := tt.Find("Search workspaces")
	footer, okFooter := tt.Find("Manage Workspaces…")
	if !okField || !okFooter {
		t.Fatalf("the panel has no search field or footer; texts: %q", tt.Texts())
	}
	if span := footer.Y + footer.H - field.Y; span > switcherListMaxHeight+120 {
		t.Errorf("the panel is %.0f points tall for 31 rows; it should stop near %d + chrome", span, switcherListMaxHeight)
	}
	// The row in front is the last: the list opened scrolled to it, the first
	// rows are above the visible part.
	if w.picker.scroll.Y <= 0 {
		t.Errorf("the list did not open on the workspace in front: scrolled %.0f of %.0f", w.picker.scroll.Y, w.picker.scroll.MaxY)
	}
	last, _ := tt.Find("repo-29") // the row above the one in front
	if last.Y < field.Y+field.H || last.Y+last.H > footer.Y {
		t.Errorf("the row above the front one is outside the list: at %.0f, list %.0f–%.0f", last.Y, field.Y+field.H, footer.Y)
	}

	// The user scrolls to the top; nothing drags the list back to the row in
	// front.
	for i := 0; i < 40; i++ {
		tt.Scroll(last.X+last.W/2, field.Y+field.H+40, 0, -120)
		settle(tt, pump, 1)
	}
	if w.picker.scroll.Y != 0 {
		t.Errorf("after scrolling up the list sits at %.0f, not the top", w.picker.scroll.Y)
	}

	// A query starts at the top of its own, shorter list.
	tt.Scroll(last.X+last.W/2, field.Y+field.H+40, 0, 2000)
	settle(tt, pump, 2)
	tt.Type("repo-2")
	settle(tt, pump, 2)
	if w.picker.scroll.Y != 0 || w.picker.sel != 0 {
		t.Errorf("a new query should start at the top: scrolled %.0f, highlight %d", w.picker.scroll.Y, w.picker.sel)
	}
}

// A row's context menu removes the workspace from the list — only from the
// list.
func TestARowRemovesItselfFromTheList(t *testing.T) {
	w, pump, tt := openThree(t)
	if err := tt.Click("Workspace: gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if err := tt.RightClick("beta"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 1)
	if err := tt.ChooseMenuItem("Remove from Workspaces"); err != nil {
		t.Fatalf("menu %q: %v", tt.Menu(), err)
	}
	settle(tt, pump, 2)
	if got := workspaceNames(w); got != "alpha,gamma" {
		t.Errorf("after removing beta: %s", got)
	}
	if w.active != 1 {
		t.Errorf("gamma stays in front at its new place: active=%d", w.active)
	}
	if w.picker.open {
		t.Error("the list closes after a removal")
	}
}

// Choosing the workspace that is already in front is a refresh; choosing
// another one reads its repository again too, so what changed on disk while it
// was away is there when it arrives.
func TestChoosingAWorkspaceReadsItAgain(t *testing.T) {
	w, pump := newTestWindow(t)
	a, b := gitFixture(t, "alpha"), gitFixture(t, "beta")
	shA := openAndLoad(t, w, pump, a)
	shB := openAndLoad(t, w, pump, b)
	has := func(sh *Shell, path string) bool {
		if sh.git.snapshot == nil {
			return false
		}
		for _, f := range sh.git.snapshot.Files {
			if f.Path == path {
				return true
			}
		}
		return false
	}

	// Something changes in alpha while beta is in front.
	writeFile(t, filepath.Join(a, "later.txt"), "new\n")
	if has(shA, "later.txt") {
		t.Fatal("setup: the snapshot already shows the new file")
	}
	w.chooseWorkspace(w.workspaces[0].id)
	pump.until(t, "alpha's second read", func() bool { return has(shA, "later.txt") })
	if w.active != 0 {
		t.Fatalf("choosing alpha: active=%d", w.active)
	}

	// The same for the workspace already in front.
	writeFile(t, filepath.Join(b, "again.txt"), "new\n")
	w.chooseWorkspace(w.workspaces[1].id)
	pump.until(t, "beta's second read", func() bool { return has(shB, "again.txt") })
	w.chooseWorkspace(w.workspaces[1].id) // beta is in front now
	if w.active != 1 {
		t.Fatalf("choosing beta: active=%d", w.active)
	}
	writeFile(t, filepath.Join(b, "third.txt"), "new\n")
	w.chooseWorkspace(w.workspaces[1].id)
	pump.until(t, "the in-place refresh of beta", func() bool { return has(shB, "third.txt") })
	if w.active != 1 {
		t.Errorf("refreshing in place moved the front: active=%d", w.active)
	}
}

// Settings → Workspaces lists them in the pull-down's order with a move and a
// remove control on each row.
func TestSettingsManagesTheWorkspaceList(t *testing.T) {
	w, pump, tt := openThree(t)
	w.openSettings()
	settle(tt, pump, 3)
	if !tt.HasText("Workspaces") || !tt.HasText("Add Workspace…") {
		t.Fatalf("the Workspaces group is not in Settings; texts: %q", tt.Texts())
	}

	if err := tt.Click("Move gamma up"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if got := workspaceNames(w); got != "alpha,gamma,beta" || w.workspaces[w.active].sh.dir == "" || filepath.Base(w.workspaces[w.active].sh.dir) != "gamma" {
		t.Fatalf("after Move gamma up: %s (front %s)", got, filepath.Base(w.workspaces[w.active].sh.dir))
	}
	_ = tt.Click("Move alpha up") // disabled: nothing may happen
	settle(tt, pump, 2)
	if got := workspaceNames(w); got != "alpha,gamma,beta" {
		t.Errorf("the first row has nowhere to move up: %s", got)
	}
	if err := tt.Click("Move alpha down"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if got := workspaceNames(w); got != "gamma,alpha,beta" {
		t.Errorf("after Move alpha down: %s", got)
	}
	// The order is what the next launch restores.
	if got := w.settings.Window.Workspaces; len(got) != 3 || filepath.Base(got[0].Root) != "gamma" || filepath.Base(got[2].Root) != "beta" {
		t.Errorf("saved order = %+v", got)
	}

	if err := tt.Click("Remove beta"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if got := workspaceNames(w); got != "gamma,alpha" {
		t.Errorf("after Remove beta: %s", got)
	}
	// The repository is still remembered: it can be added back from Quick Launch.
	found := false
	for _, root := range w.settings.Window.Recent {
		found = found || filepath.Base(root) == "beta"
	}
	if !found {
		t.Errorf("a removed workspace left Recent too: %v", w.settings.Window.Recent)
	}

	// With work pending, removing asks first and names what would be lost.
	w.workspaces[0].sh.gitQuickSubject = "half-written"
	w.active = 0
	if err := tt.Click("Remove gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if len(w.workspaces) != 2 || !tt.HasText("Remove “gamma” from Workspaces?") {
		t.Errorf("a draft must ask before removing; workspaces=%d texts: %q", len(w.workspaces), tt.Texts())
	}
	if err := tt.Click("Remove Workspace"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if got := workspaceNames(w); got != "alpha" {
		t.Errorf("after confirming: %s", got)
	}
}

// Adding from Settings puts the repository in the list and leaves the front
// where it was; a repository already listed, and a folder that is none, each
// say so.
func TestAddingAWorkspaceJoinsTheListWithoutTakingTheFront(t *testing.T) {
	w, pump := newTestWindow(t)
	front, added := gitFixture(t, "front"), gitFixture(t, "added")
	openAndLoad(t, w, pump, front)

	w.addWorkspacePath(added)
	if got := workspaceNames(w); got != "front,added" || w.active != 0 {
		t.Fatalf("after adding: %s, active=%d; want front,added with front still in front", got, w.active)
	}
	if got := w.settings.Window.Workspaces; len(got) != 2 || got[1].Root != added {
		t.Errorf("the addition was not saved: %+v", got)
	}
	if recent := w.settings.Window.Recent; len(recent) == 0 || recent[0] != added {
		t.Errorf("the addition is not remembered in Recent: %v", recent)
	}

	w.addWorkspacePath(filepath.Join(added, "src", "lib")) // a folder inside it: the same repository
	if len(w.workspaces) != 2 || !strings.Contains(w.toast, "already a workspace") {
		t.Errorf("adding it again: workspaces=%d toast=%q", len(w.workspaces), w.toast)
	}
	w.addWorkspacePath(t.TempDir())
	if len(w.workspaces) != 2 || !strings.Contains(w.toast, "is not a Git repository") {
		t.Errorf("adding a plain folder: workspaces=%d toast=%q", len(w.workspaces), w.toast)
	}

	// With nothing in the list the first addition is the one in front.
	empty, _ := newTestWindow(t)
	empty.addWorkspacePath(front)
	if len(empty.workspaces) != 1 || empty.active != 0 || empty.activeShell() == nil {
		t.Errorf("an empty window: workspaces=%d active=%d", len(empty.workspaces), empty.active)
	}
}

// Every way a repository reaches the window adds it to the list and shows it:
// nothing has to be added by hand first.
func TestEveryWayOfOpeningAddsAWorkspace(t *testing.T) {
	w, _ := newTestWindow(t)
	w.finishAttach()
	link, path, drop, recent, launched := gitFixture(t, "by-link"), gitFixture(t, "by-path"),
		gitFixture(t, "by-drop"), gitFixture(t, "by-recent"), gitFixture(t, "by-launch")

	w.OpenURL("solcogit://open?repo=" + link + "&view=commits")
	w.OpenPath(path)
	w.dropFolders([]string{drop})
	w.noteRecent(recent)
	w.launch(quickItem{Name: "by-recent", Path: recent})
	w.launch(quickItem{Name: "by-launch", Path: launched})
	if got := workspaceNames(w); got != "by-link,by-path,by-drop,by-recent,by-launch" {
		t.Fatalf("workspaces = %s", got)
	}
	if w.active != 4 || filepath.Base(w.activeShell().dir) != "by-launch" {
		t.Errorf("the last one opened is in front: active=%d", w.active)
	}
	if w.workspaces[0].sh.currentView() != viewCommits {
		t.Errorf("the link's view was dropped: %q", w.workspaces[0].sh.currentView())
	}
	if saved := w.settings.Window.Workspaces; len(saved) != 5 {
		t.Errorf("the list was not saved: %+v", saved)
	}
}

// A document from the tab-strip era comes back as workspaces, in the same
// order with the same front, and the next save writes the new key only.
func TestAnOldTabListComesBackAsWorkspaces(t *testing.T) {
	a, b := gitFixture(t, "old-a"), gitFixture(t, "old-b")
	dir := t.TempDir()
	store := settings.NewFileStore(dir)
	legacy := `{"window":{"tabs":[{"root":` + strconv.Quote(a) + `,"view":"files"},{"root":` + strconv.Quote(b) + `}],"active":1}}`
	if err := os.WriteFile(store.Path(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	w := NewWindow(store)
	w.restoreWorkspaces()
	w.finishAttach()
	if got := workspaceNames(w); got != "old-a,old-b" || w.active != 1 {
		t.Fatalf("restored %s with active=%d, want old-a,old-b with 1 in front", got, w.active)
	}
	if w.workspaces[0].sh.currentView() != viewFiles {
		t.Errorf("the old tab's view was lost: %q", w.workspaces[0].sh.currentView())
	}
	w.saveState()
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"tabs"`) || !strings.Contains(string(data), `"workspaces"`) {
		t.Errorf("the save kept the old key:\n%s", data)
	}
}

// The list stops at maxWorkspaces: adding or opening beyond it explains
// itself instead of growing without bound.
func TestTheWorkspaceListIsBounded(t *testing.T) {
	w, _ := newTestWindow(t)
	for i := 0; i < maxWorkspaces; i++ {
		w.addWorkspace(filepath.Join("/fake", "repo", itoa(i)), "", false)
	}
	extra := gitFixture(t, "one-too-many")
	w.addResolved(extra)
	if len(w.workspaces) != maxWorkspaces || !strings.Contains(w.toast, "Remove a workspace") {
		t.Errorf("adding past the cap: workspaces=%d toast=%q", len(w.workspaces), w.toast)
	}
	w.toast = ""
	w.openResolved(extra, "", "")
	if len(w.workspaces) != maxWorkspaces || !strings.Contains(w.toast, "Remove a workspace") {
		t.Errorf("opening past the cap: workspaces=%d toast=%q", len(w.workspaces), w.toast)
	}
}

// The pull-down and the Settings rows are reachable from the keyboard: Tab
// walks to the chip and to each row's move and remove buttons, and Enter on
// the chip opens the list.
func TestWorkspaceControlsAreTabStops(t *testing.T) {
	w, pump, tt := openThree(t)
	reach := func(label string) bool {
		for i := 0; i < 120; i++ {
			tt.Key(0, ui.KeyTab)
			settle(tt, pump, 1)
			if tt.Focused(label) {
				return true
			}
		}
		return false
	}
	if !reach("Workspace: gamma") {
		t.Fatal("Tab never reaches the workspace chip")
	}
	tt.Key(0, ui.KeyEnter)
	settle(tt, pump, 2)
	if !w.picker.open {
		t.Fatal("Enter on the focused chip did not open the list")
	}
	tt.Key(0, ui.KeyEscape)
	settle(tt, pump, 2)
	if w.picker.open {
		t.Error("esc did not close the list")
	}

	w.openSettings()
	settle(tt, pump, 3)
	for _, label := range []string{"Move beta up", "Move beta down", "Remove beta"} {
		if !reach(label) {
			t.Errorf("Tab never reaches %q", label)
		}
	}
}
