package nativeui

/**
 * [INPUT]: 依赖 gd_media.go 的两侧图片（gdMediaSide.bmp/w/h/size）、gdPalette、MyGo ui.Painter（Image/ImageRegion/Clip/Fill）与 ui.Slider
 * [OUTPUT]: 对外提供 compareMode（2-up / Swipe / Onion skin）、imageCompareState、compareGeometry（两张图共用的画布与缩放）、drawImageCompare（Swipe：分割线左边是 Before 右边是 After；Onion：After 以滑块透明度叠在 Before 上）、imageCompareView（切换条 + 画布 + 滑块 + 两侧尺寸行）
 * [POS]: Fork 的 image compare：Changes/提交里一张图片改了，两侧都读得出图时，除了并排的 2-up 还能滑动对比、叠影对比；全部在已解码的位图上绘制，不碰 Git、不写任何东西
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/filesview"
)

// compareMode is how a changed picture's two sides are shown.
type compareMode uint8

const (
	compareTwoUp compareMode = iota // side by side
	compareSwipe                    // one picture, a divider: Before on its left, After on its right
	compareOnion                    // After laid over Before at the slider's opacity
)

// imageCompareState is the card's choice. It is the Shell's: the mode and the
// sliders carry from one picture to the next.
type imageCompareState struct {
	mode  compareMode
	ready bool
	split float64 // Swipe: where the divider stands, 0 (all After) to 1 (all Before)
	onion float64 // Onion skin: After's opacity, 0 (Before) to 1 (After)
}

// gdCompareMaxH is the tallest the Swipe and Onion canvas is drawn.
const gdCompareMaxH = 360

// compareGeometry is the canvas the two pictures share: as large as the larger
// of them, scaled down (never up) to fit room and maxH. Both are drawn at that
// one scale from the top-left, so a picture that grew or shrank shows it.
func compareGeometry(bw, bh, aw, ah int, room, maxH float32) (cw, ch, scale float32) {
	w, h := float32(max(bw, aw)), float32(max(bh, ah))
	if w <= 0 || h <= 0 {
		return 0, 0, 1
	}
	scale = min(1, room/w, maxH/h)
	if scale <= 0 {
		return 0, 0, 1
	}
	return w * scale, h * scale, scale
}

// drawImageCompare paints the canvas of the Swipe and Onion modes into r,
// whose size is the canvas'. ui.Painter draws what it is given: a missing
// bitmap leaves its side empty.
func drawImageCompare(p *ui.Painter, r ui.Rect, mode compareMode, before, after *gdMediaSide, scale float32, split, onion float64, accent ui.Color) {
	place := func(side *gdMediaSide) ui.Rect {
		return ui.Rect{X: r.X, Y: r.Y, W: float32(side.w) * scale, H: float32(side.h) * scale}
	}
	switch mode {
	case compareSwipe:
		// Each side shows its own picture and nothing of the other: where
		// the two differ in size, the part of the canvas a picture does not
		// cover is empty on its side of the divider, not the other picture
		// showing through (2026-11-08).
		x := r.X + r.W*float32(clamp01(split))
		if after.bmp != nil {
			p.Clip(ui.Rect{X: x, Y: r.Y, W: r.X + r.W - x, H: r.H}, 0, func() {
				p.ImageRegion(after.bmp, ui.Rect{}, place(after), ui.ImageOptions{})
			})
		}
		if before.bmp != nil {
			p.Clip(ui.Rect{X: r.X, Y: r.Y, W: x - r.X, H: r.H}, 0, func() {
				p.ImageRegion(before.bmp, ui.Rect{}, place(before), ui.ImageOptions{})
			})
		}
		p.Fill(ui.Rect{X: x - 1, Y: r.Y, W: 2, H: r.H}, accent, 0)
		p.Fill(ui.Rect{X: x - 7, Y: r.Y + r.H/2 - 7, W: 14, H: 14}, accent, 7)
	case compareOnion:
		if before.bmp != nil {
			p.ImageRegion(before.bmp, ui.Rect{}, place(before), ui.ImageOptions{})
		}
		if after.bmp != nil {
			// A white tint of the slider's alpha leaves the colors alone and
			// scales the opacity.
			p.ImageRegion(after.bmp, ui.Rect{}, place(after), ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, float32(clamp01(onion)))})
		}
	}
}

func clamp01(v float64) float64 { return max(0, min(1, v)) }

// imageCompareView is the card of a picture both of whose sides could be read:
// the 2-up / Swipe / Onion skin switch, then for the last two the shared
// canvas, its slider and each side's size line. twoUp draws the side-by-side
// layout the card always had.
func (s *Shell) imageCompareView(c *ui.Context, pal *gdPalette, m *gdMedia, room float32, twoUp func()) {
	st := &s.imageCompare
	if !st.ready {
		st.ready, st.split, st.onion = true, 0.5, 0.5
	}
	t := c.Theme()
	typ := Typography()
	ui.Column(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		choice := int(st.mode)
		if segmentedControl(c, &choice, "2-up", "Swipe", "Onion skin").Label("Compare pictures").Changed() {
			st.mode = compareMode(choice)
		}
		if st.mode == compareTwoUp {
			twoUp()
			return
		}
		cw, ch, scale := compareGeometry(m.before.w, m.before.h, m.after.w, m.after.h, room, gdCompareMaxH)
		canvas := ui.Box(c).Size(cw, ch).Border(1, pal.cardBorder).Radius(Radius().Control).Background(pal.emptySide)
		accent := t.Accent
		mode, before, after, split, onion := st.mode, &m.before, &m.after, st.split, st.onion
		canvas.Draw(func(p *ui.Painter, r ui.Rect) {
			drawImageCompare(p, r, mode, before, after, scale, split, onion, accent)
		})
		ui.Row(c).Width(max(cw, 220)).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Before").FontSize(typ.BodySmall).FontWeight(WeightSemibold).TextColor(pal.delText).Shrink(0)
			if st.mode == compareSwipe {
				ui.Slider(c.Key("compare-split"), &st.split, 0, 1).Label("Swipe position").Grow(1).MinWidth(0)
			} else {
				ui.Slider(c.Key("compare-onion"), &st.onion, 0, 1).Label("Onion skin opacity").Grow(1).MinWidth(0)
			}
			ui.Text(c, "After").FontSize(typ.BodySmall).FontWeight(WeightSemibold).TextColor(pal.addText).Shrink(0)
		})
		ui.Row(c).FillWidth().Gap(16).Children(func() {
			for _, side := range []struct {
				label string
				s     *gdMediaSide
				color ui.Color
			}{{"Before", &m.before, pal.delText}, {"After", &m.after, pal.addText}} {
				ui.Text(c, side.label+" "+compareCaption(side.s)).FontSize(typ.BodySmall).TextColor(side.color).SingleLine().
					MinWidth(0)
			}
		})
	})
}

// compareCaption is "W × H · size", the line under a side of the canvas.
func compareCaption(side *gdMediaSide) string {
	caption := filesview.FormatFileSize(side.size)
	if side.w > 0 {
		caption = itoa(side.w) + " × " + itoa(side.h) + " · " + caption
	}
	return caption
}
