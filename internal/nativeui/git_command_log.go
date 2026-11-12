package nativeui

/**
 * [INPUT]: gitworkbench.CommandLog（共享的内存命令日志，由 Runner 与 engine 同时写入）、shell.go（git 状态）
 * [OUTPUT]: openCommandLog（Repository ▸ Command Log…：本仓库里这次运行做过的 git 命令，新的在前，默认不含读）、commandLogSheet、点一行复制那条命令、Copy All、Clear
 * [POS]: 2026-11-12 决定第 (11) 条命令日志的界面一侧；列表只来自内存，不落盘、不进 applog；commit 信息走 stdin，不会出现在这里
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// sharedCommandLog is the one log of the app: every workspace's Runner and
// the shared Engine write to it, each entry carrying its repository root.
var sharedCommandLog = sync.OnceValue(gitworkbench.NewCommandLog)

type commandLogState struct {
	open  bool
	reads bool
	list  ui.ListState
}

func (s *Shell) openCommandLog() {
	if s.git == nil || s.git.root == "" {
		return
	}
	s.gitMore.commands.open = true
}

func (s *Shell) commandLogSheet(c *ui.Context) {
	st := &s.gitMore.commands
	if !st.open || s.git == nil {
		return
	}
	t := c.Theme()
	root := s.git.root
	entries := sharedCommandLog().Entries(root, st.reads)
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(820).MaxWidth(1000).Height(520).MinHeight(240).MinWidth(0).Gap(Spacing().M).Children(func() {
			modalTitle(c, "Command Log")
			ui.Text(c, "The Git commands Solco Git ran in this repository since it started, newest first. Click one to copy it. Nothing is saved to disk.").
				FontSize(Typography().BodySmall).TextColor(t.TextMuted)
			if len(entries) == 0 {
				ui.Text(c, "Nothing yet.").FontSize(Typography().Body).TextColor(t.TextMuted)
			} else {
				ui.List(c, &st.list, len(entries), func(i int) {
					if i < 0 || i >= len(entries) {
						return
					}
					s.commandLogRow(c, entries[i])
				}).Grow(1).MinHeight(0)
			}
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &st.reads, "Show reads")
				ui.Spacer(c)
				if regularButton(c, "Copy All").Disabled(len(entries) == 0).Clicked() {
					s.copyTextToClipboard(commandLogText(entries))
				}
				if regularButton(c, "Clear").Disabled(len(entries) == 0).Clicked() {
					sharedCommandLog().Clear(root)
				}
				if regularPrimaryButton(c, "Close").Clicked() {
					st.open = false
				}
			})
		})
	})
}

func (s *Shell) commandLogRow(c *ui.Context, e gitworkbench.CommandEntry) {
	t := c.Theme()
	line := e.Line()
	row := ui.Row(c).Gap(8).Padding(4, 8).Radius(treeRowRadius).AlignItems(ui.Center).MinWidth(0)
	if row.Hovered() {
		row.Background(designTokens(t.Dark).SidebarHover)
	}
	if row.Clicked() {
		s.copyTextToClipboard(line)
	}
	tipped(c, row, commandLogTip(e))
	row.Children(func() {
		kind, tone := "write", t.TextMuted
		if e.Class == gitworkbench.OpRead {
			kind = "read"
		}
		if e.Failed() {
			kind, tone = "failed", t.Warning
		}
		ui.Row(c).Size(52, 18).Shrink(0).Center().Radius(treeRowRadius).Background(tone.Alpha(0.14)).Children(func() {
			ui.Text(c, kind).FontSize(Typography().Caption).FontWeight(WeightMedium).TextColor(tone).SingleLine()
		})
		ui.Text(c, line).Font(gdCodeFont()).FontSize(Typography().BodySmall).Grow(1).MinWidth(0).SingleLine()
		ui.Text(c, commandDuration(e.Duration)).FontSize(Typography().BodySmall).TextColor(t.TextMuted).
			Width(56).TextAlign(ui.End).Shrink(0).SingleLine()
		ui.Text(c, e.Created().Format("15:04:05")).FontSize(Typography().BodySmall).TextColor(t.TextMuted).
			Width(64).TextAlign(ui.End).Shrink(0).SingleLine()
	})
}

func commandLogTip(e gitworkbench.CommandEntry) string {
	tip := e.Line()
	if e.Failed() {
		tip += "\nfailed: " + string(e.Failure) + ", exit " + itoa(e.ExitCode)
	}
	return tip + "\nClick to copy"
}

// commandLogText is Copy All: one command per line, oldest first, so it reads
// as a script.
func commandLogText(entries []gitworkbench.CommandEntry) string {
	var b strings.Builder
	for i := len(entries) - 1; i >= 0; i-- {
		b.WriteString(entries[i].Line())
		b.WriteByte('\n')
	}
	return b.String()
}

func commandDuration(d time.Duration) string {
	if d < time.Second {
		return itoa(int(d/time.Millisecond)) + " ms"
	}
	return itoa(int(d/time.Second)) + "." + itoa(int(d%time.Second/(100*time.Millisecond))) + " s"
}
