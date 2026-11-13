package lsptest

/**
 * [INPUT]: 测试二进制自己（LSP_FAKE=1 时变成服务器）
 * [OUTPUT]: Serve——一个说真实帧化 JSON-RPC 的迷你语言服务器；Env——让一个测试用它所在的二进制当 Go 服务器
 * [POS]: 供 internal/lsp 与 internal/nativeui 的测试共用；不会进入应用二进制（只有 _test 文件 import）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

// A language server in miniature: the test binary itself, run with
// LSP_FAKE=1. It speaks real framed JSON-RPC on stdio, so the client under
// test runs against the wire and not a mock.
//
//   - every line holding "BAD" is reported as an error over the word BAD
//     (positions in UTF-16, with the version of the text it was made from);
//   - hover answers a fenced block and a paragraph, over three characters;
//   - definition answers a location in other.go (line 4, columns 2–6), null when
//     the line says NONE, and a file outside any repository when it says OUTSIDE;
//   - a change that holds CRASH ends the process;
//   - every notification and request method is appended to LSP_FAKE_LOG.

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

type stdio struct {
	io.Reader
	io.WriteCloser
}

func (s stdio) Close() error { return s.WriteCloser.Close() }

// Env points the process at its own test binary as the language server and
// returns the file every call it receives is appended to. Call it from a test
// and give the manager a resolver that returns []string{os.Args[0]}.
func Env(t *testing.T) (log string) {
	t.Helper()
	log = filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("LSP_FAKE", "1")
	t.Setenv("LSP_FAKE_LOG", log)
	return log
}

// Serve runs the fake server when this process was started as one and reports
// whether it did; a TestMain calls it first and returns when it is true.
func Serve() bool {
	if os.Getenv("LSP_FAKE") != "1" {
		return false
	}
	runFake()
	return true
}

func runFake() {
	logf, _ := os.OpenFile(os.Getenv("LSP_FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	note := func(s string) {
		if logf != nil {
			logf.WriteString(s + "\n")
		}
	}
	texts := map[string]string{}
	var conn *jsonrpc2.Conn
	publish := func(uri string, version int) {
		var ds []map[string]any
		for i, line := range strings.Split(texts[uri], "\n") {
			if at := strings.Index(line, "BAD"); at >= 0 {
				col := 0
				for _, r := range line[:at] {
					col++
					if r >= 0x10000 {
						col++
					}
				}
				ds = append(ds, map[string]any{
					"range":    map[string]any{"start": map[string]any{"line": i, "character": col}, "end": map[string]any{"line": i, "character": col + 3}},
					"severity": 1, "message": "bad word", "source": "fake",
				})
			}
		}
		_ = conn.Notify(context.Background(), "textDocument/publishDiagnostics", map[string]any{"uri": uri, "version": version, "diagnostics": ds})
	}
	h := jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
		note(req.Method)
		var p struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
				Text    string `json:"text"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
			Position struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"position"`
		}
		if req.Params != nil {
			_ = json.Unmarshal(*req.Params, &p)
		}
		switch req.Method {
		case "initialize":
			return map[string]any{"capabilities": map[string]any{
				"textDocumentSync": map[string]any{"openClose": true, "change": 1}, "hoverProvider": true, "definitionProvider": true}}, nil
		case "textDocument/didOpen":
			texts[p.TextDocument.URI] = p.TextDocument.Text
			publish(p.TextDocument.URI, p.TextDocument.Version)
		case "textDocument/didChange":
			text := p.ContentChanges[0].Text
			if strings.Contains(text, "CRASH") {
				os.Exit(3)
			}
			texts[p.TextDocument.URI] = text
			publish(p.TextDocument.URI, p.TextDocument.Version)
		case "textDocument/hover":
			return map[string]any{
				"contents": map[string]any{"kind": "markdown", "value": "```go\nfunc Do()\n```\n\nDoes the thing."},
				"range":    map[string]any{"start": p.Position, "end": map[string]any{"line": p.Position.Line, "character": p.Position.Character + 3}},
			}, nil
		case "textDocument/definition":
			if strings.Contains(strings.Split(texts[p.TextDocument.URI], "\n")[p.Position.Line], "NONE") {
				return nil, nil
			}
			if strings.Contains(strings.Split(texts[p.TextDocument.URI], "\n")[p.Position.Line], "OUTSIDE") {
				return []map[string]any{{"uri": "file:///usr/lib/outside.go",
					"range": map[string]any{"start": map[string]any{"line": 0, "character": 0}, "end": map[string]any{"line": 0, "character": 1}}}}, nil
			}
			dir := p.TextDocument.URI[:strings.LastIndex(p.TextDocument.URI, "/")]
			return []map[string]any{{"uri": dir + "/other.go",
				"range": map[string]any{"start": map[string]any{"line": 4, "character": 2}, "end": map[string]any{"line": 4, "character": 6}}}}, nil
		case "shutdown":
			return nil, nil
		case "exit":
			os.Exit(0)
		}
		return nil, nil
	})
	conn = jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(stdio{os.Stdin, os.Stdout}, jsonrpc2.VSCodeObjectCodec{}), h)
	<-conn.DisconnectNotify()
}
