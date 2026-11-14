package nativeui

/**
 * [INPUT]: 依赖 window.go 的工作区与状态、window_nav.go 的历史与 Back/Forward 一对、window_workspaces.go 的工作区下拉、frame.go 的侧栏/标题带工具/舞台、welcome.go、window_dialogs.go 与 quick_launch.go 的浮层、panel 动效常量
 * [OUTPUT]: 对外提供 Window.View（根视图：主题、快捷键、toast、每帧把前台位置记进历史、侧栏滑出 + 标题带 + 舞台，或没有工作区时的欢迎页）、sidebarSlide（侧栏的裁剪式宽度动画与拖拽调宽）、sidebarToggle（红绿灯旁的悬浮侧栏钮，与贴侧栏带右缘的 Back/Forward 一对，同 Solco；侧栏收起后一对落在侧栏钮旁）、titleBand（分支 + 工具；工作区下拉只在侧栏收起时才在这里，平时在侧栏页脚）、toggleSidebar（同时收起开着的工作区下拉）
 * [POS]: 窗口的单一帧入口：把当前工作区的工作台装进一个固定的外壳；侧栏宽度/折叠属于窗口（每个工作区共用），工作区只在帧开始时拿到镜像
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// sidebarWidth is the default width of the sidebar.
	sidebarWidth float32 = 252
	// panelSlideDuration matches MyGo's default transition length.
	panelSlideDuration = 200 * time.Millisecond
	// panelSlideEpsilon: widths below this read as fully closed and skip
	// building content, so a resting collapsed sidebar costs nothing.
	panelSlideEpsilon = 0.5
	// sidebarMinWidth keeps the navigator and its foot usable.
	sidebarMinWidth = float32(200)
	// sidebarMaxWidth caps the drag on wide windows.
	sidebarMaxWidth = float32(400)
	// sidebarCenterFloor is the window width reserved for the content column.
	sidebarCenterFloor = float32(480)
	// sidebarResizeHit is the drag handle's width at the sidebar's trailing
	// edge; its trailing 1px is the visible border line, so the sidebar's
	// surface ends exactly at the line.
	sidebarResizeHit = float32(6)
)

// sidebarLimits bounds the sidebar's width in a window this wide. The
// maximum never drops below the minimum.
func sidebarLimits(windowW float32) (minWidth, maxWidth float32) {
	maxWidth = sidebarMaxWidth
	if room := windowW - sidebarCenterFloor; maxWidth > room {
		maxWidth = room
	}
	if maxWidth < sidebarMinWidth {
		maxWidth = sidebarMinWidth
	}
	return sidebarMinWidth, maxWidth
}

// slideWidth drives one panel host's animated width. The host element must
// persist frame-to-frame; its first build seeds at the target, so the very
// first frame never animates. While the resize handle is down the width snaps
// to the target: easing toward a target that moves every frame reads as the
// sidebar lagging the drag.
func slideWidth(host ui.Element, key any, closed bool, openWidth float32, dragging bool) float32 {
	target := openWidth
	if closed {
		target = 0
	}
	if dragging {
		return host.AnimateTo(key, target, ui.Animation{Duration: time.Millisecond})
	}
	return host.Animate(key, target, panelSlideDuration)
}

// View is the window's one frame: the active workspace's workbench inside the
// shared chrome, or the welcome page when no repository is open.
func (w *Window) View(c *ui.Context) {
	// Every popup menu of the window is drawn in-window, in the app's own
	// theme, instead of as an NSMenu popup outside it.
	c.SetMenuPresentation(ui.PopoverMenus)
	theme := solcoTheme(w.resolvedDark(c.Theme().Dark))
	c.SetTheme(theme)
	// The window's sidebar material shows wherever the view draws no
	// background; without one the root keeps the theme's canvas.
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(theme.Background)
	}
	sh := w.activeShell()
	if sh != nil {
		w.handOver(sh)
		sh.frameSync()
		w.observeNav(sh)
	}
	w.shortcuts(c)
	w.toasts(c, sh)

	tokens := designTokens(theme.Dark)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if sh == nil {
				w.welcome(c)
				return
			}
			w.sidebarSlide(c, sh)
			ui.Column(c).Grow(1).MinWidth(0).Background(tokens.Content).Children(func() {
				w.titleBand(c, sh)
				sh.stage(c)
			})
			w.sidebarToggle(c)
		})
	})
	if sh != nil {
		sh.dialogs(c)
	}
	w.windowDialogs(c)
}

// shortcuts are the keys the menu bar cannot carry: ⌘B, a toggle, so no menu
// accelerator that could fire twice with it. Stepping workspaces is the View
// menu's ⇧⌘] / ⇧⌘[ and the Window menu's ⌃⇥ / ⌃⇧⇥ (Control-Tab never reaches
// a view, see shortcuts.go).
func (w *Window) shortcuts(c *ui.Context) {
	if c.Shortcut(sidebarToggleBinding.Modifiers, sidebarToggleBinding.Key) {
		w.toggleSidebar()
	}
}

// toasts flushes what background lanes queued: the window's own notice and
// the front workspace's pending toast and status line.
func (w *Window) toasts(c *ui.Context, sh *Shell) {
	if w.toast != "" {
		c.Toast(w.toast)
		w.toast = ""
	}
	if sh == nil {
		return
	}
	if sh.pendingToast != "" {
		c.Toast(sh.pendingToast)
		sh.pendingToast = ""
	}
	if sh.status == "" {
		sh.statusShown = ""
	} else if sh.status != sh.statusShown {
		c.Toast(sh.status)
		sh.statusShown = sh.status
	}
}

// toggleSidebar shows or hides the sidebar (⌘B, the floating toggle, the View
// menu).
func (w *Window) toggleSidebar() {
	w.sidebarCollapsed = !w.sidebarCollapsed
	w.picker.open = false // its chip changes place with the sidebar
	if sh := w.activeShell(); sh != nil {
		sh.sidebarCollapsed = w.sidebarCollapsed
	}
}

// sidebarSlide mounts the full-height sidebar — its band beside the traffic
// lights, then the workspace's column — inside the persistent slide host, sliding
// out to the left as it hides. With a vibrancy window the column draws
// nothing and the material shows through; without one it falls back to the
// opaque sidebar token. The trailing edge is a drag handle that re-widths the
// sidebar between sidebarLimits, remembered across runs.
func (w *Window) sidebarSlide(c *ui.Context, sh *Shell) {
	full := w.sidebarW + sidebarResizeHit
	host := ui.Row(c.Key("sidebar-slide")).Shrink(0).AlignItems(ui.Stretch)
	width := slideWidth(host, "width", w.sidebarCollapsed, full, w.sidebarWDragging)
	w.sidebarSlideW = width
	if width <= panelSlideEpsilon {
		w.sidebarSlideW = 0
		return
	}
	tokens := sh.tokens(c.Theme().Dark)
	host.Width(width).Clip()
	host.Children(func() {
		ui.Row(c).Width(full).Shrink(0).AlignItems(ui.Stretch).Margin(0, 0, 0, width-full).Children(func() {
			column := ui.Column(c).Width(w.sidebarW).Shrink(0)
			if !c.Vibrancy() {
				column.Background(tokens.Sidebar)
			}
			column.Children(func() {
				ui.Box(c).FillWidth().Shrink(0).Height(max(c.TitleBar().Height, 38)).DragWindow()
				sh.sidebarColumn(c)
			})
			// The line is the handle's trailing pixel: the border sits flush
			// against the content column, never with sidebar material to its
			// right.
			handle := ui.Box(c).Width(sidebarResizeHit).Shrink(0).Cursor(ui.CursorResizeEW).
				AlignItems(ui.End).Label("Resize sidebar")
			handle.Children(func() {
				ui.Box(c).Width(1).FillHeight().Background(splitLineColor(c, tokens, handle))
			})
			dx, _, held := handle.Dragged()
			if handle.DoubleClicked() {
				w.sidebarW = sidebarWidth
				held = true // saved below like a drag's end
			} else if held {
				windowW, _ := c.Size()
				minWidth, maxWidth := sidebarLimits(windowW)
				w.sidebarW = clampF(w.sidebarW+dx, minWidth, maxWidth)
			}
			// Read one frame late by design: the handle builds after the
			// width is applied, so the drag lands on the next frame.
			if w.sidebarWDragging && !held {
				w.saveState() // the drag ended: remember the width
			}
			w.sidebarWDragging = held
		})
	})
}

// sidebarToggleX is where the toggle sits: right after the traffic lights.
func sidebarToggleX(bar ui.TitleBar) float32 { return max(bar.Left, 12) + 4 }

// sidebarToggleClearance is how far from the window's left edge the content
// must start to leave the lights and the floating buttons (the toggle and,
// beside it once the sidebar is shut, Back and Forward) uncovered.
func sidebarToggleClearance(bar ui.TitleBar) float32 {
	return sidebarToggleX(bar) + sidebarChromeStripWidth() + 8
}

// sidebarToggle floats the chrome buttons over whichever column is there (the
// sidebar band when it is open, the title band when it is shut), as Solco's
// do: the sidebar toggle right after the traffic lights, and Back / Forward
// holding the sidebar band's right edge — they settle beside the toggle as
// the sidebar slides shut.
func (w *Window) sidebarToggle(c *ui.Context) {
	k := gorexColorsOf(c.Theme().Dark)
	bar := c.TitleBar()
	label := "Show Sidebar"
	if !w.sidebarCollapsed {
		label = "Hide Sidebar"
	}
	y := (max(bar.Height, 38) - gorexIconBtn) / 2
	ui.Row(c).Absolute().Left(sidebarToggleX(bar)).Top(y).AlignItems(ui.Center).Children(func() {
		toggle := gorexIconButton(c, k, iconPanel, label, "", false, gorexIconBtn, gorexIconGlyph)
		tipped(c, toggle, label, tipKeys(sidebarToggleBinding.keycaps()))
		if toggle.Clicked() {
			w.toggleSidebar()
		}
	})
	right := sidebarToggleX(bar) + sidebarLeftGroupWidth() + sidebarNavGap
	if edge := w.sidebarSlideW - sidebarResizeHit - 12 - sidebarRightGroupWidth(); edge > right {
		right = edge
	}
	ui.Row(c).Absolute().Left(right).Top(y).Gap(sidebarNavGap).AlignItems(ui.Center).Children(func() {
		w.historyButtons(c, k)
	})
}

// titleBand is the content column's top band: the workspace pull-down and the
// branch chip on the left, the tools on the right. Its height is the window
// controls' own, so every control sits on the traffic lights' line.
func (w *Window) titleBand(c *ui.Context, sh *Shell) {
	t := c.Theme()
	k := gorexColorsOf(t.Dark)
	bar := c.TitleBar()
	height := titlebarHeight(bar.Height)
	// While the sidebar slides shut the content column reaches the window's
	// left edge: its content then clears the traffic lights and the toggle.
	left := max(gorexGap*2, sidebarToggleClearance(bar)-w.sidebarSlideW)
	right := max(bar.Right, 12)
	ui.Row(c).Height(height).Padding(0, right, 0, left).Gap(8).DragWindow().AlignItems(ui.Center).Children(func() {
		// The pull-down lives in the sidebar's foot; while the sidebar is shut
		// it rides here so the workspace is never out of reach (Solco shows
		// its search button the same way, only when the sidebar's row is away).
		if w.sidebarCollapsed {
			w.workspaceChip(c, k)
		}
		if sh.git != nil && sh.git.root != "" {
			sh.branchChip(c)
		}
		ui.Box(c).Grow(1).MinWidth(0).PassThrough()
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() { sh.titleTools(c) })
	})
}

// titlebarHeight keeps the band at the window controls' own height: MyGo
// centers the traffic lights inside exactly bar.Height, so centering the
// band's content in that same height lines every control up with the lights.
// The 42 floor is the same in every state, so the chips never move.
func titlebarHeight(barHeight float32) float32 {
	return max(barHeight, 42)
}
