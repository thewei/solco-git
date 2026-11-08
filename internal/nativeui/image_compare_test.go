package nativeui

/**
 * [INPUT]: image_compare.go、纯色位图、真实临时仓库里一张被改的 PNG、headless ui.Tester
 * [OUTPUT]: 图片对比的合同：两张图共用画布与缩放、Swipe 分割线两侧各是哪一张、Onion 的混合随滑块变化、改过的图片在 Changes 里出现 2-up/Swipe/Onion skin 切换并显示两侧尺寸
 * [POS]: 2026-11-07「Solco Git」决定里图片对比（Fork 的 image compare）的回归防线；位图在 Tester 里真的被光栅化，所以像素可以直接取样
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestCompareGeometrySharesOneCanvas(t *testing.T) {
	cases := []struct {
		bw, bh, aw, ah int
		room, maxH     float32
		cw, ch, scale  float32
	}{
		{40, 30, 40, 30, 200, 360, 40, 30, 1},         // fits: drawn at its own size
		{40, 30, 80, 60, 200, 360, 80, 60, 1},         // the larger of the two
		{40, 30, 20, 60, 200, 360, 40, 60, 1},         // the union of the two
		{400, 300, 400, 300, 200, 360, 200, 150, 0.5}, // too wide: scaled down
		{100, 900, 100, 900, 500, 360, 40, 360, 0.4},  // too tall: scaled down
		{0, 0, 0, 0, 200, 360, 0, 0, 1},               // nothing to draw
	}
	for _, c := range cases {
		cw, ch, scale := compareGeometry(c.bw, c.bh, c.aw, c.ah, c.room, c.maxH)
		if cw != c.cw || ch != c.ch || scale != c.scale {
			t.Errorf("compareGeometry(%d×%d, %d×%d, %v, %v) = %v×%v @%v, want %v×%v @%v",
				c.bw, c.bh, c.aw, c.ah, c.room, c.maxH, cw, ch, scale, c.cw, c.ch, c.scale)
		}
	}
}

// canvasPixels renders the compare canvas at (20,20) of a 120×100 Tester and
// returns the frame.
func canvasPixels(t *testing.T, mode compareMode, split, onion float64) *image.RGBA {
	t.Helper()
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	before := &gdMediaSide{present: true, w: 80, h: 60, bmp: ui.NewBitmap(solid(80, 60, red))}
	after := &gdMediaSide{present: true, w: 80, h: 60, bmp: ui.NewBitmap(solid(80, 60, blue))}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(20).Children(func() {
			ui.Box(c).Size(80, 60).Draw(func(p *ui.Painter, r ui.Rect) {
				drawImageCompare(p, r, mode, before, after, 1, split, onion, ui.RGBA(0, 255, 0, 1))
			})
		})
	}, 120, 100)
	tt.Frame()
	return tt.Image()
}

func near(a, b uint8, tol int) bool {
	d := int(a) - int(b)
	return d >= -tol && d <= tol
}

func TestSwipeShowsBeforeLeftAndAfterRight(t *testing.T) {
	img := canvasPixels(t, compareSwipe, 0.25, 0.5)
	left, right := img.RGBAAt(20+8, 20+10), img.RGBAAt(20+70, 20+10)
	if left.R < 250 || left.B > 5 {
		t.Errorf("left of the divider = %v, want Before (red)", left)
	}
	if right.B < 250 || right.R > 5 {
		t.Errorf("right of the divider = %v, want After (blue)", right)
	}
	if line := img.RGBAAt(20+20, 20+10); line.G < 200 {
		t.Errorf("divider at 25%% = %v, want the accent (green)", line)
	}
	// At 0 the whole picture is After; at 1 the whole picture is Before.
	if all := canvasPixels(t, compareSwipe, 0, 0).RGBAAt(20+40, 20+10); all.B < 250 {
		t.Errorf("divider at 0: %v, want After", all)
	}
	if all := canvasPixels(t, compareSwipe, 1, 0).RGBAAt(20+40, 20+10); all.R < 250 {
		t.Errorf("divider at 1: %v, want Before", all)
	}
}

// unequalSwipe renders the Swipe canvas of a Before bw wide and an After aw
// wide (both 30 tall, drawn at scale 1 on the canvas of the wider one) at
// (20,20) of a Tester whose background shows at (5,5).
func unequalSwipe(t *testing.T, bw, aw int, split float64) *image.RGBA {
	t.Helper()
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	before := &gdMediaSide{present: true, w: bw, h: 30, bmp: ui.NewBitmap(solid(bw, 30, red))}
	after := &gdMediaSide{present: true, w: aw, h: 30, bmp: ui.NewBitmap(solid(aw, 30, blue))}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(20).Children(func() {
			ui.Box(c).Size(float32(max(bw, aw)), 30).Draw(func(p *ui.Painter, r ui.Rect) {
				drawImageCompare(p, r, compareSwipe, before, after, 1, split, 0, ui.RGBA(0, 255, 0, 1))
			})
		})
	}, 120, 70)
	tt.Frame()
	return tt.Image()
}

// A side shows its own picture and nothing of the other: where Before is
// narrower than After, the strip between Before's edge and the divider is
// Before's empty space, not After's pixels showing through, and at 1 nothing
// of After shows at all (2026-11-08: the swipe of two pictures of different
// widths showed After's extra width on Before's side).
func TestSwipeSidesShowOnlyTheirOwnPicture(t *testing.T) {
	// Before 40 wide, After 60 wide: the canvas is 60, the divider at 0.75 stands at 45.
	// Rows near the top: the divider's handle sits at mid height.
	img := unequalSwipe(t, 40, 60, 0.75)
	bg := img.RGBAAt(5, 5)
	if px := img.RGBAAt(20+42, 20+3); px != bg {
		t.Errorf("between Before's edge (40) and the divider (45) = %v, want the empty background %v", px, bg)
	}
	if px := img.RGBAAt(20+10, 20+3); px.R < 250 || px.B > 5 {
		t.Errorf("Before's own pixels = %v, want red", px)
	}
	if px := img.RGBAAt(20+55, 20+3); px.B < 250 || px.R > 5 {
		t.Errorf("right of the divider = %v, want After (blue)", px)
	}
	if px := unequalSwipe(t, 40, 60, 1).RGBAAt(20+50, 20+3); px != bg {
		t.Errorf("divider at 1, beyond Before = %v, want the empty background %v: no After shows", px, bg)
	}
	// Mirror: After narrower than Before. Past After's edge the right side is empty.
	img = unequalSwipe(t, 60, 40, 0.25)
	bg = img.RGBAAt(5, 5)
	if px := img.RGBAAt(20+50, 20+3); px != bg {
		t.Errorf("right of After's edge (40) = %v, want the empty background %v: no Before shows", px, bg)
	}
	if px := img.RGBAAt(20+30, 20+3); px.B < 250 || px.R > 5 {
		t.Errorf("After's own pixels = %v, want blue", px)
	}
	if px := unequalSwipe(t, 60, 40, 0).RGBAAt(20+50, 20+3); px != bg {
		t.Errorf("divider at 0, beyond After = %v, want the empty background %v: no Before shows", px, bg)
	}
}

func TestOnionBlendsWithTheSlider(t *testing.T) {
	cases := []struct {
		alpha float64
		r, b  uint8
	}{{0, 255, 0}, {0.5, 128, 128}, {1, 0, 255}}
	for _, c := range cases {
		px := canvasPixels(t, compareOnion, 0.5, c.alpha).RGBAAt(20+40, 20+30)
		if !near(px.R, c.r, 6) || !near(px.B, c.b, 6) || px.G > 5 {
			t.Errorf("onion at %.1f = %v, want ≈ (%d,0,%d)", c.alpha, px, c.r, c.b)
		}
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// pictureFixture is a repository whose logo.png was red 40×30 and is now blue
// 60×30 in the working tree.
func pictureFixture(t *testing.T) (*Window, *uiPump, *Shell) {
	t.Helper()
	root := gitFixture(t, "pics")
	gitRun(t, root, "stash", "-u", "-q")
	writePNG(t, filepath.Join(root, "logo.png"), solid(40, 30, color.RGBA{220, 40, 40, 255}))
	gitRun(t, root, "add", "logo.png")
	gitRun(t, root, "commit", "-q", "-m", "Add logo")
	writePNG(t, filepath.Join(root, "logo.png"), solid(60, 30, color.RGBA{40, 60, 220, 255}))
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	return w, pump, sh
}

func TestChangedPictureOffersTheCompareModes(t *testing.T) {
	w, pump, sh := pictureFixture(t)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 4)
	pump.until(t, "both pictures", func() bool {
		settle(tt, pump, 1)
		return tt.HasText("2-up")
	})
	for _, want := range []string{"2-up", "Swipe", "Onion skin"} {
		if !tt.HasText(want) {
			t.Fatalf("compare switch lacks %q; texts: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Swipe"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	if sh.imageCompare.mode != compareSwipe {
		t.Fatalf("mode = %v, want Swipe", sh.imageCompare.mode)
	}
	for _, want := range []string{"Before 40 × 30", "After 60 × 30", "Swipe position"} {
		if !tt.HasText(want) {
			t.Errorf("Swipe view lacks %q; texts: %q", want, tt.Texts())
		}
	}
	saveEvidence(t, "image-swipe", tt)
	if err := tt.Click("Onion skin"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	if sh.imageCompare.mode != compareOnion || !tt.HasText("Onion skin opacity") {
		t.Errorf("Onion view missing; mode %v texts: %q", sh.imageCompare.mode, tt.Texts())
	}
	saveEvidence(t, "image-onion", tt)
	if err := tt.Click("2-up"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	if sh.imageCompare.mode != compareTwoUp || tt.HasText("Onion skin opacity") {
		t.Error("2-up is the side-by-side layout again")
	}
	saveEvidence(t, "image-2up", tt)
}
