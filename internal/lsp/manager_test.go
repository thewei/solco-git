package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thewei/solco-git/internal/lsp/lsptest"
)

func TestMain(m *testing.M) {
	if lsptest.Serve() {
		return
	}
	os.Exit(m.Run())
}

type sink struct {
	mu   sync.Mutex
	ds   map[string][]Diagnostic
	vers map[string]uint64
	n    int
}

func newSink() *sink { return &sink{ds: map[string][]Diagnostic{}, vers: map[string]uint64{}} }

func (s *sink) hooks() Hooks {
	return Hooks{
		Diagnostics: func(path string, version uint64, ds []Diagnostic) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.ds[path], s.vers[path] = ds, version
			s.n++
		},
		Changed: func() {},
	}
}

func (s *sink) get(path string) ([]Diagnostic, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ds[path], s.vers[path]
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// fakeManager is a Manager whose Go server is this test binary.
func fakeManager(t *testing.T, s *sink) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	log := lsptest.Env(t)
	m := NewManager(root, s.hooks(), func(sp Spec) []string {
		if sp.ID == "go" {
			return []string{os.Args[0]}
		}
		return nil
	})
	t.Cleanup(m.Close)
	return m, root, log
}

func calls(log string) []string {
	b, _ := os.ReadFile(log)
	return strings.Fields(string(b))
}

func TestDiagnosticsArriveAsByteOffsetsOfTheVersionTheyWereMadeFor(t *testing.T) {
	s := newSink()
	m, root, _ := fakeManager(t, s)
	path := filepath.Join(root, "a.go")
	// "é😀" is 2+4 bytes and 1+2 UTF-16 units: the server counts 3, the editor 6.
	text := "package a\n\né😀BAD\n"
	if !m.Sync(path, 7, text) {
		t.Fatal("a Go file should have a server")
	}
	waitFor(t, "diagnostics", func() bool { d, _ := s.get(path); return len(d) == 1 })
	d, v := s.get(path)
	if v != 7 {
		t.Fatalf("version = %d, want the one sent (7)", v)
	}
	want := strings.Index(text, "BAD")
	if d[0].Start != want || d[0].End != want+3 || d[0].Severity != SevError || d[0].Source != "fake" {
		t.Fatalf("diagnostic = %+v, want bytes %d–%d", d[0], want, want+3)
	}
}

func TestOneChangeReplacesTheDiagnosticsAndTheLogOrdersTheNotifications(t *testing.T) {
	s := newSink()
	m, root, log := fakeManager(t, s)
	path := filepath.Join(root, "a.go")
	m.Sync(path, 1, "BAD\n")
	waitFor(t, "first report", func() bool { d, _ := s.get(path); return len(d) == 1 })
	m.Sync(path, 2, "fine\n")
	waitFor(t, "report for v2", func() bool { _, v := s.get(path); return v == 2 })
	if d, _ := s.get(path); len(d) != 0 {
		t.Fatalf("after the fix: %+v", d)
	}
	m.Saved(path)
	waitFor(t, "save reaches the server", func() bool {
		return strings.Contains(strings.Join(calls(log), " "), "textDocument/didSave")
	})
	m.CloseDoc(path)
	waitFor(t, "close reaches the server", func() bool {
		return strings.Contains(strings.Join(calls(log), " "), "textDocument/didClose")
	})
	got := strings.Join(calls(log), " ")
	if !strings.HasPrefix(got, "initialize initialized textDocument/didOpen textDocument/didChange textDocument/didSave textDocument/didClose") {
		t.Fatalf("calls out of order: %s", got)
	}
}

