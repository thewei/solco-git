package nativeui

/**
 * [INPUT]: MyGo 官方 plugins/terminal（Ghostty libghostty-vt，运行时加载，mygo build 放进 Contents/Resources）、shell.go 的 Shell 与舞台（frame.go stage）、window_workspaces.go 的移除确认
 * [OUTPUT]: 底部终端抽屉（2026-11-13 决定第 (5) 条 In Solco Git）：⌃` 开关（View 菜单与标题带按钮），每个工作区一个用户登录 shell、工作目录 = 仓库根；抽屉头有 Restart / Close；还有进程在跑时 Close / Restart / 移除工作区先问；窗口收起 / 工作区移除时终端随之结束
 * [POS]: 这是本应用自己的本地 shell，不是 Herdr 也不是第二个运行时：不持久化、不恢复、不记录终端字节。插件没有"前台进程"接口，所以用一层 sh 包装先把自己的 pid 写到临时文件再 exec 成登录 shell（pid 不变），"有进程在跑" = 该 pid 有子进程（pgrep -P）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/thewei/solco-git/internal/platform"
)

const (
	termMinHeight     float32 = 120
	termDefaultHeight float32 = 280
	termMinAbove      float32 = 160 // what the page above keeps
	termPageChrome    float32 = 60  // the title band and its margins, out of the window height
	termPidEnv                = "SOLCO_GIT_TERMINAL_PID_FILE"

	// termWrapper writes its own pid and becomes the user's login shell (the
	// name starts with "-", as terminal apps start it), so the pid stays the
	// shell's.
	termWrapper = `printf %s "$$" > "$` + termPidEnv + `"; sh=${SHELL:-/bin/zsh}; exec -a "-${sh##*/}" "$sh"`
)

// termDrawer is one workspace's terminal: whether the drawer is showing, the
// shell behind it and the sheets that ask before it ends.
type termDrawer struct {
	open     bool
	t        *terminal.Terminal
	gen      int // names the current terminal; a late callback of an older one is dropped
	exited   bool
	exitCode int
	title    string
	err      string
	pidFile  string
	height   float32
	focus    bool // the view takes the keyboard on the next frame
	// ask is the sheet in front: "" none, "close" or "restart". askBusy is
	// what pgrep said when it was raised (never per frame).
	ask     string
	askBusy bool
}

// terminalInSolco is the Settings choice "Open Terminal goes to Solco".
func (s *Shell) terminalInSolco() bool { return s.settings.Workbench.TerminalInSolco }

// solcoLink is the link to Solco; nil in headless tests that make no Window.
func (s *Shell) solcoLink() *platform.SolcoLink {
	if s.w == nil {
		return nil
	}
	return s.w.solco
}

// openTerminal is ⌃` and the band's button: this app's drawer, or — when the
// Settings choice says so — a terminal in Solco on this repository. It never
// falls back to the drawer without being told to: Solco missing is said.
func (s *Shell) openTerminal() {
	if !s.terminalInSolco() {
		s.toggleTerminal()
		return
	}
	link := s.solcoLink()
	if !link.Available() {
		s.pendingToast = "Solco isn't installed — choose In Solco Git in Settings to use this app's terminal"
		return
	}
	go func() {
		if err := link.OpenTerminal(s.dir); err != nil {
			slog.Warn("open terminal in Solco failed", "err", err)
			s.applyOnUI(func() { s.pendingToast = "Couldn't hand the terminal to Solco" })
		}
	}()
}

// toggleTerminal shows or hides the drawer; the first time it starts the shell.
func (s *Shell) toggleTerminal() {
	d := &s.term
	if d.open {
		d.open = false
		return
	}
	d.open = true
	if d.t == nil {
		s.startTerminal()
	}
	d.focus = true
}

// startTerminal ends the shell there is and starts a fresh one in the root.
func (s *Shell) startTerminal() {
	d := &s.term
	d.stopTerminal()
	d.err, d.title, d.exited = "", "", false
	file, err := os.CreateTemp("", "solco-git-terminal-*.pid")
	if err != nil {
		d.err = "Can't start a terminal: " + err.Error()
		return
	}
	file.Close()
	d.pidFile = file.Name()
	d.gen++
	gen := d.gen
	t, err := terminal.New(terminal.Options{
		Command: []string{"/bin/sh", "-c", termWrapper},
		Dir:     s.dir,
		Env:     []string{termPidEnv + "=" + d.pidFile},
		OnTitle: func(title string) {
			s.applyOnUI(func() {
				if gen == d.gen {
					d.title = title
				}
			})
		},
		OnExit: func(code int) {
			s.applyOnUI(func() {
				if gen == d.gen {
					d.exited, d.exitCode = true, code
				}
			})
		},
	})
	if err != nil {
		os.Remove(d.pidFile)
		d.pidFile = ""
		d.err = "Can't start a terminal: " + err.Error()
		slog.Warn("terminal start failed", "err", err)
		return
	}
	d.t = t
	d.focus = true
	slog.Info("terminal started")
}

// stopTerminal ends the shell and forgets it.
func (d *termDrawer) stopTerminal() {
	d.gen++ // whatever the old terminal still reports is dropped
	if d.t != nil {
		d.t.Close()
		d.t = nil
		slog.Info("terminal ended")
	}
	if d.pidFile != "" {
		os.Remove(d.pidFile)
		d.pidFile = ""
	}
	d.exited = false
}

