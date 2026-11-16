package nativeui

/**
 * [INPUT]: sidebar_tree.go 的 sidebarTreeRow / sidebarSectionHeader（侧栏引用列表的行与分组头）、list_nav.go 的键盘光标、真实的临时仓库、headless ui.Tester 的像素
 * [OUTPUT]: 2026-11-16 的合同：键盘把焦点交给侧栏列表时，光标所在的行（分组头与普通行）画一圈蓝色焦点环，而不只是一层和「已选中」一样的灰底；鼠标点过的行不画环
 * [POS]: 实机巡检发现：Tab 进入侧栏后，焦点行与选中行（master）的底色几乎相同，分不出「焦点在哪」；按钮早就有蓝环，列表行补上
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// ringPixels counts the saturated blue of a focus ring inside the sidebar
// column: the sidebar holds no other blue at rest.
func ringPixels(img *image.RGBA) int {
	n := 0
	for y := 90; y < 700; y++ {
		for x := 0; x < 230; x++ {
			c := img.RGBAAt(x, y)
			if isRingBlue(c) {
				n++
			}
		}
	}
	return n
}

func isRingBlue(c color.RGBA) bool {
	return int(c.B) > 150 && int(c.B) > int(c.R)+60 && int(c.B) > int(c.G)+30
}

func focusShell(t *testing.T) (*Window, *uiPump, *Shell) {
	t.Helper()
	root := gitFixture(t, "ring")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the references", func() bool { return sh.git.refsRead })
	return w, pump, sh
}

func TestKeyboardFocusRingsASidebarHeader(t *testing.T) {
	w, pump, _ := focusShell(t)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 3)
	before := ringPixels(tt.Image())
	after := before
	// The View rows (Changes / Commits / Files) are the first stop; the
	// references list, whose first row is a header, is the next.
	for i := 0; i < 4 && after < before+60; i++ {
		tt.Key(0, ui.KeyTab)
		settle(tt, pump, 3)
		after = ringPixels(tt.Image())
	}
	if after < before+60 {
		t.Errorf("Tab through the sidebar drew no focus ring on a header (blue pixels %d -> %d)", before, after)
	}
}

func TestKeyboardFocusRingsASidebarRow(t *testing.T) {
	w, pump, _ := focusShell(t)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 3)
	// Tab until the ring shows (on the references list's first header), then
	// walk down onto a branch row.
	for i := 0; i < 4 && ringPixels(tt.Image()) < 60; i++ {
		tt.Key(0, ui.KeyTab)
		settle(tt, pump, 3)
	}
	headerRing := ringPixels(tt.Image())
	tt.Key(0, ui.KeyDown) // Branches header
	tt.Key(0, ui.KeyDown) // the first branch row
	settle(tt, pump, 3)
	if got := ringPixels(tt.Image()); got < 60 {
		t.Errorf("the cursor on a branch row has no focus ring (blue pixels %d; the header had %d)", got, headerRing)
	}
}

func TestAMouseClickedRowWearsNoRing(t *testing.T) {
	w, pump, _ := focusShell(t)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 3)
	if err := tt.Click("Branches"); err != nil {
		t.Skipf("no Branches header to click: %v", err)
	}
	settle(tt, pump, 3)
	if got := ringPixels(tt.Image()); got > 20 {
		t.Errorf("a mouse click left a keyboard focus ring behind (blue pixels %d)", got)
	}
}
