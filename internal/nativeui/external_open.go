package nativeui

/**
 * [INPUT]: 依赖 window.go 的 OpenRepository、MyGo App.OnOpenURL 的投递（main.go 注册）
 * [OUTPUT]: 对外提供 Window.OpenURL（main.go 的 OnOpenURL 接缝）、parseOpenURL、openRequest；窗口还没建好时先排队，AttachWindow 之后处理
 * [POS]: 外部应用（Solco 的"在 Solco Git 中打开"）请求"打开这个仓库"的唯一入口：solcogit://open?repo=<绝对路径>&view=changes|commits|files&file=<仓库内路径>；
 *        只打开、不改任何东西；日志从不记录 URL 参数（路径是用户内容）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
)

// maxOpenURLLen bounds what a link may carry; real ones are a path and a name.
const maxOpenURLLen = 8192

// openRequest is what solcogit://open names: a repository, optionally the
// view to show and a file to reveal in Files.
type openRequest struct {
	Repo string
	View viewKind // "" keeps the workspace's view
	File string
}

// parseOpenURL accepts solcogit://open?repo=…&view=…&file=…; repo must be an
// absolute path, an unknown view means the default one.
func parseOpenURL(raw string) (openRequest, bool) {
	if len(raw) > maxOpenURLLen {
		return openRequest{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "solcogit" || u.Host != "open" {
		return openRequest{}, false
	}
	q := u.Query()
	repo, file := q.Get("repo"), q.Get("file")
	if repo == "" || !filepath.IsAbs(repo) || strings.ContainsRune(repo, 0) || strings.ContainsRune(file, 0) {
		return openRequest{}, false
	}
	req := openRequest{Repo: filepath.Clean(repo), File: file}
	if name := q.Get("view"); name != "" {
		req.View = parseView(name)
	}
	if req.File != "" && req.View == "" {
		req.View = viewFiles // naming a file means showing it
	}
	return req, true
}

// OpenURL is the main.go seam for MyGo's OnOpenURL. It may be called from any
// goroutine, before or after the window exists.
func (w *Window) OpenURL(raw string) {
	req, ok := parseOpenURL(raw)
	if !ok {
		slog.Warn("ignored unsupported deep link")
		return
	}
	w.submit(req)
}

// OpenPath is the seam for a folder (or a file inside one) handed to the app
// by the system — `open -a "Solco Git" <folder>`, a drop on the Dock icon.
func (w *Window) OpenPath(path string) { w.submit(openRequest{Repo: path}) }

// submit opens a request now, or after the window is attached.
func (w *Window) submit(req openRequest) {
	w.pendingMu.Lock()
	if !w.attached {
		w.pending = append(w.pending, req)
		w.pendingMu.Unlock()
		return
	}
	w.pendingMu.Unlock()
	w.onUI(func() { w.OpenRepository(req.Repo, req.View, req.File) })
}

// finishAttach marks the window ready and opens the links that arrived
// before it was; AttachWindow calls it once the last session's workspaces are
// back, so a link's repository ends up in front.
func (w *Window) finishAttach() {
	w.pendingMu.Lock()
	w.attached = true
	pending := w.pending
	w.pending = nil
	w.pendingMu.Unlock()
	for _, req := range pending {
		w.OpenRepository(req.Repo, req.View, req.File)
	}
}