// closeTerminal ends the shell and hides the drawer.
func (s *Shell) closeTerminal() {
	s.term.stopTerminal()
	s.term.open = false
	s.term.ask = ""
}

// termBusy reports a process still running under the shell (a command, an
// editor, a background job). It runs pgrep, so it is asked when the user acts,
// never per frame.
func (s *Shell) termBusy() bool {
	d := &s.term
	if d.t == nil || d.exited || d.pidFile == "" {
		return false
	}
	data, err := os.ReadFile(d.pidFile)
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(data))
	if _, err := strconv.Atoi(pid); err != nil {
		return false
	}
	out, err := exec.Command("pgrep", "-P", pid).Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// askTerminal raises the close / restart sheet, or acts at once when nothing
// runs under the shell.
func (s *Shell) askTerminal(kind string) {
	d := &s.term
	d.askBusy = s.termBusy()
	if !d.askBusy {
		s.doTerminal(kind)
		return
	}
	d.ask = kind
}

func (s *Shell) doTerminal(kind string) {
	s.term.ask = ""
	if kind == "restart" {
		s.startTerminal()
		return
	}
	s.closeTerminal()
}

// terminalDrawer is the drawer under the page: a resize handle, a head with
// the shell's title and Restart / Close, and the terminal.
func (s *Shell) terminalDrawer(c *ui.Context, pageH float32) {
	d := &s.term
	if !d.open {
		return
	}
	t := c.Theme()
	tokens := s.tokens(t.Dark)
	k := gorexColorsOf(t.Dark)
	if d.height == 0 {
		d.height = termDefaultHeight
	}
	maxH := max(pageH-termMinAbove, termMinHeight)
	height := clampF(d.height, termMinHeight, maxH)
	handle := ui.Row(c).Height(8).FillWidth().Shrink(0).Cursor(ui.CursorResizeNS).
		Label("Resize terminal").Center().Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(tokens.BorderSubtle)
	})
	if _, dy, held := handle.Dragged(); held {
		d.height = clampF(height-dy, termMinHeight, maxH)
		height = d.height
	}
	ui.Column(c).FillWidth().Height(height).Shrink(0).MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Height(32).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconTerminal).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, "Terminal").FontSize(Typography().BodySmall).FontWeight(WeightMedium).SingleLine().Shrink(0)
			status := d.title
			if d.exited {
				status = "exited " + strconv.Itoa(d.exitCode)
			}
			ui.Text(c, status).FontSize(Typography().Caption).TextColor(t.TextMuted).SingleLine().
				Ellipsis("…").Grow(1).MinWidth(0)
			if gorexIconButton(c, k, iconRefresh, "Restart terminal", "Restart terminal", false, gorexPaneBtn, gorexPaneGlyph).Clicked() {
				s.askTerminal("restart")
			}
			if gorexIconButton(c, k, iconClose, "Close terminal", "Close terminal", false, gorexPaneBtn, gorexPaneGlyph).Clicked() {
				s.askTerminal("close")
			}
		})
		switch {
		case d.err != "":
			ui.Text(c, d.err).FontSize(Typography().Caption).TextColor(t.Danger).Padding(Spacing().S, 12)
		case d.t != nil:
			view := terminal.View(c, d.t).Fill()
			if d.focus {
				view.AutoFocus()
				d.focus = false
			}
		}
	})
	s.terminalAsk(c)
}

// terminalAsk is the sheet before a running process is ended (Cancel last:
// Enter dismisses safely).
func (s *Shell) terminalAsk(c *ui.Context) {
	d := &s.term
	if d.ask == "" {
		return
	}
	title, verb := "Close the terminal?", "Close Terminal"
	if d.ask == "restart" {
		title, verb = "Restart the terminal?", "Restart Terminal"
	}
	open := true
	msg := "A process is still running in it; ending the terminal ends the process."
	if ui.AlertDialog(c, &open, title, msg, verb, "Cancel") == 0 {
		s.doTerminal(d.ask)
		return
	}
	if !open {
		d.ask = ""
	}
}

// terminalToggle is the title band's button: the drawer's switch, or "Open in
// Solco" when that is where the terminal lives (disabled, with the reason,
// while Solco is not installed).
func (s *Shell) terminalToggle(c *ui.Context) {
	k := gorexColorsOf(c.Theme().Dark)
	if s.terminalInSolco() {
		if !s.solcoLink().Available() {
			gorexIconButtonDisabled(c, k, iconTerminal, "Open terminal in Solco", "Solco isn't installed", gorexIconBtn, gorexIconGlyph)
			return
		}
		if gorexIconButton(c, k, iconTerminal, "Open terminal in Solco", "Open terminal in Solco", false, gorexIconBtn, gorexIconGlyph).Clicked() {
			s.openTerminal()
		}
		return
	}
	label := "Show terminal"
	if s.term.open {
		label = "Hide terminal"
	}
	if gorexIconButton(c, k, iconTerminal, label, label, s.term.open, gorexIconBtn, gorexIconGlyph).Clicked() {
		s.openTerminal()
	}
}
