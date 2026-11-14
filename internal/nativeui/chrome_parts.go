package nativeui

/**
 * [INPUT]: 依赖 theme/gorex_style 的 token 与按钮面、icons.go 的图标、git_service 的分支快照
 * [OUTPUT]: 对外提供标题带与侧栏共用的小部件：switcherChip（带下拉箭头的标题）、switcherSearch（弹层顶部搜索框）、crumbItemRow（弹层里的一行）、toggleMenu（点击翻转弹层开关）、matchingLocalBranches（当前分支优先的过滤）、headerActions（⋯ Actions 菜单触发钮）、bandAction（标题带上带文字的 Fetch/Pull/Push 按钮）
 * [POS]: 从 Solco 的 sidebar_switchers/breadcrumb_switcher/branch_menu/header_overflow 摘来的通用部分（去掉 Herdr 项目/Pane 依赖）；视觉与 Solco 同一份，改动时与 ../solco 的同名函数对照（scripts/kit-parity.sh 列出差异）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// switcherListMaxHeight caps a switcher panel's scrolling list.
const switcherListMaxHeight = 280

// switcherChip is a pull-down title: glyph, text and a chevron hugging the
// text, with a hover face that spans only the content. Its glyph and text sit
// on the tree grid, under the chevron and title columns of the rows below.
func switcherChip(c *ui.Context, key, label, text, tooltip string, glyph *ui.SVG, size float32, weight int, color ui.Color) ui.Element {
	t := c.Theme()
	chip := tipped(c, ui.ButtonBase(c.Key(key)).Height(22).Padding(0, treeRowPad).Radius(Radius().Control).FocusRing(false).
		AlignItems(ui.Center).Label(label), tooltip)
	if chip.Hovered() {
		chip.Background(t.SurfaceHover)
	}
	chip.Children(func() {
		ui.Icon(c, glyph).FontSize(treeGlyphW).TextColor(t.TextMuted).Shrink(0)
		ui.Box(c).Width(treeGlyphGap).Shrink(0).PassThrough()
		if text != "" {
			ui.Text(c, text).FontSize(size).FontWeight(weight).TextColor(color).SingleLine().Shrink(1).MinWidth(0)
		}
		ui.Box(c).Width(3).Shrink(0).PassThrough()
		ui.Icon(c, iconChevronDown).FontSize(Controls().IconSmall).TextColor(t.TextMuted).Shrink(0)
	})
	return chip
}

// switcherSearch is the search field every switcher panel opens with. It
// reports Enter, which picks the first match.
func switcherSearch(c *ui.Context, text *string, label string) (submitted bool) {
	field := searchField(c, text).FillWidth().AutoFocus().Label(label)
	return field.Submitted()
}

// crumbItemRow is one switcher row: glyph, label, selected tint.
func crumbItemRow(c *ui.Context, glyph *ui.SVG, label string, selected bool) ui.Element {
	t := c.Theme()
	tokens := designTokens(t.Dark)
	var row ui.Element
	ui.Box(c).Children(func() {
		row = ui.ButtonBase(c).
			FillWidth().
			Height(Controls().RegularHeight).
			Radius(Radius().Row).
			Padding(0, 8).
			Gap(7).
			Label(label)
		if tint, ok := rowTint(tokens, selected, row.Hovered()); ok {
			row.Background(tint)
		}
		row.Children(func() {
			ui.Icon(c, glyph).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, label).FontSize(Typography().Body).Grow(1).SingleLine()
		})
	})
	return row
}

// toggleMenu flips *open when e was clicked since the last frame. The
// click is consumed by the first pass that asks for it, so later passes
// of the same frame cannot flip it twice. Clicks() cannot be used here:
// the engine resets every element's click count at the end of each pass,
// so a count kept across frames would swallow every click after the
// first.
func toggleMenu(open *bool, e ui.Element) {
	if e.Clicked() {
		*open = !*open
	}
}

// matchingLocalBranches keeps the current branch visible first when
// it matches the query. The filter is local presentation only.
func matchingLocalBranches(branches []gitworkbench.Branch, filter string) []gitworkbench.Branch {
	query := strings.ToLower(strings.TrimSpace(filter))
	result := make([]gitworkbench.Branch, 0, len(branches))
	for _, branch := range branches {
		if branch.Current && strings.Contains(strings.ToLower(branch.Name), query) {
			result = append(result, branch)
		}
	}
	for _, branch := range branches {
		if !branch.Current && strings.Contains(strings.ToLower(branch.Name), query) {
			result = append(result, branch)
		}
	}
	return result
}

// headerActions draws the Actions trigger with build as its menu: a bare
// ellipsis icon button that opens the menu. Every view's title band shows
// its actions through this one trigger, so the control sits in the same place
// and reads the same on each view.
func headerActions(c *ui.Context, build func(m *ui.Menu)) {
	k := gorexColorsOf(c.Theme().Dark)
	var trigger ui.Element
	ui.Box(c).Children(func() {
		trigger = gorexIconButton(c, k, iconActions, "Actions", "Actions", false, gorexIconBtn, gorexIconGlyph)
	})
	trigger.Menu(build)
}

// bandAction is a labeled button of the title band (Fetch, Pull, Push): glyph,
// word, and an optional count in the accent. tip is the tooltip; on lights
// the label (a push with commits waiting).
func bandAction(c *ui.Context, k *gorexColors, glyph *ui.SVG, label, tip string, count int, on bool) ui.Element {
	t := c.Theme()
	b := tipped(c, ui.ButtonBase(c.Key("band:"+label)).Height(Controls().RegularHeight).Padding(0, 10).Gap(6).
		Radius(Radius().Row).AlignItems(ui.Center).Shrink(0).Label(label), tip)
	gorexToggleFace(b, k, false)
	ink := t.TextMuted
	if on || b.Hovered() {
		ink = t.Text
	}
	b.Children(func() {
		ui.Icon(c, glyph).Size(Controls().IconRegular, Controls().IconRegular).TextColor(ink).Shrink(0).PassThrough()
		ui.Text(c, label).FontSize(Typography().Body).FontWeight(WeightMedium).TextColor(ink).SingleLine().PassThrough()
		if count > 0 {
			ui.Text(c, itoa(count)).FontSize(Typography().BodySmall).FontWeight(WeightSemibold).
				TextColor(t.Accent).SingleLine().PassThrough()
		}
	})
	return b
}
