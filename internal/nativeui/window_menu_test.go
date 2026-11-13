package nativeui

/**
 * [INPUT]: window_menu.go MenuBar、headless 窗口
 * [OUTPUT]: 应用菜单合同：About 之后才是 Check for Updates…（只在 main 接上更新器时才有，每次重装菜单都拿一个新的菜单项），Settings… 与 Quit 不丢；File 菜单没有标签的残留（New Tab / Close Tab），⌘W 是 Close Window，View 菜单换工作区的两项带 ⇧⌘] / ⇧⌘[，View 的 Back ⌘[ / Forward ⌘]，Window 菜单的 ⌃⇥ / ⌃⇧⇥ 是可见项且真的换工作区（Hidden 项的键位不触发，整条菜单栏不允许有带键位的 Hidden 项），View ▸ Highlight Moved Lines 默认勾选、选一次关、再选一次开
 * [POS]: 2026-11-07「Solco Git」决定里更新通道（G6）与 2026-11-08 工作区下拉在应用一侧的回归防线；没有更新器的 headless 窗口菜单里一个更新项都没有
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"testing"

	"github.com/egoist/mygo"
)

// appMenuLabels is the first menu's items, by label (role items by role name).
func appMenuLabels(w *Window) []string {
	var out []string
	for _, item := range w.MenuBar().Items()[0].Submenu {
		switch {
		case item.Role != "": // a role fills in its own label ("Quit <app>")
			out = append(out, "role:"+string(item.Role))
		case item.Label != "":
			out = append(out, item.Label)
		default:
			out = append(out, "-")
		}
	}
	return out
}

func indexOf(labels []string, want string) int {
	for i, l := range labels {
		if l == want {
			return i
		}
	}
	return -1
}

func TestAppMenuHasNoUpdateItemUntilTheUpdaterIsWired(t *testing.T) {
	w, _ := newTestWindow(t)
	labels := appMenuLabels(w)
	if indexOf(labels, "Check for Updates…") >= 0 {
		t.Errorf("a window without an updater shows an update item: %q", labels)
	}
	if indexOf(labels, "Settings…") < 0 || indexOf(labels, "role:"+string(mygo.RoleQuit)) < 0 {
		t.Errorf("the app menu lost Settings… or Quit: %q", labels)
	}
}

func TestAppMenuPutsCheckForUpdatesRightAfterAbout(t *testing.T) {
	w, _ := newTestWindow(t)
	made := 0
	w.SetUpdateItem(func() *mygo.MenuItem {
		made++
		return &mygo.MenuItem{Label: "Check for Updates…"}
	})
	labels := appMenuLabels(w)
	if len(labels) < 3 || labels[0] != "role:"+string(mygo.RoleAbout) || labels[1] != "Check for Updates…" || labels[2] != "-" {
		t.Fatalf("app menu begins %q, want About, Check for Updates…, separator", labels)
	}
	if indexOf(labels, "Settings…") < 0 || indexOf(labels, "role:"+string(mygo.RoleQuit)) < 0 {
		t.Errorf("the app menu lost Settings… or Quit: %q", labels)
	}
	// The menu is rebuilt whenever its state changes; each build needs its own item.
	appMenuLabels(w)
	if made != 2 {
		t.Errorf("the updater item was made %d times for 2 menus", made)
	}
}

// fileAndViewItems maps each File and View menu item's label to its accelerator.
func fileAndViewItems(w *Window) map[string]string {
	out := map[string]string{}
	for _, menu := range w.MenuBar().Items() {
		if menu.Label != "File" && menu.Label != "View" {
			continue
		}
		for _, item := range menu.Submenu {
			if item.Label != "" {
				out[item.Label] = item.Accelerator
			}
		}
	}
	return out
}

// The tab strip is gone, so are its menu items: ⌘W is the standard Close
// Window (the window hides, the workspaces stay), there is no New Tab, and the
// two View items that stepped through tabs step through workspaces.
func TestMenusSpeakWorkspacesNotTabs(t *testing.T) {
	w, _ := newTestWindow(t)
	items := fileAndViewItems(w)
	for _, gone := range []string{"New Tab", "Close Tab", "Show Next Tab", "Show Previous Tab"} {
		if _, ok := items[gone]; ok {
			t.Errorf("the menu still has %q", gone)
		}
	}
	for label, accelerator := range map[string]string{
		"Close Window":            accelCloseWindow,
		"Open Repository…":        accelOpen,
		"Quick Launch…":           accelQuickLaunch,
		"Go to File…":             accelGoToFile,
		"Find in Files…":          accelFindInFiles,
		"Show Next Workspace":     accelNextWorkspace,
		"Show Previous Workspace": accelPrevWorkspace,
	} {
		got, ok := items[label]
		if !ok || got != accelerator {
			t.Errorf("%q = %q (present %v), want accelerator %q", label, got, ok, accelerator)
		}
	}
	if accelCloseWindow != "CmdOrCtrl+W" {
		t.Errorf("Close Window is %q, want the standard ⌘W", accelCloseWindow)
	}
}

// menuNamed is a top-level menu's items.
func menuNamed(w *Window, label string) []*mygo.MenuItem {
	for _, menu := range w.MenuBar().Items() {
		if menu.Label == label {
			return menu.Submenu
		}
	}
	return nil
}

func itemNamed(items []*mygo.MenuItem, label string) *mygo.MenuItem {
	for _, item := range items {
		if item.Label == label {
			return item
		}
	}
	return nil
}

// ⌘[ and ⌘] are the View menu's Back and Forward, ⌃⇥ and ⌃⇧⇥ the Window
// menu's workspace stepping: AppKit gives Control-Tab to no view, so a menu
// item is the only way to catch it, and a hidden item's key equivalent does not
// fire (real-machine run 2026-11-08) — so the items are visible, and no item
// of the menu bar may be hidden with a key.
func TestMenusCarryHistoryAndControlTab(t *testing.T) {
	w, pump, tt := openThree(t)
	view, window := menuNamed(w, "View"), menuNamed(w, "Window")
	for label, accelerator := range map[string]string{"Back": accelBack, "Forward": accelForward} {
		item := itemNamed(view, label)
		if item == nil || item.Accelerator != accelerator {
			t.Errorf("View ▸ %s = %+v, want accelerator %q", label, item, accelerator)
		}
	}
	next, prev := itemNamed(window, "Show Next Workspace"), itemNamed(window, "Show Previous Workspace")
	if next == nil || next.Accelerator != "Ctrl+Tab" || prev == nil || prev.Accelerator != "Ctrl+Shift+Tab" {
		t.Fatalf("Window ▸ Show Next/Previous Workspace = %+v / %+v, want Ctrl+Tab / Ctrl+Shift+Tab", next, prev)
	}
	if next.Hidden || prev.Hidden || next.Disabled || prev.Disabled {
		t.Errorf("the Control-Tab items must be visible and enabled: next hidden=%v disabled=%v, prev hidden=%v disabled=%v",
			next.Hidden, next.Disabled, prev.Hidden, prev.Disabled)
	}

	// The items step the workspaces, wrapping.
	if w.active != 2 {
		t.Fatalf("gamma should be in front: active=%d", w.active)
	}
	next.Click(next, nil)
	settle(tt, pump, 2)
	if w.active != 0 {
		t.Errorf("⌃⇥ from the last workspace should wrap to the first: active=%d", w.active)
	}
	prev.Click(prev, nil)
	settle(tt, pump, 2)
	if w.active != 2 {
		t.Errorf("⌃⇧⇥ from the first workspace should wrap to the last: active=%d", w.active)
	}

	var walk func(items []*mygo.MenuItem, path string)
	walk = func(items []*mygo.MenuItem, path string) {
		for _, item := range items {
			if item.Hidden && item.Accelerator != "" {
				t.Errorf("%s%q is hidden with a key equivalent (%s): AppKit never fires it", path, item.Label, item.Accelerator)
			}
			walk(item.Submenu, path+item.Label+" ▸ ")
		}
	}
	for _, menu := range w.MenuBar().Items() {
		walk(menu.Submenu, menu.Label+" ▸ ")
	}
}

// View ▸ Highlight Moved Lines is a checked switch that starts on, turns the
// highlight off when chosen and on again the second time (the settings store
// the reverse, so a fresh settings file means on).
func TestViewMenuHighlightMovedLinesSwitch(t *testing.T) {
	w, pump := newTestWindow(t)
	item := func() *mygo.MenuItem {
		for _, menu := range w.MenuBar().Items() {
			if menu.Label != "View" {
				continue
			}
			for _, it := range menu.Submenu {
				if it.Label == "Highlight Moved Lines" {
					return it
				}
			}
		}
		t.Fatal("the View menu has no Highlight Moved Lines item")
		return nil
	}
	if it := item(); it.Type != mygo.MenuItemCheckbox || !it.Checked || it.Disabled {
		t.Fatalf("item = type %v checked %v disabled %v, want an enabled checked switch", it.Type, it.Checked, it.Disabled)
	}
	item().Click(item(), nil)
	pump.drain()
	if !w.settings.Workbench.HideMovedLines || item().Checked {
		t.Errorf("after choosing it: hide=%v checked=%v, want the highlight off", w.settings.Workbench.HideMovedLines, item().Checked)
	}
	item().Click(item(), nil)
	pump.drain()
	if w.settings.Workbench.HideMovedLines || !item().Checked {
		t.Errorf("after choosing it again: hide=%v checked=%v, want the highlight back on", w.settings.Workbench.HideMovedLines, item().Checked)
	}
}
