package nativeui

/**
 * [INPUT]: 依赖 window.go 的最近列表与 OpenRepository/chooseRepository、gitworkbench.Runner 的当前分支读取、resources.AppIconPNG
 * [OUTPUT]: 对外提供 Window.welcome（清单为空时显示同一张应用图标、一句话、Open Repository…、最近仓库与拖入提示）、loadRecentBranches（后台读取最近仓库分支）、dropFolders（接收窗口拖入的文件夹）
 * [POS]: 窗口在没有工作区时的唯一内容；拖入、点最近、⌘O、Quick Launch 都汇到 OpenRepository，不在这里开第二条路
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-git/internal/settings"
	"github.com/thewei/solco-git/resources"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

var welcomeAppIcon = func() ui.ImageSource {
	image, err := ui.DecodeImage(resources.AppIconPNG)
	if err != nil {
		panic("decode Solco Git app icon: " + err.Error())
	}
	return image
}()

// welcome is the page of a window with no repository open.
func (w *Window) welcome(c *ui.Context) {
	t := c.Theme()
	tokens := designTokens(t.Dark)
	w.loadRecentBranches()
	ui.Column(c).Grow(1).MinWidth(0).Background(tokens.Content).Children(func() {
		ui.Box(c).FillWidth().Shrink(0).Height(titlebarHeight(c.TitleBar().Height)).DragWindow()
		ui.Column(c).Grow(1).MinHeight(0).Center().Padding(0, 0, 40).Children(func() {
			ui.Column(c).Width(520).MaxWidth(520).AlignItems(ui.Center).Children(func() {
				ui.Image(c, welcomeAppIcon).Size(72, 72).Fit(ui.Cover).Shrink(0)
				ui.Box(c).Height(14).Shrink(0).PassThrough()
				ui.Text(c, "Solco Git").FontSize(Typography().Title).FontWeight(WeightBold)
				ui.Box(c).Height(4).Shrink(0).PassThrough()
				// Two sentences, one line each: as one paragraph the second
				// wrapped to a single orphan word under the centered title.
				ui.Text(c, "Review changes, browse history, read source.").
					FontSize(Typography().Body).TextColor(t.TextMuted)
				ui.Text(c, "Each repository you open becomes a workspace.").
					FontSize(Typography().Body).TextColor(t.TextMuted)
				ui.Box(c).Height(18).Shrink(0).PassThrough()
				open := regularPrimaryButton(c, "").Label("Open Repository…").Gap(Spacing().S)
				open.Children(func() {
					ui.Icon(c, iconFolderOpen).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.AccentText).Shrink(0).PassThrough()
					ui.Text(c, "Open Repository…").Shrink(0).PassThrough()
					tipKeycaps(c, acceleratorKeycaps(accelOpen), t.Dark, t.AccentText)
				})
				if open.Clicked() {
					w.chooseRepository()
				}
				w.recentCard(c)
				ui.Box(c).Height(12).Shrink(0).PassThrough()
				ui.Text(c, "or drop a folder here").FontSize(Typography().BodySmall).TextColor(t.TextMuted)
			})
		})
	})
}

// recentCard lists the repositories opened before, newest first; nothing is
// drawn until there is one.
func (w *Window) recentCard(c *ui.Context) {
	recent := w.settings.Window.Recent
	if len(recent) == 0 {
		return
	}
	t := c.Theme()
	tokens := designTokens(t.Dark)
	ui.Box(c).Height(22).Shrink(0).PassThrough()
	ui.Column(c).FillWidth().Radius(cardRadius()).Background(tokens.Panel).Border(1, tokens.BorderSubtle).Clip().Children(func() {
		ui.Text(c, "Recent").FontSize(Typography().BodySmall).FontWeight(WeightMedium).TextColor(t.TextMuted).
			Padding(8, 14).FillWidth()
		for _, root := range recent {
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(tokens.BorderSubtle)
			row := ui.ButtonBase(c.Key("recent:"+root)).FillWidth().Height(44).Padding(0, 14).Gap(10).
				AlignItems(ui.Center).Label(pathBase(root))
			if tint, ok := rowTint(tokens, false, row.Hovered()); ok {
				row.Background(tint)
			}
			row.Children(func() {
				ui.Icon(c, iconFolder).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).Shrink(0).PassThrough()
				ui.Text(c, pathBase(root)).FontWeight(WeightSemibold).SingleLine().Shrink(0).PassThrough()
				ui.Text(c, tildePath(root)).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().
					Ellipsis("…").Grow(1).MinWidth(0).PassThrough()
				if branch := w.recentBranch[root]; branch != "" {
					ui.Icon(c, iconBranch).Size(Controls().IconSmall, Controls().IconSmall).TextColor(t.TextMuted).Shrink(0).PassThrough()
					ui.Text(c, branch).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine().
						Ellipsis("…").Shrink(0).MaxWidth(150).PassThrough()
				}
			})
			if row.Clicked() {
				w.OpenRepository(root, "", "")
			}
			row.ContextMenu(func(m *ui.Menu) {
				if m.Item("Remove from Recent").Chosen() {
					w.forgetRecent(root)
				}
			})
		}
	})
}

// forgetRecent drops one repository from the recent list.
func (w *Window) forgetRecent(root string) {
	_, _ = w.mutateSettings(func(cfg *settings.Settings) error {
		kept := cfg.Window.Recent[:0:0]
		for _, r := range cfg.Window.Recent {
			if r != root {
				kept = append(kept, r)
			}
		}
		cfg.Window.Recent = kept
		return nil
	})
}

// loadRecentBranches reads each recent repository's current branch once, off
// the UI lane; the list shows them as they arrive.
func (w *Window) loadRecentBranches() {
	if w.recentBranch == nil {
		w.recentBranch = map[string]string{}
		w.recentAsked = map[string]bool{}
	}
	for _, root := range w.settings.Window.Recent {
		if w.recentAsked[root] {
			continue
		}
		w.recentAsked[root] = true
		root := root
		read := func() {
			ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout)
			defer cancel()
			branch := w.runner.CurrentBranch(ctx, root)
			if branch == "" {
				return
			}
			w.onUI(func() {
				w.recentBranch[root] = branch
				if w.win != nil {
					w.win.Invalidate()
				}
			})
		}
		if w.win == nil {
			read() // headless: inline, like OpenRepository
			continue
		}
		go read()
	}
}

// dropFolders opens what was dropped on the window: a folder is a repository
// (or inside one); OpenRepository lets a file stand for the folder it is in.
func (w *Window) dropFolders(paths []string) {
	for _, path := range paths {
		w.OpenRepository(path, "", "")
	}
}
