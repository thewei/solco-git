package lsp

/**
 * [INPUT]: 一个语言服务器的 stdio 子进程、sourcegraph/jsonrpc2（Content-Length 帧与请求配对，MIT，无传递依赖）
 * [OUTPUT]: client——initialize 握手、文档同步通知（open/change/save/close）、hover 与 definition 请求、服务器发来的请求的应答、publishDiagnostics 的转交、有序关闭
 * [POS]: 一个进程一个 client；不碰 UI、不碰文件系统（文本由 Manager 交来）。服务器的 stderr 丢弃——它可能带路径与代码，也从不记日志
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

const (
	initTimeout     = 45 * time.Second // rust-analyzer and gopls load the project first
	requestTimeout  = 8 * time.Second
	shutdownTimeout = 2 * time.Second
)

// Sync kinds a server declares for textDocument/didChange.
const (
	syncNone = 0
	// 1 = full, 2 = incremental: both accept a whole-document change.
)

// diagSink receives a server's diagnostics: the document version they are
// for (nil when the server does not say) and the raw items.
type diagSink func(path string, version *int, items []rawDiagnostic)

type client struct {
	conn     *jsonrpc2.Conn
	cmd      *exec.Cmd
	root     string
	syncKind int
	done     chan struct{} // closed when the connection ends (the server exited or crashed)
	closing  bool
}

type stdio struct {
	io.Reader
	io.WriteCloser
}

func (s stdio) Close() error { return s.WriteCloser.Close() }

// startClient launches argv in root and completes the initialize handshake.
func startClient(ctx context.Context, argv []string, root string, sink diagSink) (*client, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Env = childEnv()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &client{cmd: cmd, root: root, done: make(chan struct{})}
	handler := jsonrpc2.HandlerWithError(func(_ context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
		return c.handle(req, sink)
	})
	c.conn = jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(stdio{out, in}, jsonrpc2.VSCodeObjectCodec{}), handler)
	go func() {
		<-c.conn.DisconnectNotify()
		_ = cmd.Wait()
		close(c.done)
	}()

	ictx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	var res struct {
		Capabilities struct {
			TextDocumentSync json.RawMessage `json:"textDocumentSync"`
		} `json:"capabilities"`
	}
	params := map[string]any{
		"processId":        os.Getpid(),
		"clientInfo":       map[string]any{"name": "Solco Git"},
		"rootUri":          FileURI(root),
		"workspaceFolders": []map[string]any{{"uri": FileURI(root), "name": "workspace"}},
		"capabilities": map[string]any{
			"general": map[string]any{"positionEncodings": []string{"utf-16"}},
			"textDocument": map[string]any{
				"synchronization":    map[string]any{"didSave": true},
				"publishDiagnostics": map[string]any{"versionSupport": true},
				"hover":              map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
				"definition":         map[string]any{"linkSupport": true},
			},
			"workspace": map[string]any{"workspaceFolders": true, "configuration": true},
		},
	}
	if err := c.conn.Call(ictx, "initialize", params, &res); err != nil {
		c.kill()
		return nil, fmt.Errorf("initialize: %w", err)
	}
	c.syncKind = textDocumentSyncChange(res.Capabilities.TextDocumentSync)
	if err := c.conn.Notify(ctx, "initialized", map[string]any{}); err != nil {
		c.kill()
		return nil, err
	}
	return c, nil
}

// textDocumentSyncChange reads the sync kind off the server's capability,
// which is a number or an object with a "change" number. A server that says
// nothing is treated as wanting full text.
func textDocumentSyncChange(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 1
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var o struct {
		Change *int `json:"change"`
	}
	if json.Unmarshal(raw, &o) == nil && o.Change != nil {
		return *o.Change
	}
	return 1
}

// handle answers what a server sends: diagnostics are passed on, the few
// requests that need an answer to let the server go on get the plainest one.
func (c *client) handle(req *jsonrpc2.Request, sink diagSink) (any, error) {
	raw := json.RawMessage(nil)
	if req.Params != nil {
		raw = *req.Params
	}
	switch req.Method {
	case "textDocument/publishDiagnostics":
		var p struct {
			URI         string          `json:"uri"`
			Version     *int            `json:"version"`
			Diagnostics []rawDiagnostic `json:"diagnostics"`
		}
		if json.Unmarshal(raw, &p) == nil {
			if path, ok := URIPath(p.URI); ok && sink != nil {
				sink(path, p.Version, p.Diagnostics)
			}
		}
		return nil, nil
	case "workspace/configuration":
		var p struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(raw, &p)
		return make([]any, len(p.Items)), nil // no settings: every section is null
	case "workspace/workspaceFolders":
		return []map[string]any{{"uri": FileURI(c.root), "name": "workspace"}}, nil
	case "workspace/applyEdit":
		return map[string]any{"applied": false, "failureReason": "read-only client"}, nil
	case "window/workDoneProgress/create", "client/registerCapability", "client/unregisterCapability",
		"window/showMessageRequest", "window/showDocument":
		return nil, nil
	}
	if req.Notif {
		return nil, nil // $/progress, window/logMessage, telemetry …
	}
	return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: "method not supported"}
}

func (c *client) didOpen(path, languageID string, version int, text string) error {
	return c.conn.Notify(context.Background(), "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": FileURI(path), "languageId": languageID, "version": version, "text": text},
	})
}

func (c *client) didChange(path string, version int, text string) error {
	if c.syncKind == syncNone {
		return nil
	}
	return c.conn.Notify(context.Background(), "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": FileURI(path), "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

func (c *client) didSave(path string) error {
	return c.conn.Notify(context.Background(), "textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": FileURI(path)},
	})
}

func (c *client) didClose(path string) error {
	return c.conn.Notify(context.Background(), "textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": FileURI(path)},
	})
}

func (c *client) position(ctx context.Context, method, path string, pos Position, result any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return c.conn.Call(ctx, method, map[string]any{
		"textDocument": map[string]any{"uri": FileURI(path)},
		"position":     pos,
	}, result)
}

// hover returns the plain text under pos and the range it covers (nil when
// the server gives none); empty text means nothing to say.
func (c *client) hover(ctx context.Context, path string, pos Position) (string, *Range, error) {
	var res *struct {
		Contents json.RawMessage `json:"contents"`
		Range    *Range          `json:"range"`
	}
	if err := c.position(ctx, "textDocument/hover", path, pos, &res); err != nil {
		return "", nil, err
	}
	if res == nil {
		return "", nil, nil
	}
	return hoverText(res.Contents), res.Range, nil
}

func (c *client) definition(ctx context.Context, path string, pos Position) ([]Location, error) {
	var raw json.RawMessage
	if err := c.position(ctx, "textDocument/definition", path, pos, &raw); err != nil {
		return nil, err
	}
	return locations(raw), nil
}

// close ends the server in order — shutdown, exit — and kills it when it
// does not go.
func (c *client) close() {
	c.closing = true
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = c.conn.Call(ctx, "shutdown", nil, nil)
	_ = c.conn.Notify(ctx, "exit", nil)
	select {
	case <-c.done:
	case <-time.After(shutdownTimeout):
		c.kill()
	}
	_ = c.conn.Close()
}

func (c *client) kill() {
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.conn.Close()
}

// ErrNoServer is what a request gets when no server is running for the file.
var ErrNoServer = errors.New("no language server is running for this file")
