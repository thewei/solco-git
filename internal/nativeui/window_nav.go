package nativeui

/**
 * [INPUT]: 依赖 window.go 的工作区清单（indexOfID/selectWorkspace/activeShell）、view.go 的 viewKind 与 Shell.showView/currentView（含提交草稿拦截）、gorex_style.go 的图标按钮、shortcuts.go 的 accelBack/accelForward、tooltip.go 的 tipped
 * [OUTPUT]: 对外提供 navLocation（一处可回到的位置：工作区 id + 视图）、navHistory（observe/step/forget 的纯历史栈，上限 navHistoryMax）、Window.observeNav（每帧把前台位置记进历史）、Window.navBack/navForward（按钮、⌘[ ⌘] 菜单项的唯一入口）、historyButtons（侧栏带右缘的 Back/Forward 一对，与 Solco 同位同形）
 * [POS]: 取代 Solco 的 Router 历史：Solco Git 没有页面栈，「页面」就是（工作区，视图）；视图内部选中了什么（哪条提交、哪个文件）留在工作区自己手里，回来时原样还在，所以历史不记它；历史只在本次运行里，不落盘
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import "github.com/egoist/mygo/ui"

// navHistoryMax bounds the stack: a session of view hopping, not a log.
const navHistoryMax = 64

// navLocation is a place Back and Forward can return to: a workspace and the
// view it shows. What a view shows inside (the picked commit, the open file)
// stays with the workspace, so a return finds it as it was left.
type navLocation struct {
	workspace int // the workspace's id, never reused
	view      viewKind
}

// navHistory is the Back / Forward stack: the locations the window has been
// on, and the one it is on now.
type navHistory struct {
	entries []navLocation
	at      int // index of the current location; meaningless while empty
}

func (h *navHistory) current() (navLocation, bool) {
	if h.at < 0 || h.at >= len(h.entries) {
		return navLocation{}, false
	}
	return h.entries[h.at], true
}

// observe records where the window is. The same place again changes nothing;
// a new place drops whatever lay forward of the current one, like a browser.
func (h *navHistory) observe(loc navLocation) bool {
	if cur, ok := h.current(); ok && cur == loc {
		return false
	}
	if len(h.entries) > 0 {
		h.entries = h.entries[:h.at+1]
	}
	h.entries = append(h.entries, loc)
	if over := len(h.entries) - navHistoryMax; over > 0 {
		h.entries = h.entries[over:]
	}
	h.at = len(h.entries) - 1
	return true
}

func (h *navHistory) canBack() bool    { return h.at > 0 && h.at < len(h.entries) }
func (h *navHistory) canForward() bool { return h.at >= 0 && h.at+1 < len(h.entries) }

// peek is the location delta (-1 back, +1 forward) away, without moving.
func (h *navHistory) peek(delta int) (navLocation, bool) {
	i := h.at + delta
	if h.at < 0 || i < 0 || i >= len(h.entries) {
		return navLocation{}, false
	}
	return h.entries[i], true
}

// forget drops every location of a workspace that left the list, keeping the
// cursor on the same place (or the nearest one before it) and folding the
// neighbours that became the same place.
func (h *navHistory) forget(workspace int) {
	kept := make([]navLocation, 0, len(h.entries))
	at := -1
	for i, e := range h.entries {
		if e.workspace == workspace {
			continue
		}
		if n := len(kept); n > 0 && kept[n-1] == e {
			if i <= h.at {
				at = n - 1
			}
			continue
		}
		kept = append(kept, e)
		if i <= h.at {
			at = len(kept) - 1
		}
	}
	h.entries, h.at = kept, at
}

// observeNav is the frame's one line of history keeping: wherever the user
// went — a view tab, ⌘1–3, the pull-down, a link, a step — the front workspace
// and its view are what the history records, so no navigation path has to
// remember to.
func (w *Window) observeNav(sh *Shell) {
	if w.active < 0 || w.active >= len(w.workspaces) || sh == nil {
		return
	}
	if w.nav.observe(navLocation{workspace: w.workspaces[w.active].id, view: sh.currentView()}) {
		w.refreshMenu() // Back and Forward in the View menu follow the stack
	}
}

func (w *Window) navBack()    { w.navMove(-1) }
func (w *Window) navForward() { w.navMove(1) }

// navMove walks one step of the history: the workspace comes to the front
// (read again on the way in, as choosing it from the pull-down does) and shows
// the view it had. A Commit draft still open holds the window where it is, as
// it holds every other navigation, and then the cursor stays too.
func (w *Window) navMove(delta int) {
	target, ok := w.nav.peek(delta)
	if !ok {
		return
	}
	i := w.indexOfID(target.workspace)
	if i < 0 { // a workspace that left the list: drop its places and try again
		w.nav.forget(target.workspace)
		w.navMove(delta)
		return
	}
	if from := w.activeShell(); from != nil && from.holdCommitNavigation() {
		return
	}
	w.nav.at += delta
	if i != w.active {
		w.selectWorkspace(i)
	}
	if sh := w.workspaces[i].sh; sh.currentView() != target.view {
		sh.showView(target.view)
	}
	w.refreshMenu()
}

// sidebarNavGap is the gap inside the floating chrome cluster: within each
// group and between them while the sidebar is shut.
const sidebarNavGap float32 = 4

// sidebarLeftGroupWidth is the left group's width: the sidebar toggle with the
// sidebar's search and Refresh beside it.
func sidebarLeftGroupWidth() float32 { return 3*gorexIconBtn + 2*sidebarNavGap }

// sidebarRightGroupWidth is the history pair holding the sidebar band's right
// edge, as Solco's Back and Forward do.
func sidebarRightGroupWidth() float32 { return 2*gorexIconBtn + sidebarNavGap }

// sidebarChromeStripWidth is the widest the floating chrome gets: with the
// sidebar shut the pair settles beside the left group, and the content clears
// all five buttons.
func sidebarChromeStripWidth() float32 {
	return sidebarLeftGroupWidth() + sidebarNavGap + sidebarRightGroupWidth()
}

// historyButtons are Back and Forward: the shared icon button while there is a
// place to reach, the shared disabled face (half-strength glyph, no hover)
// where the history ends. Their tooltips name the View menu's keys.
func (w *Window) historyButtons(c *ui.Context, k *gorexColors) {
	one := func(glyph *ui.SVG, label, accelerator string, can bool, move func()) {
		keys := tipKeys(acceleratorKeycaps(accelerator))
		if !can {
			tipped(c, gorexIconButtonDisabled(c, k, glyph, label, "", gorexIconBtn, gorexIconGlyph), label, keys)
			return
		}
		b := gorexIconButton(c, k, glyph, label, "", false, gorexIconBtn, gorexIconGlyph)
		tipped(c, b, label, keys)
		if b.Clicked() {
			move()
		}
	}
	one(iconArrowLeft, "Back", accelBack, w.nav.canBack(), w.navBack)
	one(iconArrowRight, "Forward", accelForward, w.nav.canForward(), w.navForward)
}
