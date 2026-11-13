package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The real thing, when it is installed: a throwaway Go module, the user's own
// gopls. Skips itself otherwise (CI has none; SOLCOGIT_GOPLS names one).
func TestRealGoplsReportsHoversAndFindsDefinitions(t *testing.T) {
	bin := os.Getenv("SOLCOGIT_GOPLS")
	if bin == "" {
		bin = findExecutable("gopls")
	}
	if bin == "" {
		t.Skip("gopls is not installed")
	}
	if _, err := exec.LookPath("go"); err != nil && findExecutable("go") == "" {
		t.Skip("go is not installed")
	}
	root := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("go.mod", "module example.test/m\n\ngo 1.22\n")
	write("lib.go", "package m\n\n// Greet says hello to name.\nfunc Greet(name string) string { return \"hi \" + name }\n")
	text := "package m\n\nfunc use() int {\n\treturn Greet(\"é😀\")\n}\n"
	main := write("main.go", text)

	s := newSink()
	m := NewManager(root, s.hooks(), func(sp Spec) []string {
		if sp.ID == "go" {
			return []string{bin}
		}
		return nil
	})
	defer m.Close()
	if !m.Sync(main, 1, text) {
		t.Fatal("no server")
	}

	// Diagnostics: returning a string from a func that returns int.
	waitFor(t, "gopls diagnostics", func() bool { d, _ := s.get(main); return len(d) > 0 })
	d, v := s.get(main)
	if v != 1 || d[0].Severity != SevError || !strings.Contains(d[0].Message, "string") {
		t.Fatalf("diagnostics = %+v (version %d)", d, v)
	}
	if got := text[d[0].Start:d[0].End]; !strings.Contains(got, "Greet") {
		t.Fatalf("the squiggle covers %q, want the Greet call", got)
	}

	ctx := context.Background()
	at := strings.Index(text, "Greet") + 1
	hov, _, _, err := m.Hover(ctx, main, 1, at)
	if err != nil || !strings.Contains(hov, "func Greet(name string) string") || !strings.Contains(hov, "says hello") {
		t.Fatalf("hover = %q, %v", hov, err)
	}
	locs, err := m.Definition(ctx, main, 1, at)
	if err != nil || len(locs) != 1 {
		t.Fatalf("definition = %v, %v", locs, err)
	}
	// macOS temp dirs are symlinked (/var → /private/var): compare resolved.
	want, _ := filepath.EvalSymlinks(filepath.Join(root, "lib.go"))
	got, _ := filepath.EvalSymlinks(locs[0].Path)
	if got != want || locs[0].Range.Start.Line != 3 {
		t.Fatalf("definition at %+v, want lib.go line 3", locs[0])
	}
}
