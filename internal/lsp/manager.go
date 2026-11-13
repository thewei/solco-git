package lsp

/**
 * [INPUT]: 一个工作区根、各 Spec 的已装命令、界面交来的文档（路径、版本、文本）
 * [OUTPUT]: Manager——每种语言至多一个服务器进程（首次打开该语言的文件时才起，随工作区结束）；对账式同步（界面只说"这个文档现在是这样"，服务器端的 open/change/close 由对账发出，崩溃重启后重放）；Hover / Definition；诊断换算成字节偏移后交给界面；Status / Close
 * [POS]: 每分钟最多重启 3 次，再坏就保持关闭并留一句话给状态栏（2026-11-13 决定第 (4) 条）。日志只有操作名、语言、计数、耗时、错误类别——从不记路径、文本、诊断、请求
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// State is what a server is doing, for the status line.
type State int

const (
	StateNone     State = iota // no server is known for this kind of file
	StateIdle                  // installed; starts when a file of the language is opened
	StateMissing               // known, but not installed
	StateStarting              // process launched, handshake not finished
	StateRunning
	StateOff // crashed too often; left off until the workspace is reopened
)

// Status is the server of one file, as the status line names it.
type Status struct {
	State State
	Name  string // "gopls", "rust-analyzer" … (the command, not the path)
	Note  string // set with StateOff
}

// Hooks are how a Manager tells the UI. Both run on Manager goroutines:
// the UI hops onto its own lane.
type Hooks struct {
	// Diagnostics: the problems of path at the version they were made for;
	// offsets are bytes into that version's text.
	Diagnostics func(path string, version uint64, ds []Diagnostic)
	// Changed: a server started, stopped or failed (the status line moves).
	Changed func()
}

const (
	maxStartsPerMinute = 3
	// MaxDocBytes is the biggest document sent to a server; a larger file is
	// simply not synced.
	MaxDocBytes = 2 << 20
)

type doc struct {
	langID  string
	version uint64
	index   *lineIndex
	saveSeq int
}

type slot struct {
	spec   Spec
	argv   []string // nil: not installed
	state  State
	note   string
	cl     *client
	starts []time.Time
	docs   map[string]*doc
	ready  chan struct{} // closed when cl is usable or the slot gave up

	sendMu sync.Mutex // serialises reconcile, so notifications keep their order
	sent   map[string]uint64
	saved  map[string]int
}

// Manager owns the language servers of one workspace.
type Manager struct {
	root    string
	hooks   Hooks
	resolve func(Spec) []string // Spec.Resolve; tests swap it

	mu     sync.Mutex
	slots  map[string]*slot
	closed bool
}

// NewManager makes a manager for a workspace root. Nothing starts until a
// document is opened. resolve finds a spec's installed command; nil means
// the user's PATH (tests pass their own).
func NewManager(root string, hooks Hooks, resolve func(Spec) []string) *Manager {
	if resolve == nil {
		resolve = Spec.Resolve
	}
	return &Manager{root: root, hooks: hooks, resolve: resolve, slots: map[string]*slot{}}
}

func (m *Manager) changed() {
	if m.hooks.Changed != nil {
		m.hooks.Changed()
	}
}

// slotFor finds or creates the slot of a spec. Called with m.mu held.
func (m *Manager) slotFor(sp Spec) *slot {
	if sl := m.slots[sp.ID]; sl != nil {
		return sl
	}
	sl := &slot{spec: sp, argv: m.resolve(sp), docs: map[string]*doc{},
		sent: map[string]uint64{}, saved: map[string]int{}}
	if sl.argv == nil {
		sl.state = StateMissing
	}
	m.slots[sp.ID] = sl
	return sl
}

// Status is the server state for a file.
func (m *Manager) Status(path string) Status {
	sp, _, ok := SpecFor(path)
	if !ok {
		return Status{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sl := m.slots[sp.ID]
	if sl == nil {
		// Never opened here: report what Open would find, without starting it.
		argv := m.resolve(sp)
		if argv == nil {
			return Status{State: StateMissing, Name: sp.Name}
		}
		return Status{State: StateIdle, Name: baseName(argv[0])}
	}
	return Status{State: sl.state, Name: commandName(sl), Note: sl.note}
}

func commandName(sl *slot) string {
	if len(sl.argv) > 0 {
		return baseName(sl.argv[0])
	}
	return sl.spec.Name
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// Sync says the document at path now reads text at version. The first call
// for a file opens it, later ones change it; a language with no known or
// installed server is left alone. It reports whether a server is, or is
// being, there for the file.
func (m *Manager) Sync(path string, version uint64, text string) bool {
	sp, langID, ok := SpecFor(path)
	if !ok || len(text) > MaxDocBytes {
		return false
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false
	}
	sl := m.slotFor(sp)
	if sl.argv == nil || sl.state == StateOff {
		m.mu.Unlock()
		return false
	}
	d := sl.docs[path]
	if d == nil {
		d = &doc{langID: langID}
		sl.docs[path] = d
	}
	d.version, d.index = version, newLineIndex(text)
	start := sl.cl == nil && sl.state != StateStarting
	if start {
		m.startLocked(sl)
	}
	m.mu.Unlock()
	m.poke(sl)
	return true
}

// Saved tells the server a file was written.
func (m *Manager) Saved(path string) {
	sp, _, ok := SpecFor(path)
	if !ok {
		return
	}
	m.mu.Lock()
	sl := m.slots[sp.ID]
	if sl != nil {
		if d := sl.docs[path]; d != nil {
			d.saveSeq++
		}
	}
	m.mu.Unlock()
	if sl != nil {
		m.poke(sl)
	}
}

// CloseDoc says the document is no longer open.
func (m *Manager) CloseDoc(path string) {
	sp, _, ok := SpecFor(path)
	if !ok {
		return
	}
	m.mu.Lock()
	sl := m.slots[sp.ID]
	if sl != nil {
		delete(sl.docs, path)
	}
	m.mu.Unlock()
	if sl != nil {
		m.poke(sl)
	}
}

func (m *Manager) poke(sl *slot) {
	go m.reconcile(sl)
}

// startLocked launches the slot's server in the background. m.mu is held.
func (m *Manager) startLocked(sl *slot) {
	now := time.Now()
	recent := sl.starts[:0]
	for _, t := range sl.starts {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	sl.starts = recent
	if len(sl.starts) >= maxStartsPerMinute {
		sl.state, sl.note = StateOff, commandName(sl)+" stopped repeatedly — left off for now"
		slog.Warn("language server left off", "language", sl.spec.ID)
		go m.changed()
		return
	}
	sl.starts = append(sl.starts, now)
	sl.state, sl.note = StateStarting, ""
	ready := make(chan struct{})
	sl.ready = ready
	argv := sl.argv
	go func() {
		began := time.Now()
		cl, err := startClient(context.Background(), argv, m.root, func(path string, ver *int, items []rawDiagnostic) {
			m.diagnostics(sl, path, ver, items)
		})
		m.mu.Lock()
		if err != nil || m.closed {
			if err == nil { // closed while starting
				m.mu.Unlock()
				cl.close()
				close(ready)
				return
			}
			slog.Warn("language server did not start", "language", sl.spec.ID, "err_class", errClass(err))
			sl.state, sl.note = StateOff, commandName(sl)+" could not start"
			sl.cl = nil
			m.mu.Unlock()
			close(ready)
			m.changed()
			return
		}
		sl.cl, sl.state = cl, StateRunning
		sl.sendMu.Lock()
		sl.sent, sl.saved = map[string]uint64{}, map[string]int{} // a new process knows nothing
		sl.sendMu.Unlock()
		m.mu.Unlock()
		slog.Info("language server started", "language", sl.spec.ID, "duration_ms", time.Since(began).Milliseconds())
		close(ready)
		m.changed()
		m.reconcile(sl)
		<-cl.done
		m.exited(sl, cl)
	}()
}

// exited handles a server going away: a deliberate close is nothing, a crash
// restarts it (within the limit) while documents are still open.
func (m *Manager) exited(sl *slot, cl *client) {
	m.mu.Lock()
	if sl.cl != cl || m.closed || cl.closing {
		m.mu.Unlock()
		return
	}
	sl.cl, sl.state = nil, StateStarting
	slog.Warn("language server exited", "language", sl.spec.ID)
	if len(sl.docs) == 0 {
		sl.state = StateNone
		m.mu.Unlock()
		m.changed()
		return
	}
	m.startLocked(sl)
	m.mu.Unlock()
	m.changed()
}

// reconcile makes the server's view of the documents the one the UI last
// described: open what it does not know, change what moved, close what is
// gone, tell it about saves. Sends run in order under sendMu.
func (m *Manager) reconcile(sl *slot) {
	sl.sendMu.Lock()
	defer sl.sendMu.Unlock()
	type step struct {
		kind    int // 0 open, 1 change, 2 close, 3 save
		path    string
		langID  string
		version uint64
		text    string
	}
	m.mu.Lock()
	cl := sl.cl
	if cl == nil {
		m.mu.Unlock()
		return
	}
	var steps []step
	for path, d := range sl.docs {
		v, known := sl.sent[path]
		switch {
		case !known:
			steps = append(steps, step{0, path, d.langID, d.version, d.index.text})
			sl.sent[path] = d.version
		case v != d.version:
			steps = append(steps, step{1, path, d.langID, d.version, d.index.text})
			sl.sent[path] = d.version
		}
		if d.saveSeq != sl.saved[path] {
			steps = append(steps, step{kind: 3, path: path})
			sl.saved[path] = d.saveSeq
		}
	}
	for path := range sl.sent {
		if _, ok := sl.docs[path]; !ok {
			steps = append(steps, step{kind: 2, path: path})
			delete(sl.sent, path)
			delete(sl.saved, path)
		}
	}
	m.mu.Unlock()
	for _, st := range steps {
		var err error
		switch st.kind {
		case 0:
			err = cl.didOpen(st.path, st.langID, int(st.version), st.text)
		case 1:
			err = cl.didChange(st.path, int(st.version), st.text)
		case 2:
			err = cl.didClose(st.path)
		case 3:
			err = cl.didSave(st.path)
		}
		if err != nil {
			return // the process is going away; exited() restarts and replays
		}
	}
}

// diagnostics converts a server's report for the version it was made at.
// Reports for a version the document has moved past are dropped: a newer
// one is on its way.
func (m *Manager) diagnostics(sl *slot, path string, ver *int, items []rawDiagnostic) {
	m.mu.Lock()
	d := sl.docs[path]
	if d == nil || (ver != nil && uint64(*ver) != d.version) {
		m.mu.Unlock()
		return
	}
	version, idx := d.version, d.index
	m.mu.Unlock()
	out := make([]Diagnostic, 0, len(items))
	for _, it := range items {
		start, end := idx.offset(it.Range.Start), idx.offset(it.Range.End)
		sev := it.Severity
		if sev < SevError || sev > SevHint {
			sev = SevError
		}
		out = append(out, Diagnostic{Start: start, End: max(end, start), Severity: sev, Message: it.Message, Source: it.Source})
	}
	if m.hooks.Diagnostics != nil {
		m.hooks.Diagnostics(path, version, out)
	}
}

// ErrStale: the document moved on between the request and the server's
// copy of it; the caller just drops the request.
var ErrStale = errors.New("the document changed")

// request waits for the file's server, brings it up to date with the
// document, and returns the document's index for the version asked about.
func (m *Manager) request(ctx context.Context, path string, version uint64) (*client, *lineIndex, error) {
	sp, _, ok := SpecFor(path)
	if !ok {
		return nil, nil, ErrNoServer
	}
	m.mu.Lock()
	sl := m.slots[sp.ID]
	var ready chan struct{}
	if sl != nil {
		ready = sl.ready
	}
	m.mu.Unlock()
	if sl == nil || ready == nil {
		return nil, nil, ErrNoServer
	}
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	m.reconcile(sl)
	m.mu.Lock()
	defer m.mu.Unlock()
	d := sl.docs[path]
	if sl.cl == nil || d == nil {
		return nil, nil, ErrNoServer
	}
	if d.version != version {
		return nil, nil, ErrStale
	}
	return sl.cl, d.index, nil
}

// Hover asks what is at byte offset of the document at version. The result
// is plain text and the byte range it covers (start == end when the server
// names none); empty text means nothing to say.
func (m *Manager) Hover(ctx context.Context, path string, version uint64, offset int) (text string, start, end int, err error) {
	cl, idx, err := m.request(ctx, path, version)
	if err != nil {
		return "", 0, 0, err
	}
	text, rng, err := cl.hover(ctx, path, idx.position(offset))
	if err != nil || text == "" {
		return "", 0, 0, err
	}
	start, end = offset, offset
	if rng != nil {
		start, end = idx.offset(rng.Start), idx.offset(rng.End)
	}
	return text, start, end, nil
}

// Definition asks where the symbol at byte offset is defined.
func (m *Manager) Definition(ctx context.Context, path string, version uint64, offset int) ([]Location, error) {
	cl, idx, err := m.request(ctx, path, version)
	if err != nil {
		return nil, err
	}
	return cl.definition(ctx, path, idx.position(offset))
}

// Close ends every server, in order, and refuses further work.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	var cls []*client
	for _, sl := range m.slots {
		if sl.cl != nil {
			cls = append(cls, sl.cl)
			sl.cl = nil
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, cl := range cls {
		wg.Add(1)
		go func() { defer wg.Done(); cl.close() }()
	}
	wg.Wait()
}

// errClass names an error's kind without its text (which can carry paths).
func errClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case err == nil:
		return ""
	}
	return "error"
}
