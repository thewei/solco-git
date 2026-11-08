package nativeui

/**
 * [INPUT]: frame.go（viewTabs / viewTabBases / viewTabNeed）、ds_components.go（materialFace 与 materialTab*、materialSegment*）、git_sidebar_tree.go（segmentTabs）、files_git.go（filesScopeTabs）、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 侧栏视图页签的合同：三个页签都有图形，只有选中的显示名称（其余只有图形，名字在提示与无障碍名里），选中的吃掉剩余宽度，Changes 的计数在任何宽度下都完整且空间不够时只丢"闲置计数"；页签在 vibrancy 材质上不画纸（选中是侧栏选中色的一层淡墨，不是白底/不透明槽），侧栏里另外两个两段开关（Show / Search in）同样
 * [POS]: 2026-11-08「Solco Git 侧栏页签：选中显示图标加名称、其余只有图标，不再是一个白底的分段控件」决定（用户："现在白底的话或者颜色直接显示出来，有点难以融入我们现在侧栏的半透明的效果"）的回归防线
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

// stripRects reads the three view tabs' boxes off the accessibility tree: the
// tabs are the segments of the "View" radio group, named by their view.
func stripRects(t *testing.T, tt *ui.Tester) (rects [len(sidebarViews)]ui.Rect, checked int) {
	t.Helper()
	checked = -1
	for i, v := range sidebarViews {
		a, ok := tt.AccessByRole(ui.RoleRadio, v.label)
		if !ok {
			t.Fatalf("no %q tab in the accessibility tree", v.label)
		}
		if !a.Segment {
			t.Errorf("tab %q is not a segment of the View group", v.label)
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
	start, end := slices.Index(all, "View"), slices.Index(all, repo)
	if start < 0 || end < start {
		t.Fatalf("no tab strip before %q in %q", repo, all)
	}
	return all[start+1 : end]
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

// Every tab has a glyph; only the chosen one says its name and it takes the
// room the others leave. The strip stays inside the sidebar's gutters, tabs in
// order and apart, at the narrowest, the default and the widest sidebar.
func TestViewTabsNameOnlyTheChosenView(t *testing.T) {
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
				t.Fatalf("width %.0f, %s: the chosen tab is %d, want %d", width, view.label, got, want)
			}
			texts := stripTexts(t, tt, "tabs")
			y, h := rects[0].Y, rects[0].H
			if h != materialTabHeight {
				t.Errorf("width %.0f, %s: the strip is %.1f high, want %d", width, view.label, h, materialTabHeight)
			}
			for i, r := range rects {
				if r.Y != y || r.H != h {
					t.Errorf("width %.0f, %s: tab %d is %v, not on the strip's row", width, view.label, i, r)
				}
				if i > 0 && r.X < rects[i-1].X+rects[i-1].W {
					t.Errorf("width %.0f, %s: tab %d %v overlaps tab %d %v", width, view.label, i, r, i-1, rects[i-1])
				}
				if i != want && r.W >= rects[want].W {
					t.Errorf("width %.0f, %s: idle tab %d (%.1f) is not narrower than the chosen one (%.1f)", width, view.label, i, r.W, rects[want].W)
				}
				if named := occurrences(texts, sidebarViews[i].label) == 2; named != (i == want) {
					t.Errorf("width %.0f, %s: tab %d name drawn = %v, want %v (strip: %q)", width, view.label, i, named, i == want, texts)
				}
			}
			first, last := rects[0], rects[len(rects)-1]
			if first.X < sidebarGutter-0.5 || last.X+last.W > width-sidebarGutter+0.5 {
				t.Errorf("width %.0f, %s: the strip %.1f..%.1f leaves the %d-point gutters", width, view.label, first.X, last.X+last.W, int(sidebarGutter))
			}
			// The fixture has two changed files: the count shows in the strip
			// whichever tab is chosen.
			if occurrences(texts, "2") != 1 {
				t.Errorf("width %.0f, %s: the Changes count is not in the strip once (strip: %q)", width, view.label, texts)
			}
		}
	}
}

// With the least room and a count too long for it, the idle Changes tab gives
// up its count (the tooltip still says it) rather than squeezing the chosen
// tab; with room it keeps it, and the chosen tab never loses a letter.
func TestViewTabBasesDropTheIdleCountOnlyWhenTheRowIsFull(t *testing.T) {
	cases := []struct {
		width   float32
		chosen  int
		changes int
		idle    bool
	}{
		{sidebarWidth, 1, 330, true},
		{sidebarWidth, 2, 12, true},
		{sidebarWidth, 1, 0, true}, // nothing to count: nothing to drop
		{sidebarMinWidth, 1, 12, true},
		{sidebarMinWidth, 0, 12, true},
		{sidebarMinWidth, 1, 99999, false},
		{sidebarMinWidth, 2, 99999, true}, // a short name leaves the room back
	}
	for _, tc := range cases {
		sh := &Shell{sidebarW: tc.width}
		var bases [len(sidebarViews)]float32
		var idle bool
		var need float32
		probe := ui.NewTester(func(c *ui.Context) {
			bases, idle = sh.viewTabBases(c, tc.chosen, tc.changes)
			count := 0
			if sidebarViews[tc.chosen].kind == viewChanges {
				count = tc.changes
			}
			need = viewTabNeed(c, sidebarViews[tc.chosen].label, count, true)
		}, 100, 100)
		probe.Frame()
		name := fmt.Sprintf("width %.0f, tab %d chosen, %d changes", tc.width, tc.chosen, tc.changes)
		if idle != tc.idle {
			t.Errorf("%s: idle count = %v, want %v", name, idle, tc.idle)
		}
		if bases[tc.chosen] != need {
			t.Errorf("%s: the chosen tab starts from %.1f, want its own need %.1f", name, bases[tc.chosen], need)
		}
		for i, b := range bases {
			if i != tc.chosen && sidebarViews[i].kind != viewChanges && b != materialTabIdleW {
				t.Errorf("%s: idle tab %d starts from %.1f, want %d", name, i, b, materialTabIdleW)
			}
		}
		if !idle && sidebarViews[0].kind == viewChanges && tc.chosen != 0 && bases[0] != materialTabIdleW {
			t.Errorf("%s: the idle Changes tab keeps %.1f without its count, want %d", name, bases[0], materialTabIdleW)
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

// The view tabs sit on the sidebar's vibrancy: the chosen tab is a wash of ink
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
			// Nothing is drawn behind the strip either: no track.
			img := tt.Image()
			y := rects[0].Y + rects[0].H/2
			for _, gap := range []float32{sidebarGutter / 2, rects[len(rects)-1].X + rects[len(rects)-1].W + sidebarGutter/2} {
				if d := luminance(img, gap, y) - luminance(img, 1, y); d < -2 || d > 2 {
					t.Errorf("dark=%v, %s chosen: the strip has a track (%d at x=%.0f)", dark, view.label, d, gap)
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
