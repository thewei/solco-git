package nativeui

import "github.com/egoist/mygo/ui"

/**
 * [INPUT]: 一个文件的新增/删除行数、gdPalette
 * [OUTPUT]: gdStatBlocks（五格里绿、红各占几格）、gdStatBar（把它画成一条按比例的小条）
 * [POS]: 文件行上 +N −N 旁的比例条（2026-11-12 决定第 (14) 条）；规则同 GitHub：有新增至少一格绿，有删除至少一格红，其余按比例，剩下的灰
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

// gdStatCells is how many cells the bar has.
const gdStatCells = 5

// gdStatBlocks says how many of the bar's cells are green and how many red.
// Both zero (no change) gives an empty bar.
func gdStatBlocks(adds, dels int) (green, red int) {
	total := adds + dels
	if total <= 0 {
		return 0, 0
	}
	// Rounded with ties down, so equal counts fill the same number of cells.
	green = (2*adds*gdStatCells + total - 1) / (2 * total)
	red = (2*dels*gdStatCells + total - 1) / (2 * total)
	if adds > 0 && green == 0 {
		green = 1
	}
	if dels > 0 && red == 0 {
		red = 1
	}
	for green+red > gdStatCells {
		if green >= red {
			green--
		} else {
			red--
		}
	}
	return green, red
}

// gdStatBar draws the cells of gdStatBlocks, small and quiet next to the
// counts it summarises.
func gdStatBar(c *ui.Context, pal *gdPalette, adds, dels int) {
	green, red := gdStatBlocks(adds, dels)
	ui.Row(c).Gap(1).Shrink(0).AlignItems(ui.Center).Label("Change proportion").Children(func() {
		for i := 0; i < gdStatCells; i++ {
			fill := pal.pill
			switch {
			case i < green:
				fill = pal.addBar
			case i < green+red:
				fill = pal.delBar
			}
			ui.Box(c).Size(4, 8).Radius(1).Shrink(0).Background(fill)
		}
	})
}
