package nativeui

/**
 * [INPUT]: internal/lsp 的 Manager（每个工作区一个，首次打开有服务器的语言的文件时才建）、editor.State 的 SetDiagnostics / OnHover / OnDefinition、files_native_preview.go 里打开的那一个缓冲区
 * [OUTPUT]: Files 里的语言服务器只读能力（2026-11-13 决定第 (4) 条，E4）：诊断（行内波浪线 + 页脚计数，点计数跳到下一个）、悬停、⌘点击 / F12 跳转定义（仓库内的落到 Files，仓库外的说一句）；页脚的服务器状态一行（没装 / 启动中 / 运行 / 反复崩溃后关闭）
 * [POS]: 缓冲区是真相：每帧比一次 Buffer.Version，变了就等 250 ms 静止再把整份文本交给 Manager（对账式，Manager 决定 open/change）；发给服务器的版本 = 缓冲区版本 + 本文档的基数（新缓冲区——换文件、从磁盘重载整个重建——基数抬高，LSP 版本因此永远递增）。结果带版本回来，过期的丢掉（SetDiagnostics 自己会把旧版本的诊断随编辑挪过去）。日志从不含路径、文本、诊断
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-git/internal/lsp"
)

// lspDebounce is how long typing must pause before the text goes to the
// server; tests shorten it.
var lspDebounce = 250 * time.Millisecond

// lspDocState is the open file as the language server knows it.
type lspDocState struct {
	path   string
	buf    *editor.Buffer
	base   uint64 // LSP version = base + buffer version
	synced uint64 // buffer version last sent (valid when sentOnce)
	sent   bool
	gen    int // bumped to cancel a pending debounce
	timer  *time.Timer

	errors, warnings, others int // from the last report that landed
	jump                     int // which problem the footer's next click goes to
}

// lspState is the workspace's language-server side.
type lspState struct {
	mgr  *lsp.Manager
	high uint64 // the highest LSP version ever sent, so versions only grow
	doc  lspDocState
	// rename is a rename the server answered, waiting on the unsaved-edits sheet;
	// renameAt is the caret it was asked at.
	rename   *lspRenameJob
	renameAt int
	// dropText is set by a key the editor was spared (⌥⇧F): macOS types the
	// key's Option character ("Ï") right after a taken keyDown, and that text
	// is dropped too.
	dropText bool
	// resolve, when set (tests), replaces the installed-server lookup.
	resolve func(lsp.Spec) []string
}

// manager is the workspace's lsp.Manager, made when first needed.
func (s *Shell) lspManager() *lsp.Manager {
	if s.lsp.mgr == nil {
		s.lsp.mgr = lsp.NewManager(s.dir, lsp.Hooks{
			Diagnostics: func(path string, version uint64, ds []lsp.Diagnostic) {
				s.applyOnUI(func() { s.lspApplyDiagnostics(path, version, ds) })
			},
			Changed: func() { s.applyOnUI(func() {}) }, // a frame, so the footer reads the new state
		}, s.lsp.resolve)
	}
	return s.lsp.mgr
}

// lspClose ends every server of the workspace (Shell.Close).
func (s *Shell) lspClose() {
	d := &s.lsp.doc
	d.gen++
	if d.timer != nil {
		d.timer.Stop()
	}
	if s.lsp.mgr != nil {
		go s.lsp.mgr.Close() // ordered shutdown can take a moment; the window must not wait
		s.lsp.mgr = nil
	}
}

// lspRelease tells the server the open file is no longer open here.
func (s *Shell) lspRelease() {
	d := &s.lsp.doc
	if d.path == "" {
		return
	}
	d.gen++
	if d.timer != nil {
		d.timer.Stop()
	}
	if s.lsp.mgr != nil {
		s.lsp.mgr.CloseDoc(d.path)
	}
	*d = lspDocState{}
}

// lspWanted: the open file has a server kind and is whole text.
func (s *Shell) lspWanted() bool {
	st := &s.rightPanel
	if st.nativeCode == nil || st.nativeCode.Buffer == nil || st.nativePreviewPath == "" || st.nativeText == nil || st.nativeText.Truncated {
		return false
	}
	_, _, ok := lsp.SpecFor(st.nativePreviewPath)
	return ok
}

// lspTick runs once a frame while the code view shows: it notices a new
// buffer or an edited one and schedules the sync.
func (s *Shell) lspTick() {
	st := &s.rightPanel
	d := &s.lsp.doc
	if !s.lspWanted() {
		s.lspRelease()
		return
	}
	buf := st.nativeCode.Buffer
	if d.path != st.nativePreviewPath || d.buf != buf {
		s.lspRelease()
		d.path, d.buf, d.base = st.nativePreviewPath, buf, s.lsp.high+1
		s.lspPush()
		return
	}
	if v := buf.Version(); !d.sent || v != d.synced {
		if d.timer == nil {
			gen := d.gen
			d.timer = time.AfterFunc(lspDebounce, func() {
				s.applyOnUI(func() {
					if s.lsp.doc.gen != gen {
						return
					}
					s.lsp.doc.timer = nil
					s.lspPush()
				})
			})
		}
	}
}

// lspPush hands the buffer's text, as it is now, to the server.
func (s *Shell) lspPush() {
	d := &s.lsp.doc
	if d.buf == nil || d.path == "" {
		return
	}
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	snap := d.buf.Snapshot()
	v := d.base + snap.Version()
	s.lsp.high = max(s.lsp.high, v)
	d.synced, d.sent = snap.Version(), true
	s.lspManager().Sync(d.path, v, snap.String())
}

// lspSaved is told after a successful write.
func (s *Shell) lspSaved() {
	d := &s.lsp.doc
	if d.path == "" || d.path != s.edit.path {
		return
	}
	s.lspPush() // the server must see the text that was written
	s.lspManager().Saved(d.path)
}

// lspApplyDiagnostics lands a report on the open buffer (UI lane).
func (s *Shell) lspApplyDiagnostics(path string, version uint64, ds []lsp.Diagnostic) {
	d := &s.lsp.doc
	st := &s.rightPanel
	if d.path != path || version < d.base || st.nativeCode == nil || st.nativeCode.Buffer != d.buf {
		return
	}
	out := make([]editor.Diagnostic, 0, len(ds))
	d.errors, d.warnings, d.others = 0, 0, 0
	for _, x := range ds {
		sev := editor.SeverityError
		switch x.Severity {
		case lsp.SevWarning:
			sev = editor.SeverityWarning
			d.warnings++
		case lsp.SevInfo:
			sev = editor.SeverityInfo
			d.others++
		case lsp.SevHint:
			sev = editor.SeverityHint
			d.others++
		default:
			d.errors++
		}
		out = append(out, editor.Diagnostic{Start: x.Start, End: x.End, Severity: sev, Message: x.Message, Source: x.Source})
	}
	st.nativeCode.SetDiagnostics(version-d.base, out)
	d.jump = 0
}

// lspBind wires the editor's hover and go-to-definition to the server. Both
// are asked on the UI lane and answered on it; the request itself runs off it.
func (s *Shell) lspBind(code *editor.State, path string) {
	code.OnHover = func(req editor.HoverRequest) {
		d := &s.lsp.doc
		if d.path != path || s.lsp.mgr == nil {
			return
		}
		mgr, ver, buf := s.lsp.mgr, d.base+req.Version, d.buf
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			text, start, end, err := mgr.Hover(ctx, path, ver, req.Offset)
			if err != nil || text == "" {
				return
			}
			s.applyOnUI(func() {
				if s.lsp.doc.buf != buf || buf.Version() != req.Version || s.rightPanel.nativeCode != code {
					return // the text moved on: the answer is about something else now
				}
				code.ShowHover(start, end, text, nil)
			})
		}()
	}
	code.OnCompletion = func(req editor.CompletionRequest) { s.lspComplete(code, path, req) }
	code.OnInput = s.lspInput
	code.OnDefinition = func(offset int) {
		d := &s.lsp.doc
		if d.path != path {
			return
		}
		if st := s.lspManager().Status(path); st.State == lsp.StateMissing {
			s.pendingToast = st.Name + ": no language server is installed, so there is no go to definition."
			return
		}
		mgr, buf, version := s.lspManager(), d.buf, buf0(d)
		ver := d.base + version
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			locs, err := mgr.Definition(ctx, path, ver, offset)
			s.applyOnUI(func() {
				if s.lsp.doc.buf != buf || buf.Version() != version {
					return
				}
				s.lspGoTo(locs, err)
			})
		}()
	}
}

// buf0 is the buffer's current version (the one a request is made for).
func buf0(d *lspDocState) uint64 {
	if d.buf == nil {
		return 0
	}
	return d.buf.Version()
}

// lspGoTo opens the first definition: inside the repository it lands in
// Files, anywhere else it says where it is.
func (s *Shell) lspGoTo(locs []lsp.Location, err error) {
	if err != nil {
		s.pendingToast = "Go to definition didn't get an answer."
		return
	}
	if len(locs) == 0 {
		s.pendingToast = "No definition found."
		return
	}
	loc := locs[0]
	line, col, length := lspTargetBytes(loc)
	if rel, ok := s.underRoot(loc.Path); ok {
		s.revealFileAt(filepath.Join(s.dir, rel), line, col, length)
		if len(locs) > 1 {
			s.pendingToast = itoa(len(locs)) + " definitions — showing the first."
		}
		return
	}
	s.pendingToast = "The definition is outside this repository: " + shortPath(loc.Path)
}

// lspTargetBytes turns a location's UTF-16 range into the byte line, column
// and length Files' reveal takes, by reading the target's own line.
func lspTargetBytes(loc lsp.Location) (line, col, length int) {
	line = loc.Range.Start.Line
	text := ""
	if b, err := os.ReadFile(loc.Path); err == nil {
		lines := strings.Split(string(b), "\n")
		if line >= 0 && line < len(lines) {
			text = strings.TrimSuffix(lines[line], "\r")
		}
	}
	col = lsp.ByteCol(text, loc.Range.Start.Character)
	length = 1
	if loc.Range.End.Line == loc.Range.Start.Line {
		length = max(1, lsp.ByteCol(text, loc.Range.End.Character)-col)
	}
	return line, col, length
}

// underRoot maps an absolute path from a server (which may have resolved
// symbolic links) to a path relative to the workspace root, when it is in it.
func (s *Shell) underRoot(abs string) (string, bool) {
	roots := []string{s.dir}
	if r, err := filepath.EvalSymlinks(s.dir); err == nil && r != s.dir {
		roots = append(roots, r)
	}
	// A path can come by a symlink the root does not name (a save panel's
	// /tmp/… for a repository opened as /private/tmp/…): try where it really
	// is too. The file itself may not exist yet, so resolve its folder.
	paths := []string{abs}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		if real := filepath.Join(dir, filepath.Base(abs)); real != abs {
			paths = append(paths, real)
		}
	}
	for _, root := range roots {
		for _, p := range paths {
			rel, err := filepath.Rel(root, p)
			if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return rel, true
			}
		}
	}
	return "", false
}

// shortPath is the last two elements of a path, enough to say which file.
func shortPath(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, "/")
}

// lspJump moves the caret to the next error (then warning) of the open file;
// each click goes on to the next.
func (s *Shell) lspJump() {
	code := s.rightPanel.nativeCode
	if code == nil {
		return
	}
	var list []editor.Diagnostic
	for _, sev := range []editor.Severity{editor.SeverityError, editor.SeverityWarning} {
		for _, x := range code.Diagnostics() {
			if x.Severity == sev {
				list = append(list, x)
			}
		}
	}
	if len(list) == 0 {
		return
	}
	d := &s.lsp.doc
	x := list[d.jump%len(list)]
	d.jump++
	code.GoTo(code.Buffer.Pos(x.Start))
	code.Select(x.Start, x.End)
}

// lspFoot is the server's one line in the code view's foot: what is running
// and how many problems there are.
func (s *Shell) lspFoot(c *ui.Context) {
	path := s.rightPanel.nativePreviewPath
	if path == "" || !s.lspWanted() {
		return
	}
	t := c.Theme()
	typ := Typography()
	tokens := designTokens(t.Dark)
	muted := t.TextMuted
	st := s.lspManager().Status(path)
	d := &s.lsp.doc
	dot := func(tone StatusTone) {
		ui.Box(c).Size(7, 7).Radius(3.5).Background(tokens.StatusColor(tone, t.Dark)).Shrink(0)
	}
	switch st.State {
	case lsp.StateMissing:
		tipped(c, ui.Text(c, st.Name+": no language server").FontSize(typ.Caption).TextColor(muted).SingleLine(),
			"Install the language server for "+st.Name+" to get problems, hover and go to definition (Settings lists what was found).")
	case lsp.StateIdle, lsp.StateStarting:
		ui.Text(c, st.Name+" starting…").FontSize(typ.Caption).TextColor(muted).SingleLine()
	case lsp.StateOff:
		tipped(c, ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			dot(ToneAttention)
			ui.Text(c, st.Note).FontSize(typ.Caption).TextColor(muted).SingleLine()
		}), "Reopen the workspace to try again.")
	case lsp.StateRunning:
		problems := d.errors + d.warnings
		tip := st.Name
		if problems > 0 {
			tip += " — click to go to the next problem"
		}
		k := gorexColorsOf(t.Dark)
		b := tipped(c, ui.ButtonBase(c.Key("lsp-problems")).Height(gorexPaneBtn).Padding(0, 6).
			Radius(iconButtonRadius(gorexPaneBtn)).AlignItems(ui.Center).FocusRing(false).Label(tip), tip)
		gorexToggleFace(b, k, false)
		b.Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).PassThrough().Children(func() {
				if problems == 0 && d.others == 0 {
					dot(ToneSuccess)
					ui.Text(c, "No problems").FontSize(typ.Caption).TextColor(muted).SingleLine()
					return
				}
				if d.errors > 0 {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						dot(ToneError)
						ui.Text(c, itoa(d.errors)).FontSize(typ.Caption).TextColor(t.Text)
					})
				}
				if d.warnings > 0 {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						dot(ToneWarning)
						ui.Text(c, itoa(d.warnings)).FontSize(typ.Caption).TextColor(t.Text)
					})
				}
			})
		})
		if b.Clicked() && problems > 0 {
			s.lspJump()
		}
	}
}

// languageServers is the installed-server list Settings shows. Finding them
// stats a dozen directories, so the answer is kept a few seconds; a test
// sets fixed.
type languageServers struct {
	fixed []lsp.Detected
	at    time.Time
	last  []lsp.Detected
}

func (l *languageServers) list() []lsp.Detected {
	if l.fixed != nil {
		return l.fixed
	}
	if l.last == nil || time.Since(l.at) > 5*time.Second {
		l.last, l.at = lsp.Detect(), time.Now()
	}
	return l.last
}

// languageServerNote says what was found, or what is looked for.
func languageServerNote(d lsp.Detected) string {
	if d.Command != nil {
		return d.Command[0]
	}
	var names []string
	for _, c := range d.Spec.Commands {
		names = append(names, c[0])
	}
	return "Looks for " + strings.Join(names, " or ") + " on your PATH."
}
