package nativeui

/**
 * [INPUT]: window_workspaces.go（workspaceSwitcher / workspaceChip）、frame.go（sidebarFoot / fetchTip）、window_view.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 侧栏页脚的合同：工作区芯片在侧栏最下方（与 Settings 钮同一行，Solco 页脚芯片的样子）、点开向上弹出；侧栏收起时芯片才出现在标题带里（工作区永远够得着）、展开后带里不再有；页脚不再有 "Fetched … ago"，那句话进了 Fetch 钮的提示；开着下拉时切换侧栏会把它收起
 * [POS]: 2026-11-08「Solco Git 侧栏：工作区切换放到最下方」决定（用户："把工作区的切换放到侧栏最下方，在我们原来 Solco 最底部切换工作区的地方，代替掉…使用跟切换工作区一样的样式"）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// The workspace chip is the foot's: at the bottom of the sidebar, in a row
// with the Settings button, and the band carries no second one.
func TestTheWorkspaceChipSitsInTheSidebarFoot(t *testing.T) {
	w, pump, tt := openThree(t)
	chip, ok := tt.Find("Workspace: gamma")
	if !ok {
		t.Fatalf("no workspace chip: %q", tt.Texts())
	}
	gear, okGear := tt.Find("Settings")
	if !okGear {
		t.Fatalf("no Settings button: %q", tt.Texts())
	}
	if chip.Y < 800-60 || chip.X+chip.W > w.sidebarW {
		t.Errorf("the chip %v is not in the sidebar's foot (sidebar %.0f wide, window 800 high)", chip, w.sidebarW)
	}
	if math.Abs(float64((chip.Y+chip.H/2)-(gear.Y+gear.H/2))) > 1 || gear.X < chip.X+chip.W {
		t.Errorf("the chip %v and Settings %v are not one row, chip first", chip, gear)
	}
	if tt.HasText("Fetched") || tt.HasText("Not fetched yet") {
		t.Errorf("the foot still carries the fetched label; texts: %q", tt.Texts())
	}

	// Its pull-down opens upward, above the chip.
	if err := tt.Click("Workspace: gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	field, ok := tt.Find("Search workspaces")
	if !ok {
		t.Fatalf("the pull-down did not open; texts: %q", tt.Texts())
	}
	if field.Y+field.H > chip.Y {
		t.Errorf("the pull-down's search field %v is not above the foot chip %v", field, chip)
	}
	saveEvidence(t, "foot-chip-open", tt)
}

// While the sidebar is shut the foot is gone, so the band carries the chip;
// opening the sidebar again takes it back, and a pull-down left open does not
// follow the chip from one place to the other.
func TestTheBandCarriesTheWorkspaceChipOnlyWhileTheSidebarIsShut(t *testing.T) {
	w, pump, tt := openThree(t)
	if err := tt.Click("Workspace: gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	if !w.picker.open {
		t.Fatal("the pull-down did not open")
	}

	w.toggleSidebar()
	settle(tt, pump, 4)
	if w.picker.open {
		t.Error("the pull-down stayed open and would reappear on the band's chip")
	}
	chip, ok := tt.Find("Workspace: gamma")
	if !ok || chip.Y > 60 {
		t.Fatalf("with the sidebar shut the chip should be on the band (top): %v %v", chip, ok)
	}
	// It is the same control: it opens the same list and switches.
	if err := tt.Click("Workspace: gamma"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	if err := tt.Click("alpha"); err != nil {
		t.Fatalf("choosing alpha from the band's pull-down: %v", err)
	}
	settle(tt, pump, 3)
	if w.active != 0 {
		t.Errorf("the band's pull-down did not switch: active=%d", w.active)
	}

	w.toggleSidebar()
	settle(tt, pump, 4)
	chip, ok = tt.Find("Workspace: alpha")
	if !ok || chip.Y < 800-60 {
		t.Errorf("with the sidebar open again the chip should be in the foot: %v %v", chip, ok)
	}
}

// The words the foot used to carry are the Fetch button's tooltip now: what
// it does and, where there is a remote, when it last ran.
func TestFetchTipCarriesTheFetchedLabel(t *testing.T) {
	w, pump, tt := openThree(t)
	sh := w.activeShell()
	settle(tt, pump, 2)
	if got := sh.fetchTip(); got != "Fetch from the remote" {
		t.Errorf("a repository with no remote says %q", got)
	}
	sh.git.remotes = []gitworkbench.Remote{{Name: "origin"}}
	sh.git.fetchedOK = false
	if got := sh.fetchTip(); got != "Fetch from the remote — Not fetched yet" {
		t.Errorf("never fetched says %q", got)
	}
	sh.git.fetchedOK, sh.git.fetchedAt = true, timeNow().Add(-5*time.Minute)
	if got := sh.fetchTip(); !strings.HasSuffix(got, "Fetched 5 min ago") {
		t.Errorf("fetched five minutes ago says %q", got)
	}
}

// While the sidebar slides, its foot is still on screen and the band already
// carries the chip: two chips in one frame. Only one may own the pull-down —
// two popovers on one handle crash the build ("Handle bound to two controls").
func TestOnlyOneWorkspaceChipOwnsThePullDownWhileTheSidebarSlides(t *testing.T) {
	for _, collapse := range []bool{true, false} {
		w, pump, tt := openThree(t)
		if !collapse { // start shut, so the first frame of opening has both chips too
			w.toggleSidebar()
			settle(tt, pump, 4)
		}
		tt.SetPreferences(ui.Preferences{}) // real motion: the clock stands still, so the slide holds its first frame
		w.toggleSidebar()
		tt.Frame()
		w.picker.open = true // what a click on either chip does
		tt.Frame()
		tt.Frame()
		if !w.picker.open {
			t.Errorf("collapse=%v: the pull-down closed itself", collapse)
		}
		tt.Advance(time.Second)
		tt.Frame()
		if _, ok := tt.Find("Search workspaces"); !ok && w.picker.open {
			t.Errorf("collapse=%v: the pull-down is open but not drawn once the slide is over", collapse)
		}
	}
}
