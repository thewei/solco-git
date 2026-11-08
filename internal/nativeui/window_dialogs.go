package nativeui

/**
 * [INPUT]: 依赖 window.go 的设置与工作区、window_workspaces.go 的工作区管理与移除确认、settings 包的偏好文档、ds_components 的分组行、quick_launch.go 的浮层
 * [OUTPUT]: 对外提供 Window.windowDialogs（窗口级浮层：Settings 偏好面板、移除有草稿/正在运行的工作区前的确认、Quick Launch）、settingsSheet（Workspaces 工作区管理、Appearance 外观/侧栏密度/增强对比度、Window 恢复窗口与工作区、Files 富文本预览（仅当应用内嵌了预览页，`webPageShipped`）、Git 自动抓取）
 * [POS]: 不属于任何一个仓库工作区的浮层；偏好写入走 mutateSettings（落盘、广播给每个工作区、必要时重装已读过的工作区的自动抓取计时器）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"github.com/egoist/mygo/ui"

	"github.com/thewei/solco-git/internal/settings"
)

// windowDialogs renders the window's own modals over whatever workspace is in
// front.
func (w *Window) windowDialogs(c *ui.Context) {
	w.settingsSheet(c)
	w.removeConfirm(c)
	w.quickLaunchView(c)
}

// settingsSheet is the preferences panel (⌘,): what the app itself offers,
// grouped as macOS' own Settings does.
func (w *Window) settingsSheet(c *ui.Context) {
	if !w.settingsOpen {
		return
	}
	cfg := w.settings
	ui.Modal(c, &w.settingsOpen, func() {
		ui.Column(c).Width(520).MaxWidth(560).Gap(Spacing().L).Children(func() {
			modalTitle(c, "Settings")
			ui.Scroll(c).FillWidth().MaxHeight(460).Children(func() {
				ui.Column(c).FillWidth().Gap(Spacing().L).Children(func() {
					w.workspacesGroup(c)
					settingsGroup(c, "Appearance",
						row("Theme", "", func() {
							chosen := appearanceIndex(cfg.General.Appearance)
							if segmentedControl(c, &chosen, appearanceOptions...).Label("Theme").Changed() {
								w.change(func(v *settings.Settings) { v.General.Appearance = appearanceFromIndex(chosen) })
							}
						}),
						row("Sidebar density", "", func() {
							idx := densityIndex(cfg.General.Density)
							if segmentedControl(c, &idx, "Compact", "Default", "Comfortable").Label("Sidebar density").Changed() {
								w.change(func(v *settings.Settings) { v.General.Density = densityFromIndex(idx) })
							}
						}),
						row("Increase contrast", "", func() {
							if on, changed := settingSwitch(c, "Increase contrast", cfg.General.HighContrast); changed {
								w.change(func(v *settings.Settings) { v.General.HighContrast = on })
							}
						}),
					)
					settingsGroup(c, "Window",
						row("Restore window and workspaces", "Reopens your workspaces, on the one you left, at the same place on screen.", func() {
							if on, changed := settingSwitch(c, "Restore window and workspaces", cfg.General.RestoreWindow); changed {
								w.change(func(v *settings.Settings) { v.General.RestoreWindow = on })
							}
						}),
					)
					settingsGroup(c, "Git",
						row("Auto-fetch", "Fetches the remote of each workspace you have opened this session, quietly, in the background. It never merges or pushes.", func() {
							label := autoFetchLabel(cfg.Workbench.AutoFetchMinutes)
							if selectField(c, &label, autoFetchLabels()).Label("Auto-fetch").Changed() {
								w.change(func(v *settings.Settings) { v.Workbench.AutoFetchMinutes = autoFetchMinutes(label) })
								for _, ws := range w.workspaces {
									// Only a workspace that was read keeps a timer; the
									// ones never shown stay quiet until their first visit.
									if ws.sh.git != nil && ws.sh.git.root != "" {
										ws.sh.rearmAutoFetch()
									}
								}
							}
						}),
					)
				})
			})
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if regularPrimaryButton(c, "Done").Clicked() {
					w.settingsOpen = false
				}
			})
		})
	})
}

// change writes one preference.
func (w *Window) change(edit func(*settings.Settings)) {
	_, _ = w.mutateSettings(func(cfg *settings.Settings) error {
		edit(cfg)
		return nil
	})
	w.syncThemeSource()
}

// densityIndex and densityFromIndex map the density preference onto its
// segmented control: "" and "default" are the middle one.
func densityIndex(value string) int {
	switch value {
	case "compact":
		return 0
	case "comfortable":
		return 2
	}
	return 1
}

func densityFromIndex(index int) string {
	switch index {
	case 0:
		return "compact"
	case 2:
		return "comfortable"
	}
	return "default"
}

// autoFetchMinutes is the cadence a menu label stands for; unknown means off.
func autoFetchMinutes(label string) int {
	for _, choice := range autoFetchChoices {
		if choice.label == label {
			return choice.minutes
		}
	}
	return 0
}
