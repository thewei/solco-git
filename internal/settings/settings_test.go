package settings

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func sample() Settings {
	s := Default()
	s.General.Appearance = "dark"
	s.General.Density = "compact"
	s.General.HighContrast = true
	s.Workbench.AutoFetchMinutes = 15
	s.Workbench.SplitDiff = true
	s.Workbench.GraphHidden = map[string][]string{"/r/app": {"refs/heads/old"}}
	s.Workbench.TreeFolds = map[string]map[string]bool{"/r/app": {"Tags": true}}
	s.Window.Workspaces = []WorkspaceSettings{{Root: "/r/app", View: "files"}, {Root: "/r/tool", View: "commits"}}
	s.Window.Active = 1
	s.Window.Recent = []string{"/r/tool", "/r/app"}
	s.Window.SidebarWidth = 280
	return s
}

func TestFileStoreRoundTripsTheWholeDocument(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	want := sample()
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaded %+v\nwant   %+v", got, want)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("settings file mode = %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Errorf("the directory holds %d entries (a temp file was left behind?)", len(entries))
	}
}

func TestMissingFileIsAFirstLaunch(t *testing.T) {
	got, err := NewFileStore(t.TempDir()).Load(context.Background())
	if err != nil || !reflect.DeepEqual(got, Default()) {
		t.Errorf("a missing file = %+v, %v; want the defaults", got, err)
	}
	if got.General.Appearance != "system" || !got.General.RestoreWindow {
		t.Errorf("defaults follow the system appearance and restore the window: %+v", got.General)
	}
}

func TestUnreadableDocumentsFallBackToDefaultsWithAnError(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	for name, content := range map[string]string{
		"corrupt":      "{not json",
		"newer schema": `{"schema_version": 99}`,
	} {
		if err := os.WriteFile(store.Path(), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(context.Background())
		if err == nil {
			t.Errorf("%s: want an error so the app can log it", name)
		}
		if !reflect.DeepEqual(got, Default()) {
			t.Errorf("%s: want the defaults, got %+v", name, got)
		}
	}
}

func TestAnOlderDocumentWithoutAVersionIsAccepted(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := os.WriteFile(store.Path(), []byte(`{"general":{"appearance":"light"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil || got.SchemaVersion != CurrentSchemaVersion || got.General.Appearance != "light" {
		t.Errorf("loaded %+v, %v", got, err)
	}
}

// The "Rich file preview" switch (general.webview_files_preview) was retired
// on 2026-11-08 with the Files page: an older document that still carries it
// loads, and the next save writes it no more.
func TestARetiredPreviewSwitchLoadsAndIsNotWrittenBack(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := os.WriteFile(store.Path(), []byte(`{"general":{"appearance":"dark","webview_files_preview":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil || got.General.Appearance != "dark" {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	if err := store.Save(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "webview_files_preview") {
		t.Errorf("the retired key is written back:\n%s", data)
	}
}

func TestSaveRefusesAForeignSchema(t *testing.T) {
	store := NewFileStore(t.TempDir())
	bad := Default()
	bad.SchemaVersion = CurrentSchemaVersion + 1
	if err := store.Save(context.Background(), bad); err == nil {
		t.Error("saving a schema this build does not know must be refused")
	}
	if _, err := os.Stat(store.Path()); err == nil {
		t.Error("a refused save must not leave a file")
	}
}

func TestACancelledContextTouchesNothing(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Save(ctx, sample()); err == nil {
		t.Error("save on a cancelled context must fail")
	}
	if _, err := store.Load(ctx); err == nil {
		t.Error("load on a cancelled context must fail")
	}
	if _, err := os.Stat(store.Path()); err == nil {
		t.Error("nothing is written")
	}
}

// The document is paths and switches. Its keys are listed here so that a new
// field is a conscious decision — nothing that is a credential, a commit
// message, a diff or a file's content belongs in it.
func TestDocumentKeysAreTheKnownOnes(t *testing.T) {
	keys := map[string]bool{}
	var walk func(prefix string, typ reflect.Type)
	walk = func(prefix string, typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			keys[path] = true
			ft := field.Type
			if ft.Kind() == reflect.Slice {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				walk(path, ft)
			}
		}
	}
	walk("", reflect.TypeOf(Settings{}))
	want := []string{
		"schema_version",
		"general", "general.appearance", "general.restore_window", "general.density", "general.high_contrast",
		"workbench", "workbench.auto_fetch_minutes", "workbench.split_diff", "workbench.graph_hidden",
		"workbench.graph_folded", "workbench.tree_folds", "workbench.hide_stashes", "workbench.hide_moved_lines",
		"workbench.terminal_in_solco",
		"window", "window.workspaces", "window.workspaces.root", "window.workspaces.view",
		"window.tabs", "window.tabs.root", "window.tabs.view", "window.active", "window.recent",
		"window.sidebar_width",
	}
	for _, key := range want {
		if !keys[key] {
			t.Errorf("expected key %q is gone", key)
		}
		delete(keys, key)
	}
	for key := range keys {
		t.Errorf("unexpected settings key %q: decide that it belongs in the document, then list it here", key)
	}
}

func TestMemoryStoreHoldsTheLatestDocument(t *testing.T) {
	store := NewMemoryStore(Default())
	if err := store.Save(context.Background(), sample()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil || !reflect.DeepEqual(got, sample()) {
		t.Errorf("memory store = %+v, %v", got, err)
	}
}

// A document from the tab-strip era keeps its repositories: Migrate reads the
// old "tabs" list as the workspaces, once, and a save then writes the new key
// only. A document that already has workspaces is not overridden.
func TestMigrateFoldsTheTabListIntoWorkspaces(t *testing.T) {
	store := NewFileStore(t.TempDir())
	old := `{"window":{"tabs":[{"root":"/r/app","view":"files"},{"root":"/r/tool"}],"active":1}}`
	if err := os.WriteFile(store.Path(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got.Window.Migrate()
	want := []WorkspaceSettings{{Root: "/r/app", View: "files"}, {Root: "/r/tool"}}
	if !reflect.DeepEqual(got.Window.Workspaces, want) || got.Window.Tabs != nil || got.Window.Active != 1 {
		t.Fatalf("migrated window = %+v, want workspaces %+v, no tabs, active 1", got.Window, want)
	}
	got.Window.Migrate() // idempotent
	if !reflect.DeepEqual(got.Window.Workspaces, want) {
		t.Fatalf("a second Migrate changed the list: %+v", got.Window.Workspaces)
	}
	if err := store.Save(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(store.Path())
	if strings.Contains(string(data), `"tabs"`) || !strings.Contains(string(data), `"workspaces"`) {
		t.Fatalf("a save must write the workspaces key only:\n%s", data)
	}

	both := WindowSettings{
		Workspaces: []WorkspaceSettings{{Root: "/r/new"}},
		Tabs:       []WorkspaceSettings{{Root: "/r/old"}},
	}
	both.Migrate()
	if len(both.Workspaces) != 1 || both.Workspaces[0].Root != "/r/new" || both.Tabs != nil {
		t.Fatalf("existing workspaces must win over the legacy list: %+v", both)
	}
}