func TestHoverAndDefinitionAnswerInEditorTerms(t *testing.T) {
	s := newSink()
	m, root, _ := fakeManager(t, s)
	path := filepath.Join(root, "a.go")
	text := "é😀 Do()\ncall NONE here\n"
	m.Sync(path, 3, text)
	ctx := context.Background()

	off := strings.Index(text, "Do") // byte offset; the server sees UTF-16 column 4
	txt, a, b, err := m.Hover(ctx, path, 3, off)
	if err != nil {
		t.Fatal(err)
	}
	if txt != "func Do()\n\nDoes the thing." { // fences dropped, paragraph kept
		t.Fatalf("hover text = %q", txt)
	}
	if a != off || b != off+3 {
		t.Fatalf("hover range = %d–%d, want %d–%d", a, b, off, off+3)
	}

	locs, err := m.Definition(ctx, path, 3, off)
	if err != nil || len(locs) != 1 {
		t.Fatalf("definition = %v, %v", locs, err)
	}
	if locs[0].Path != filepath.Join(root, "other.go") || locs[0].Range.Start != (Position{4, 2}) {
		t.Fatalf("location = %+v", locs[0])
	}
	none, err := m.Definition(ctx, path, 3, strings.Index(text, "NONE"))
	if err != nil || len(none) != 0 {
		t.Fatalf("a null answer is no locations: %v %v", none, err)
	}
}

func TestARequestForAnOlderVersionIsDroppedNotAnswered(t *testing.T) {
	s := newSink()
	m, root, _ := fakeManager(t, s)
	path := filepath.Join(root, "a.go")
	m.Sync(path, 1, "x\n")
	m.Sync(path, 2, "xy\n")
	if _, _, _, err := m.Hover(context.Background(), path, 1, 0); err != ErrStale {
		t.Fatalf("err = %v, want ErrStale", err)
	}
}

func TestMissingServerAndUnknownLanguageAreQuietNotErrors(t *testing.T) {
	s := newSink()
	m, root, _ := fakeManager(t, s)
	if m.Sync(filepath.Join(root, "a.rs"), 1, "fn main(){}") { // rust: not installed in this manager
		t.Fatal("no server for rust here: Sync must say so")
	}
	if got := m.Status(filepath.Join(root, "a.rs")); got.State != StateMissing {
		t.Fatalf("rust status = %+v, want missing", got)
	}
	if m.Sync(filepath.Join(root, "notes.txt"), 1, "hi") {
		t.Fatal("plain text has no server")
	}
	if got := m.Status(filepath.Join(root, "notes.txt")); got.State != StateNone {
		t.Fatalf("txt status = %+v", got)
	}
	if got := m.Status(filepath.Join(root, "a.go")); got.State != StateIdle {
		t.Fatalf("go before opening = %+v, want idle (installed, not started)", got)
	}
}

func TestACrashRestartsAndReplaysOpenDocumentsThenGivesUp(t *testing.T) {
	s := newSink()
	m, root, log := fakeManager(t, s)
	path := filepath.Join(root, "a.go")
	m.Sync(path, 1, "BAD\n")
	waitFor(t, "running", func() bool { return m.Status(path).State == StateRunning })

	// First crash: restarts, and the document is opened again on the new process.
	m.Sync(path, 2, "CRASH\n")
	waitFor(t, "second initialize", func() bool {
		n := 0
		for _, c := range calls(log) {
			if c == "initialize" {
				n++
			}
		}
		return n == 2
	})
	waitFor(t, "running again", func() bool { return m.Status(path).State == StateRunning })
	waitFor(t, "replayed document", func() bool { _, v := s.get(path); return v == 2 })

	// Keep crashing: at most three starts a minute, then it stays off with a note.
	for i := 0; i < 6 && m.Status(path).State != StateOff; i++ {
		m.Sync(path, uint64(10+i), "CRASH again\n")
		time.Sleep(300 * time.Millisecond)
	}
	waitFor(t, "left off", func() bool { return m.Status(path).State == StateOff })
	if st := m.Status(path); !strings.Contains(st.Note, "stopped repeatedly") {
		t.Fatalf("note = %q", st.Note)
	}
	if m.Sync(path, 99, "x\n") {
		t.Fatal("a server left off takes no more documents")
	}
}

