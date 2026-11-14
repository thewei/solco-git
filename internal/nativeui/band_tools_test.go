package nativeui

/**
 * [INPUT]: window_view.go / frame.go / window_nav.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 2026-11-14「搜索和刷新搬进标题带」的合同：侧栏顶上不再有仓库名/搜索/刷新这一行（仓库名只在页脚工作区芯片里）；搜索与刷新是侧栏开关右边的两颗按钮，Back/Forward 仍在侧栏右缘；侧栏收起时点搜索先展开侧栏再开输入框；侧栏最窄也容得下带里全部按钮
 * [POS]: 用户："侧栏需要显示对应的搜索、刷新和项目名称。我希望把这些内容去掉，然后把搜索和刷新放到上面，放在展开/收缩按钮旁边"
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSearchAndRefreshSitBesideTheSidebarToggle(t *testing.T) {
	w, pump, tt := openThree(t)
	toggle, _ := tt.Find("Hide Sidebar")
	search, okS := tt.Find("Search references")
	refresh, okR := tt.Find("Refresh repository")
	back, _ := tt.Find("Back")
	if !okS || !okR {
		t.Fatalf("search and Refresh are not in the band: %q", tt.Texts())
	}
	if search.Y != toggle.Y || refresh.Y != toggle.Y {
		t.Errorf("the buttons are not on the toggle's line: toggle %v search %v refresh %v", toggle, search, refresh)
	}
	if !(toggle.X < search.X && search.X < refresh.X && refresh.X+refresh.W <= back.X) {
		t.Errorf("order must be toggle, search, Refresh, then Back: %v %v %v %v", toggle, search, refresh, back)
	}
	// The repository's name is said by the foot's chip, after the sidebar's own
	// content, and nowhere above it.
	texts := tt.Texts()
	for _, text := range texts[:slices.Index(texts, "Workspace: gamma")] {
		if text == "gamma" {
			t.Fatalf("the repository's name is still a row of the sidebar: %q", texts)
		}
	}

	// With the sidebar shut, search opens it first: its field lives there.
	w.toggleSidebar()
	settle(tt, pump, 20)
	if err := tt.Click("Search references"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 20)
	if w.sidebarCollapsed {
		t.Errorf("search did not open the sidebar")
	}
	if !w.activeShell().gitRefsSearchOpen {
		t.Errorf("search did not open its field")
	}
	if _, ok := tt.Find("Close search references"); !ok {
		t.Errorf("the search button does not say it can close; texts: %q", tt.Texts())
	}
}

// The band holds the traffic lights, the toggle with search and Refresh, and
// Back / Forward before the resize handle, at the narrowest sidebar too.
func TestTheNarrowestSidebarHoldsTheBandsButtons(t *testing.T) {
	bar := ui.TitleBar{Height: 38, Left: 68}
	min, max := sidebarLimits(1280, bar)
	need := sidebarToggleX(bar) + sidebarLeftGroupWidth() + sidebarNavGap + sidebarRightGroupWidth() + 12
	if min < need {
		t.Errorf("minimum sidebar %.0f is narrower than the band's buttons need (%.0f)", min, need)
	}
	if max < min {
		t.Errorf("maximum %.0f is under the minimum %.0f", max, min)
	}
	if bare, _ := sidebarLimits(1280, ui.TitleBar{}); bare != sidebarMinWidth {
		t.Errorf("without window controls the floor is the plain %.0f, got %.0f", sidebarMinWidth, bare)
	}
}
