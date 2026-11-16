package nativeui

/**
 * [INPUT]: keymap.go 的 keysIn（键位表）、tooltip.go 的 tipKeycaps（键帽）、dialogs.go 的 modalTitle
 * [OUTPUT]: openKeyHelp（`?`：列出当前上下文的键与到处都能用的键）、openKeyHelpForView（Help 菜单的 Keyboard Shortcuts：按当前视图选上下文，菜单不靠字母键）、keyHelpSheet（浮层本身，由 gitMoreDialogs 挂载）
 * [POS]: LazyGit 的 `?`：键位不靠记，按一下就看到「这个列表里按什么」；读的是 keymap 本身，没有第二份清单
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import "github.com/egoist/mygo/ui"

type keyHelpState struct {
	open bool
	ctx  keyContext
}

func (s *Shell) openKeyHelp(ctx keyContext) {
	s.gitMore.keys = keyHelpState{open: true, ctx: ctx}
}

// openKeyHelpForView is the Help menu's entry: the keys of the view in front
// (no list holds the keyboard when a menu is used).
func (s *Shell) openKeyHelpForView() {
	ctx := keysChanges
	switch s.currentView() {
	case viewCommits:
		ctx = keysCommits
	case viewFiles:
		ctx = keysFiles
	}
	s.openKeyHelp(ctx)
}

func (s *Shell) keyHelpSheet(c *ui.Context) {
	st := &s.gitMore.keys
	if !st.open {
		return
	}
	t := c.Theme()
	own, anywhere := keysIn(st.ctx)
	ui.Modal(c, &st.open, func() {
		ui.Column(c).Width(460).MaxWidth(560).MinWidth(0).Gap(Spacing().M).Children(func() {
			modalTitle(c, "Keyboard Shortcuts")
			s.keyHelpGroup(c, st.ctx.title(), own)
			s.keyHelpGroup(c, keysAny.title(), anywhere)
			ui.Text(c, "These keys work while a list holds the keyboard, not while you type in a field.").
				FontSize(Typography().BodySmall).TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Children(func() {
				ui.Spacer(c)
				if regularPrimaryButton(c, "Close").Clicked() {
					st.open = false
				}
			})
		})
	})
}

func (s *Shell) keyHelpGroup(c *ui.Context, title string, rows []keyAction) {
	if len(rows) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(Spacing().XS).Children(func() {
		ui.Text(c, title).FontSize(Typography().BodySmall).FontWeight(WeightMedium).TextColor(t.TextMuted)
		for _, a := range rows {
			ui.Row(c).FillWidth().Gap(Spacing().M).AlignItems(ui.Center).Padding(2, 0).Children(func() {
				ui.Row(c).Width(132).Shrink(0).Children(func() { tipKeycaps(c, a.caps, t.Dark, t.Text) })
				ui.Text(c, a.label).FontSize(Typography().Body).Grow(1).MinWidth(0)
			})
		}
	})
}
