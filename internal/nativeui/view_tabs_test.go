package nativeui

/**
 * [INPUT]: frame.go（viewTabs）、ds_components.go（materialNavGroup / materialNavRow、materialFace 与 materialSegment*）、git_sidebar_tree.go（segmentTabs）、files_git.go（filesScopeTabs）、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 侧栏三行视图的合同：Changes / Commits / Files 一行一个图形加名称、自上而下排成一列、整行宽，名称在任何宽度下都画出；Changes 的计数贴行尾、在任何宽度下都完整；行在 vibrancy 材质上不画纸（选中是侧栏选中色的一层淡墨整行铺满，不是白底/不透明槽），侧栏里另外两个两段开关（Show / Search in）同样
 * [POS]: 2026-11-12「Solco Git on LazyGit's command layer …」决定第 (5) 条（视图开关从页签条改为三行）与 2026-11-08 的材质原则（用户："现在白底的话或者颜色直接显示出来，有点难以融入我们现在侧栏的半透明的效果"）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"fmt"
	"image"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The sidebar's three view tabs share one narrow track, and the Changes tab
// carries the count of changed files. Tabs that each start from nothing split
// that track in three and clipped "Changes 330" to "Chan… 3…" on the real
// machine (2026-11-08): the count must show whole at any size.
func TestViewTabsKeepTheChangeCountWhole(t *testing.T) {
	root := gitFixture(t, "bulk")
	// Three hundred tracked files, then all modified: a three-digit count.
	for i := 0; i < 330; i++ {
		writeFile(t, filepath.Join(root, "bulk", fmt.Sprintf("f%03d.txt", i)), "a\n")
	}
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "bulk")
	for i := 0; i < 330; i++ {
		writeFile(t, filepath.Join(root, "bulk", fmt.Sprintf("f%03d.txt", i)), "b\n")
	}

	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	n := len(sh.git.snapshot.Files)
	if n < 100 {
		t.Fatalf("the fixture has %d changed files, want a three-digit count", n)
	}
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)

	var want float32
	measure := ui.NewTester(func(c *ui.Context) {
		want, _ = c.MeasureText(0, ui.Span{Text: strconv.Itoa(n), Size: Typography().Caption, Weight: WeightSemibold})
	}, 100, 100)
	measure.Frame()

	got, ok := tt.Find(strconv.Itoa(n))
	if !ok {
		t.Fatalf("no count %d on screen: %q", n, tt.Texts())
	}
	if got.W < want-0.5 {
		t.Errorf("count %d is clipped to %.1f of %.1f px", n, got.W, want)
	}
	// And the tab is the first thing that shows it, not a header further down.
	if got.X > float32(sh.sidebarW) {
		t.Errorf("the first %d is at x=%.0f, outside the %v px sidebar: not the Changes tab", n, got.X, sh.sidebarW)
	}
}

// stripRects reads the three view rows' boxes off the accessibility tree: the
// rows are the radios of the "View" group, named by their view.
func stripRects(t *testing.T, tt *ui.Tester) (rects [len(sidebarViews)]ui.Rect, checked int) {
	t.Helper()
	checked = -1
	for i, v := range sidebarViews {
		a, ok := tt.AccessByRole(ui.RoleRadio, v.label)
		if !ok {
			t.Fatalf("no %q tab in the accessibility tree", v.label)
		}
		rects[i] = a.Bounds
		if a.Checked {
			if checked >= 0 {
				t.Errorf("tabs %d and %d are both chosen", checked, i)
			}
			checked = i
		}
	}
	return rects, checked
}

// stripTexts is what the strip lists on screen, in order: the entries between
// its group label "View" and the repository name that opens the sidebar's head.
// A tab appears once by its accessible name and once more for each text it
// draws, so a name listed twice is a name that is drawn.
func stripTexts(t *testing.T, tt *ui.Tester, repo string) []string {
	t.Helper()
	all := tt.Texts()
	// The strip ends with the last view's name (each is listed twice: the row
	// and its text); the repository's name no longer follows it in the sidebar.
	last := sidebarViews[len(sidebarViews)-1].label
	start := slices.Index(all, "View")
	end := slices.Index(all, last)
	if start < 0 || end < start {
		t.Fatalf("no tab strip in %q", all)
	}
	return all[start+1 : end+2]
}

func occurrences(texts []string, s string) int {
	n := 0
	for _, text := range texts {
		if text == s {
			n++
		}
	}
	return n
}

// The three views are three rows, each with its glyph and its name whichever is
// chosen: stacked top to bottom in order, one row height each, the same x and
// the full width between the sidebar's gutters, at the narrowest, the default
// and the widest sidebar.
func TestViewRowsAreThreeStackedRows(t *testing.T) {
	root := gitFixture(t, "tabs")
	for _, width := range []float32{sidebarMinWidth, sidebarWidth, sidebarMaxWidth} {
		w, pump := newTestWindow(t)
		sh := openAndLoad(t, w, pump, root)
		w.sidebarW = width
		tt := newTester(w, 1280, 800, false)
		for want, view := range sidebarViews {
			sh.showView(view.kind)
			settle(tt, pump, 3)
			rects, got := stripRects(t, tt)
			if got != want {
				t.Fatalf("width %.0f, %s: the chosen row is %d, want %d", width, view.label, got, want)
			}
			texts := stripTexts(t, tt, "tabs")
			rowH := sh.sidebarRowHeight()
			for i, r := range rects {
				if r.H != rowH {
					t.Errorf("width %.0f, %s: row %d is %.1f high, want %.1f", width, view.label, i, r.H, rowH)
				}
				if r.X != rects[0].X || r.W != rects[0].W {
					t.Errorf("width %.0f, %s: row %d (%v) is not in the first row's column (%v)", width, view.label, i, r, rects[0])
				}
				if i > 0 && r.Y < rects[i-1].Y+rects[i-1].H {
					t.Errorf("width %.0f, %s: row %d %v does not sit below row %d %v", width, view.label, i, r, i-1, rects[i-1])
				}
				if occurrences(texts, sidebarViews[i].label) != 2 {
					t.Errorf("width %.0f, %s: row %d's name is not drawn (list: %q)", width, view.label, i, texts)
				}
			}
			first := rects[0]
			if first.X < sidebarGutter-0.5 || first.X+first.W > width-sidebarGutter+0.5 || first.W < width-2*sidebarGutter-0.5 {
				t.Errorf("width %.0f, %s: the rows span %.1f..%.1f, want the full %d-point gutters", width, view.label, first.X, first.X+first.W, int(sidebarGutter))
			}
			// The fixture has two changed files: the count shows once whichever
			// view is chosen, at the right edge of the Changes row.
			if occurrences(texts, "2") != 1 {
				t.Errorf("width %.0f, %s: the Changes count is not listed once (list: %q)", width, view.label, texts)
			}
			if c, ok := tt.Find("2"); !ok || c.X+c.W > rects[0].X+rects[0].W || c.X < rects[0].X+rects[0].W/2 {
				t.Errorf("width %.0f, %s: the count %+v is not at the Changes row's right edge %v", width, view.label, c, rects[0])
			}
		}
	}
}

// luminance is the sum of a pixel's channels: enough to tell a wash of ink
// (darker on the light sidebar, lighter on the dark one) from a sheet of paper.
func luminance(img *image.RGBA, x, y float32) int {
	c := img.RGBAAt(int(math.Round(float64(x))), int(math.Round(float64(y))))
	return int(c.R) + int(c.G) + int(c.B)
}

// assertWash checks the paint just inside a segment's left edge (clear of its
// content) against the sidebar's own fill beside the control: the chosen
// segment is a faint wash of ink — never paper — and every other one is the
// sidebar itself.
func assertWash(t *testing.T, tt *ui.Tester, what string, seg ui.Rect, dark, chosen bool) {
	t.Helper()
	img := tt.Image()
	mid := seg.Y + seg.H/2
	sidebar := luminance(img, 3, mid)
	here := luminance(img, seg.X+3, mid)
	switch {
	case !chosen:
		if d := here - sidebar; d < -2 || d > 2 {
			t.Errorf("%s: an idle segment is painted (%d against the sidebar's %d)", what, here, sidebar)
		}
	case dark && here <= sidebar, !dark && here >= sidebar:
		t.Errorf("%s: the chosen segment (%d) is not a wash of ink on the sidebar (%d), dark=%v", what, here, sidebar, dark)
	case math.Abs(float64(here-sidebar)) > 80:
		t.Errorf("%s: the chosen segment (%d) is a slab against the sidebar's %d, not a wash", what, here, sidebar)
	}
}

// The view rows sit on the sidebar's vibrancy: the chosen row is a wash of ink
// and the others are bare — no white pill, no opaque track — in both windows.
func TestViewTabsAreAWashNotPaper(t *testing.T) {
	root := gitFixture(t, "wash")
	for _, dark := range []bool{false, true} {
		w, pump := newTestWindow(t)
		sh := openAndLoad(t, w, pump, root)
		tt := newTester(w, 1280, 800, dark)
		for want, view := range sidebarViews {
			sh.showView(view.kind)
			settle(tt, pump, 3)
			rects, _ := stripRects(t, tt)
			for i, r := range rects {
				assertWash(t, tt, fmt.Sprintf("dark=%v, %s chosen, tab %q", dark, view.label, sidebarViews[i].label), r, dark, i == want)
			}
			// Nothing is drawn behind the rows either: no track, and the gap
			// between two rows is the sidebar itself.
			img := tt.Image()
			y := rects[0].Y + rects[0].H/2
			for _, gap := range []float32{sidebarGutter / 2, rects[0].X + rects[0].W + sidebarGutter/2} {
				if d := luminance(img, gap, y) - luminance(img, 1, y); d < -2 || d > 2 {
					t.Errorf("dark=%v, %s chosen: the rows have a track (%d at x=%.0f)", dark, view.label, d, gap)
				}
			}
			saveEvidence(t, fmt.Sprintf("view-tabs-%s-%s", map[bool]string{false: "light", true: "dark"}[dark], view.label), tt)
		}
	}
}

// The two other switches that sit on the sidebar — the Files tree's
// "Changed | All" and the search's "Files | Content" — wear the same wash, so
// the sidebar holds one kind of segmented control.
func TestSidebarSwitchesAreAWashNotPaper(t *testing.T) {
	for _, dark := range []bool{false, true} {
		w, pump, sh, _ := filesFixtureShell(t)
		sh.filesSearch.open = true
		sh.filesSearch.mode = filesSearchText
		tt := newTester(w, 1280, 800, dark)
		settle(tt, pump, 3)
		for _, c := range []struct {
			group, chosen, idle string
		}{{"Show", "All", "Changed"}, {"Search in", "Content", "Files"}} {
			on, ok := tt.AccessByRole(ui.RoleRadio, c.chosen)
			if !ok || !on.Checked {
				t.Fatalf("%s: %q is not the chosen segment (found %v, %+v)", c.group, c.chosen, ok, on)
			}
			assertWash(t, tt, fmt.Sprintf("dark=%v, %s: %s", dark, c.group, c.chosen), on.Bounds, dark, true)
			if c.idle == "Files" {
				continue // the view tab of the same name answers to it first
			}
			off, ok := tt.AccessByRole(ui.RoleRadio, c.idle)
			if !ok {
				t.Fatalf("%s: no %q segment", c.group, c.idle)
			}
			assertWash(t, tt, fmt.Sprintf("dark=%v, %s: %s", dark, c.group, c.idle), off.Bounds, dark, false)
		}
		saveEvidence(t, fmt.Sprintf("sidebar-switches-%s", map[bool]string{false: "light", true: "dark"}[dark]), tt)
	}
}

// An idle tab is a button like any other: a click chooses its view, the arrows
// walk the strip once it has the focus, and the pointer over it paints the same
// faint wash a hovered sidebar row does (and says its name, count and shortcut
// in its tooltip, since the glyph alone does not).
func TestViewTabsChooseByClickArrowsAndHover(t *testing.T) {
	root := gitFixture(t, "click")
	for _, dark := range []bool{false, true} {
		w, pump := newTestWindow(t)
		sh := openAndLoad(t, w, pump, root)
		tt := newTester(w, 1280, 800, dark)
		settle(tt, pump, 3)

		// Hover: an idle tab is washed under the pointer, and says what it is.
		rects, _ := stripRects(t, tt)
		files := rects[2]
		tt.Move(files.X+files.W/2, files.Y+files.H/2)
		tt.Advance(time.Second)
		tt.Frame()
		img := tt.Image()
		sidebar, here := luminance(img, 3, files.Y+files.H/2), luminance(img, files.X+3, files.Y+files.H/2)
		if dark && here <= sidebar || !dark && here >= sidebar {
			t.Errorf("dark=%v: the hovered idle tab (%d) is not a wash of ink on the sidebar (%d)", dark, here, sidebar)
		}
		// The tooltip is the last thing the frame lists: name, then the keycaps.
		if texts := tt.Texts(); len(texts) < 3 || !slices.Equal(texts[len(texts)-3:], []string{"Files", "⌘", "3"}) {
			t.Errorf("dark=%v: the Files tab's tooltip does not spell its shortcut ⌘3: %q", dark, texts)
		}
		tt.Move(900, 700)
		settle(tt, pump, 2)
		changes := rects[0]
		tt.Move(changes.X+changes.W/2, changes.Y+changes.H/2)
		tt.Advance(time.Second)
		tt.Frame()
		if !tt.HasText("Changes · 2 files") {
			t.Errorf("dark=%v: the Changes tab's tooltip does not carry the count: %q", dark, tt.Texts())
		}
		tt.Move(900, 700)
		settle(tt, pump, 2)

		// Click: the idle Commits tab chooses its view; the arrows then walk on.
		tt.ClickAt(rects[1].X+rects[1].W/2, rects[1].Y+rects[1].H/2)
		settle(tt, pump, 3)
		if got := sh.currentView(); got != viewCommits {
			t.Fatalf("dark=%v: a click on the Commits tab left the view at %v", dark, got)
		}
		// ← and → walk the strip while a tab holds the focus (Files hands it to
		// its tree on entry, as it did before the tabs had glyphs).
		for _, step := range []struct {
			key  ui.Key
			want viewKind
		}{{ui.KeyLeft, viewChanges}, {ui.KeyRight, viewCommits}, {ui.KeyRight, viewFiles}} {
			tt.Key(0, step.key)
			settle(tt, pump, 3)
			if got := sh.currentView(); got != step.want {
				t.Fatalf("dark=%v: an arrow chose %v, want %v", dark, got, step.want)
			}
		}
	}
}
