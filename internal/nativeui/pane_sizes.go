package nativeui

/**
 * [INPUT]: internal/settings 的 Window.Panes、ds_components.go 的 splitHandle/splitDrag、shell.go 的各拖拽尺寸字段
 * [OUTPUT]: paneSizeMax；Shell.loadPaneSizes（新工作区带上上次拖出的尺寸）/ splitFinished（一次拖拽或双击复位结束的瞬间）/ savePaneSizes（四个尺寸一次写回偏好，0 = 默认）
 * [POS]: 拖拽缝的尺寸记忆（2026-11-14）；窗口级偏好，不含内容；已开的工作区各保各的，重开才取最新
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import "github.com/thewei/solco-git/internal/settings"

// paneSizeMax bounds a stored pane size.
const paneSizeMax = 4000

// loadPaneSizes applies the stored sizes over the defaults; the render clamps
// them to what the window leaves, so an old size on a small window is safe.
func (s *Shell) loadPaneSizes() {
	p := s.settings.Window.Panes
	if p.StagedHeight > 0 {
		s.stagedPaneHeight, s.stagedPaneSized = float32(p.StagedHeight), true
	}
	if p.HistoryHeight > 0 {
		s.gitUpperHeight, s.gitUpperSized = float32(p.HistoryHeight), true
	}
	if p.FileNavWidth > 0 {
		s.gitFilePaneWidth = float32(p.FileNavWidth)
	}
	if p.TerminalHeight > 0 {
		s.term.height = float32(p.TerminalHeight)
	}
}

// splitFinished reports the frame a seam's drag ended or a double click reset
// it: the moment to write the sizes down (never every frame of a drag).
func (s *Shell) splitFinished(key string, drag splitDrag) bool {
	if s.splitDown == nil {
		s.splitDown = map[string]bool{}
	}
	was := s.splitDown[key]
	s.splitDown[key] = drag.Held
	return drag.Reset || (was && !drag.Held)
}

// savePaneSizes writes the four sizes into the preferences; a size still at its
// default is stored as 0, so a later change of the default reaches it.
func (s *Shell) savePaneSizes() {
	var p settings.PaneSizes
	if s.stagedPaneSized {
		p.StagedHeight = int(s.stagedPaneHeight)
	}
	if s.gitUpperSized {
		p.HistoryHeight = int(s.gitUpperHeight)
	}
	if s.gitFilePaneWidth != gitFilePaneDefaultWidth {
		p.FileNavWidth = int(s.gitFilePaneWidth)
	}
	if s.term.height != 0 && s.term.height != termDefaultHeight {
		p.TerminalHeight = int(s.term.height)
	}
	s.applySettings(func(cfg *settings.Settings) error {
		cfg.Window.Panes = p
		return nil
	})
}
