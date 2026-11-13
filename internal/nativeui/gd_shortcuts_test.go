package nativeui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

/**
 * [INPUT]: gd_surface.go 的 gdShortcuts、真实临时仓库的 Changes 视图、提交主题输入框
 * [OUTPUT]: 单键快捷键（J / K / ⌥Z）在输入框取走文本时让路：键入 "jk" 进到主题里而不是移动 hunk，⌥Z 不切换换行（macOS 随后送来的 “Ω” 照常进输入框）
 * [POS]: 2026-11-13 真窗口验证清单里「⌥Z 与 Option 字符」一项的合同
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

func TestSingleKeyShortcutsStepAsideWhileTheCommitFieldTakesText(t *testing.T) {
	root := gitFixture(t, "gd-keys")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { settle(tt, pump, 1); return sh.gitSnapshot() != nil && len(sh.git.gdFiles) > 0 })

	// outside a field the keys work
	tt.Key(ui.Alt, ui.KeyZ)
	settle(tt, pump, 1)
	if !sh.git.gdWordWrap {
		t.Fatal("⌥Z toggles word wrap when no field takes text")
	}
	tt.Key(ui.Alt, ui.KeyZ)
	settle(tt, pump, 1)

	if err := tt.Click("Commit subject draft"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 1)
	tt.TypeKey(0, ui.KeyJ, "j")
	tt.TypeKey(0, ui.KeyK, "k")
	tt.TypeKey(ui.Alt, ui.KeyZ, "Ω")
	settle(tt, pump, 1)
	if sh.git.gdWordWrap {
		t.Error("⌥Z must not toggle word wrap while the commit subject takes text")
	}
	if sh.gitQuickSubject != "jkΩ" {
		t.Errorf("subject = %q, want the three typed characters", sh.gitQuickSubject)
	}
}
