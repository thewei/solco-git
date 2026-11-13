package nativeui

import "testing"

/**
 * [INPUT]: gd_diffstat.go
 * [OUTPUT]: 比例条的合同：空改动无格、有新增必有绿、有删除必有红、两者同数则同格、总格数不超五
 * [POS]: 2026-11-12 决定第 (14) 条
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

func TestStatBlocksFollowTheProportion(t *testing.T) {
	for _, c := range []struct{ adds, dels, green, red int }{
		{0, 0, 0, 0},
		{10, 0, 5, 0},
		{0, 10, 0, 5},
		{1, 1, 2, 2},
		{3, 1, 4, 1},
		{1, 400, 1, 4}, // a lone add still shows one green cell
		{400, 1, 4, 1},
		{1, 1000000, 1, 4},
		{7, 3, 3, 1}, // 3.5 and 1.5 round down
	} {
		g, r := gdStatBlocks(c.adds, c.dels)
		if g != c.green || r != c.red {
			t.Errorf("gdStatBlocks(%d, %d) = %d,%d, want %d,%d", c.adds, c.dels, g, r, c.green, c.red)
		}
		if g+r > gdStatCells {
			t.Errorf("(%d,%d): %d cells exceed the bar", c.adds, c.dels, g+r)
		}
	}
}

func TestFileRowsShowTheProportionBar(t *testing.T) {
	root := movedRepo(t, "statbar-ui")
	sh, pump, tt := historyShell(t, root)
	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	settle(tt, pump, 3)
	saveEvidence(t, "diffstat-bar", tt)
	if _, ok := tt.Access("Change proportion"); !ok {
		t.Fatal("the changed file's row has no proportion bar")
	}
}