func TestCloseEndsTheServerInOrder(t *testing.T) {
	s := newSink()
	m, root, log := fakeManager(t, s)
	m.Sync(filepath.Join(root, "a.go"), 1, "x\n")
	waitFor(t, "running", func() bool { return m.Status(filepath.Join(root, "a.go")).State == StateRunning })
	m.Close()
	got := strings.Join(calls(log), " ")
	if !strings.HasSuffix(got, "shutdown exit") {
		t.Fatalf("calls = %s, want a shutdown then exit", got)
	}
	if m.Sync(filepath.Join(root, "b.go"), 1, "x\n") {
		t.Fatal("a closed manager takes no documents")
	}
}

func TestColumnsConvertBothWaysAndClamp(t *testing.T) {
	line := "a😀é" // 1 + 4 + 2 bytes; 1 + 2 + 1 UTF-16 units
	for _, c := range []struct{ byteCol, utf int }{{0, 0}, {1, 1}, {5, 3}, {7, 4}} {
		if got := UTF16Col(line, c.byteCol); got != c.utf {
			t.Errorf("UTF16Col(%d) = %d, want %d", c.byteCol, got, c.utf)
		}
		if got := ByteCol(line, c.utf); got != c.byteCol {
			t.Errorf("ByteCol(%d) = %d, want %d", c.utf, got, c.byteCol)
		}
	}
	if ByteCol(line, 99) != len(line) || ByteCol(line, -3) != 0 {
		t.Error("columns clamp to the line")
	}
	if ByteCol(line, 2) != 5 { // inside the surrogate pair: after the character
		t.Errorf("a column inside a pair = %d", ByteCol(line, 2))
	}
}

func TestSpecForPicksByExtensionAndKnowsLanguageIDs(t *testing.T) {
	for file, want := range map[string][2]string{
		"main.go": {"go", "go"}, "a/b.TSX": {"typescript", "typescriptreact"}, "x.mjs": {"typescript", "javascript"},
		"lib.rs": {"rust", "rust"}, "t.hpp": {"clangd", "cpp"}, "App.swift": {"swift", "swift"},
	} {
		sp, id, ok := SpecFor(file)
		if !ok || sp.ID != want[0] || id != want[1] {
			t.Errorf("SpecFor(%q) = %s %s %v, want %v", file, sp.ID, id, ok, want)
		}
	}
	if _, _, ok := SpecFor("README.md"); ok {
		t.Error("Markdown has no server")
	}
}

func TestHoverTextReadsEveryShapeTheProtocolAllows(t *testing.T) {
	for in, want := range map[string]string{
		`"plain"`:                                "plain",
		`{"language":"go","value":"func F()"}`:   "func F()",
		`{"kind":"markdown","value":"**bold**"}`: "**bold**",
		`["a",{"language":"go","value":"b"}]`:    "a\n\nb",
		`null`:                                   "",
		"{\"kind\":\"markdown\",\"value\":\"```\\n\\n```\"}": "",
	} {
		if got := hoverText([]byte(in)); got != want {
			t.Errorf("hoverText(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestLocationsReadsLocationsAndLinks(t *testing.T) {
	one := `{"uri":"file:///a/b.go","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":5}}}`
	link := `[{"targetUri":"file:///c/d.go","targetRange":{"start":{"line":0,"character":0},"end":{"line":9,"character":0}},` +
		`"targetSelectionRange":{"start":{"line":3,"character":4},"end":{"line":3,"character":8}}}]`
	if l := locations([]byte(one)); len(l) != 1 || l[0].Path != "/a/b.go" || l[0].Range.Start.Line != 1 {
		t.Errorf("single = %+v", l)
	}
	if l := locations([]byte(link)); len(l) != 1 || l[0].Path != "/c/d.go" || l[0].Range.Start != (Position{3, 4}) {
		t.Errorf("link = %+v", l)
	}
	if l := locations([]byte(`[{"uri":"https://x/y","range":{}}]`)); len(l) != 0 {
		t.Errorf("a non-file URI is not a location: %+v", l)
	}
}
