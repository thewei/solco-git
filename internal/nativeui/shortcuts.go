package nativeui

/**
 * [INPUT]: 依赖 mygo/ui 的 Modifiers/Key
 * [OUTPUT]: 对外提供菜单加速键常量（accel*，菜单栏与工具提示读同一份；含 Back ⌘[ / Forward ⌘]、⌃⇥ / ⌃⇧⇥）、acceleratorKeycaps（把加速键拼成 ⌃⌥⇧⌘ 顺序的键帽）、shortcutBinding（窗口内快捷键与它的键帽）
 * [POS]: 键位的单一来源：菜单里列出的键就是绑定的键；窗口内快捷键只有菜单给不了的那一个（⌘B）。⌃Tab / ⌃⇧Tab 不能放这里：AppKit 在内容视图收到按键之前就把它们当作 key-view 循环吞掉（2026-11-08 探针实测），只有菜单项的加速键拦得住，而且必须是可见项（Hidden 项的键位不触发，同日实机实测）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

type shortcutBinding struct {
	Modifiers ui.Modifiers
	Key       ui.Key
}

// The in-window binding. Everything else is a menu accelerator below. ⌃Tab
// and ⌃⇧Tab cannot be bound here: AppKit eats them as the key-view loop before
// the content view's keyDown runs (a bare NSView probe, 2026-11-08, saw
// performKeyEquivalent and no keyDown), so they are View menu items too —
// the one place an AppKit app can catch them (Terminal's and Safari's
// Control-Tab are menu items).
var sidebarToggleBinding = shortcutBinding{Modifiers: ui.Super, Key: ui.KeyB}

// The menu bar's accelerators.
const (
	accelChanges       = "CmdOrCtrl+1"
	accelCommits       = "CmdOrCtrl+2"
	accelFiles         = "CmdOrCtrl+3"
	accelOpen          = "CmdOrCtrl+O"
	accelSave          = "CmdOrCtrl+S"
	accelQuickLaunch   = "CmdOrCtrl+Shift+O"
	accelCloseWindow   = "CmdOrCtrl+W"
	accelBack          = "CmdOrCtrl+["
	accelForward       = "CmdOrCtrl+]"
	accelNextWorkspace = "CmdOrCtrl+Shift+]"
	accelPrevWorkspace = "CmdOrCtrl+Shift+["
	accelNextCtrlTab   = "Ctrl+Tab"
	accelPrevCtrlTab   = "Ctrl+Shift+Tab"
	accelRefresh       = "CmdOrCtrl+R"
	// Undo / Redo Git Action are not ⌘Z / ⇧⌘Z: those belong to the Edit menu's
	// text undo, which the framework routes to the focused field as an edit
	// command, so a key bound to them never reaches the view.
	accelUndoGit  = "CmdOrCtrl+Alt+Z"
	accelRedoGit  = "CmdOrCtrl+Alt+Shift+Z"
	accelSettings = "CmdOrCtrl+,"
)

// acceleratorKeycaps spells a menu accelerator as keycaps in the macOS order
// (⌃⌥⇧⌘ then the key).
func acceleratorKeycaps(accelerator string) []string {
	parts := strings.Split(accelerator, "+")
	key := parts[len(parts)-1]
	var caps []string
	for _, modifier := range []struct{ name, cap string }{
		{"Ctrl", "⌃"}, {"Alt", "⌥"}, {"Shift", "⇧"}, {"CmdOrCtrl", "⌘"}, {"Cmd", "⌘"},
	} {
		for _, part := range parts[:len(parts)-1] {
			if part == modifier.name {
				caps = append(caps, modifier.cap)
			}
		}
	}
	if key == "Tab" {
		key = "⇥"
	}
	return append(caps, strings.ToUpper(key))
}

// keycaps spells the binding as a tooltip shows it. A key with no known cap
// yields nil — no hint rather than a wrong one.
func (b shortcutBinding) keycaps() []string {
	key := keycapName(b.Key)
	if key == "" {
		return nil
	}
	var caps []string
	for _, modifier := range []struct {
		flag ui.Modifiers
		cap  string
	}{{ui.Ctrl, "⌃"}, {ui.Alt, "⌥"}, {ui.Shift, "⇧"}, {ui.Super, "⌘"}} {
		if b.Modifiers&modifier.flag != 0 {
			caps = append(caps, modifier.cap)
		}
	}
	return append(caps, key)
}

// keycapName is the cap of a key; only the keys this app binds.
func keycapName(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + key - ui.Key0))
	}
	return ""
}
