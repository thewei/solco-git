package nativeui

/**
 * [INPUT]: gd_text.go 的 gdStatText、sidebar_tree.go 的分区计数、changes_panel / gd_surface / sidebar 的 +/- 统计
 * [OUTPUT]: 零不上屏的合同：+/- 统计只写非零的一侧、没有任何文字恰好是 "0" 或 "-0"、分区标题的零计数不画
 * [POS]: 2026-11-14 UX 打磨：隐藏显示为 0 的统计
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"
)

func TestStatTextLeavesAZeroSideOut(t *testing.T) {
	for _, c := range []struct {
		adds, dels int
		want       string
	}{{3, 0, "+3"}, {0, 5, "-5"}, {2, 7, "+2 -7"}, {0, 0, ""}} {
		if got := gdStatText(c.adds, c.dels, itoa, "-"); got != c.want {
			t.Errorf("gdStatText(%d, %d) = %q, want %q", c.adds, c.dels, got, c.want)
		}
	}
}

// An added file has no removed lines to count; a header with no remotes has no
// number to show. Neither draws a zero.
func TestNoZeroStatisticIsDrawn(t *testing.T) {
	root := gitFixture(t, "zero-stats")
	writeFile(t, root+"/added.txt", "one\ntwo\n")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 6)

	check := func(where string) {
		t.Helper()
		for _, text := range tt.Texts() {
			trimmed := strings.TrimSpace(text)
			if trimmed == "0" || trimmed == "-0" || strings.HasSuffix(trimmed, " -0") || strings.HasPrefix(trimmed, "+0 ") {
				t.Errorf("%s draws a zero: %q", where, text)
			}
		}
	}
	check("Changes")
	if err := tt.Click("Commits"); err != nil {
		t.Fatal(err)
	}
	framesUntil(t, tt, pump, "the references", func() bool { return sh.git.refsRead && tt.HasText("Remotes") })
	settle(tt, pump, 4)
	if !tt.HasText("Remotes") || !tt.HasText("Stashes") {
		t.Fatalf("empty sections keep their headers: %q", tt.Texts())
	}
	check("the references")
}
