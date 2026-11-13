package nativeui

/**
 * [INPUT]: terminal_drawer.go 的 openTerminal / terminalToggle、window_dialogs.go 的 terminalGroup、window_menu.go、platform.SolcoLink（测试里用 NewSolcoLinkAt，不会启动 Solco）
 * [OUTPUT]: 「终端归属」的合同（2026-11-13 决定第 (5) 条）：默认是本应用的抽屉；设置选 In Solco 后 ⌃` / 按钮 / View 菜单项把仓库根交给 Solco（链接带的是仓库根）且不开抽屉；Solco 没装就说明并且不悄悄退回抽屉；设置页的二选一写进偏好
 * [POS]: 不依赖终端库（抽屉只检查开合）；变异检查：去掉归属判断 / 没装时回退抽屉 / 链接换成别的目录，对应测试必红
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"testing"
	"time"

	"github.com/thewei/solco-git/internal/platform"
	"github.com/thewei/solco-git/internal/settings"
)

func solcoShell(t *testing.T, inSolco bool, installed bool) (*Window, *uiPump, *Shell, chan string) {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")
	w, pump, sh, _, _ := previewShell(t, nil)
	got := make(chan string, 4)
	path := ""
	if installed {
		path = "/Applications/Solco.app"
	}
	w.solco = platform.NewSolcoLinkAt(path, func(app, link string) error { got <- app + " " + link; return nil })
	w.change(func(v *settings.Settings) { v.Workbench.TerminalInSolco = inSolco })
	t.Cleanup(sh.closeTerminal)
	return w, pump, sh, got
}

func TestOpenTerminalDefaultsToTheDrawer(t *testing.T) {
	_, _, sh, got := solcoShell(t, false, true)
	sh.openTerminal()
	if !sh.term.open {
		t.Error("by default Open Terminal is this app's drawer")
	}
	select {
	case link := <-got:
		t.Errorf("nothing goes to Solco by default: %q", link)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOpenTerminalInSolcoHandsTheRepositoryRootOverAndOpensNoDrawer(t *testing.T) {
	_, _, sh, got := solcoShell(t, true, true)
	sh.openTerminal()
	select {
	case link := <-got:
		want, _ := platform.SolcoTerminalURL(sh.dir)
		if link != "/Applications/Solco.app "+want {
			t.Errorf("link = %q, want the repository root %q", link, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Solco was never asked")
	}
	if sh.term.open || sh.term.t != nil {
		t.Errorf("no drawer of our own while the terminal is Solco's: open=%v term=%v", sh.term.open, sh.term.t != nil)
	}
}

func TestOpenTerminalInSolcoWithoutSolcoSaysSoAndDoesNotFallBack(t *testing.T) {
	_, _, sh, got := solcoShell(t, true, false)
	sh.openTerminal()
	if sh.pendingToast == "" {
		t.Error("a missing Solco is said")
	}
	if sh.term.open || sh.term.t != nil {
		t.Error("never silently falls back to the drawer")
	}
	select {
	case link := <-got:
		t.Errorf("nothing to hand a link to: %q", link)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTheSettingsPageChoosesTheTerminalOwnerAndMenuAndButtonFollow(t *testing.T) {
	w, pump, sh, _ := solcoShell(t, false, true)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 2)
	if _, ok := fileAndViewItems(w)["Toggle Terminal"]; !ok {
		t.Fatal("the drawer's menu item by default")
	}
	w.openSettings()
	settle(tt, pump, 3)
	if !tt.HasText("Where Open Terminal goes") || !tt.HasText("A drawer under the page") {
		t.Fatalf("the group explains the drawer: %q", tt.Texts())
	}
	if err := tt.Click("In Solco"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 3)
	if !w.settings.Workbench.TerminalInSolco || !sh.terminalInSolco() {
		t.Fatalf("the choice is stored: window=%v shell=%v", w.settings.Workbench.TerminalInSolco, sh.terminalInSolco())
	}
	if !tt.HasText("hands the repository to Solco") {
		t.Errorf("the note follows the choice: %q", tt.Texts())
	}
	saveEvidence(t, "settings-terminal-solco", tt)
	w.settingsOpen = false
	settle(tt, pump, 2)
	items := fileAndViewItems(w)
	if got, ok := items["Open Terminal in Solco"]; !ok || got != accelTerminal {
		t.Errorf("the menu item follows: %q (present %v)", got, ok)
	}
	if _, ok := tt.Find("Open terminal in Solco"); !ok {
		t.Errorf("the band's button follows: %q", tt.Texts())
	}
}

func TestSettingsSaysWhenSolcoIsNotInstalled(t *testing.T) {
	w, pump, _, _ := solcoShell(t, true, false)
	tt := newTester(w, 1280, 800, false)
	w.openSettings()
	settle(tt, pump, 3)
	if !tt.HasText("Solco isn't installed") {
		t.Errorf("the group says it: %q", tt.Texts())
	}
}
