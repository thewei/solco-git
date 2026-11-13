package nativeui

import (
	"image"
	"strings"
	"testing"

	"github.com/thewei/solco-git/internal/settings"
)

/**
 * [INPUT]: gd_surface.go 的移动行着色、gitworkbench.MovedLines、真实临时仓库、View 菜单开关
 * [OUTPUT]: 移动行高亮的界面合同：搬家的行用移动色（紫）而非增删色、悬停提示对侧行号、View 菜单开关关闭后回到增删色且设置落盘
 * [POS]: 2026-11-12 决定第 (14) 条的界面防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

func movedRepo(t *testing.T, name string) string {
	t.Helper()
	root := gitFixture(t, name)
	emptyBranch(t, root, "work")
	var old, moved strings.Builder
	for i := 1; i <= 40; i++ {
		line := "line number " + itoa(i) + "\n"
		switch i {
		case 4:
			old.WriteString("func relocatedHelper() int { return 42 }\n")
			continue
		case 36:
			moved.WriteString("func relocatedHelper() int { return 42 }\n")
		}
		old.WriteString(line)
		moved.WriteString(line)
	}
	writeFile(t, root+"/m.go", old.String())
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c1")
	writeFile(t, root+"/m.go", strings.Replace(moved.String(), "line number 36\n", "line number 36\n", 1))
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c2")
	return root
}

func TestMovedLinesAreDrawnInTheirOwnColorAndTheMenuSwitchTurnsThemOff(t *testing.T) {
	root := movedRepo(t, "moved-ui")
	sh, pump, tt := historyShell(t, root)
	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	f := gdFileOf(t, sh, "m.go")
	settle(tt, pump, 3)

	del := lineOf(t, f, 0, "func relocatedHelper() int { return 42 }")
	m, ok := f.movedAt(0, int32(del))
	if !ok || !m.Removed || m.Other == 0 {
		t.Fatalf("the relocated line is not marked as moved: %+v %v", m, ok)
	}
	if got, want := gdMovedTip(m), "Moved to line "+itoa(m.Other); got != want {
		t.Fatalf("tip = %q, want %q", got, want)
	}
	if m.Other != 35 {
		t.Fatalf("the line is at 35 in the new file, the mark says %d", m.Other)
	}

	f.resetDerived()
	if f.movedDone || f.moved != nil {
		t.Error("what was derived from the hunks survived a reset")
	}
	saveEvidence(t, "moved-on", tt)
	on := movedPixels(tt.Image())
	if on < 500 {
		t.Fatalf("moved background pixels = %d, want the relocated lines drawn in it", on)
	}

	sh.w.change(func(v *settings.Settings) { v.Workbench.HideMovedLines = true })
	settle(tt, pump, 3)
	saveEvidence(t, "moved-off", tt)
	if off := movedPixels(tt.Image()); off != 0 {
		t.Fatalf("with the switch off %d moved pixels remain", off)
	}
}

// movedPixels counts the pixels in the moved background of either palette.
func movedPixels(img *image.RGBA) int {
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		for _, p := range []*gdPalette{&gdLightPalette, &gdDarkPalette} {
			if img.Pix[i] == p.movedBg.R && img.Pix[i+1] == p.movedBg.G && img.Pix[i+2] == p.movedBg.B {
				n++
			}
		}
	}
	return n
}
