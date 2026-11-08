# AGENTS.md

## Start here

1. `CLAUDE.md` — what is where, and the product and implementation rules.
2. `README.md` — scope, how to run and check, the `solcogit://` link, keys, data and logs, known limits.
3. The decision record: Solco's `docs/product-decisions.md`, "2026-11-07 · Solco Git and the right panel (T3 Code review, rounds 4 and 5)" (`rg -n 'Solco Git' ../solco/docs/product-decisions.md`). The design lives in Solco's `designs/git-files-app/`; a behaviour that differs from it is either a recorded limit (README) or a bug.
4. `internal/nativeui/CLAUDE.md` before touching presentation.

## Private assets (solco-docs)

`docs/` (the release runbook) is a symlink into the **private** sibling repository `../solco-docs` (its `solco-git/` folder), and so are the Solco documents named above (`../solco/docs/`, `../solco/designs/`): the development record is not in this repository, and `.gitignore` keeps `docs/` out of every commit here. The updater key pair lives in solco-docs `secrets` and in the CI secret, never here. The rulebook is the `solco-docs` skill in the Solco repository (`../solco/.agents/skills/solco-docs/SKILL.md`); these rules apply to every session:

- **Read.** A path that does not resolve means a missing link or checkout: `bash ../solco-docs/link.sh --check`, then `bash ../solco-docs/link.sh`. No `../solco-docs`: stop and tell the user; never recreate, stub or guess those files here.
- **Write and commit.** The release runbook and the design record are written through the links and committed in solco-docs (`bash ../solco/.agents/skills/solco-docs/scripts/commit-docs.sh -m "…" -- docs/…`); code, tests and rules are a separate commit here (`bash ../solco/.agents/skills/solco-commit/scripts/commit-paths.sh`). Two repositories, two commits, never `git add -A`; also check `git -C ../solco-docs status --short`.
- **Authorship.** Commit as the user's GitHub identity: both scripts set it, a hand-typed `git commit` passes it (the skill has name and address). Never a placeholder name, never the personal address from the global git config.
- **No leak back.** Nothing private goes into this repository's files, commit messages or CI: document text, design contents or evidence, keys, LAN addresses, home paths, user and machine names, personal mail. Those facts live in solco-docs only; a fixture uses a neutral value (`/Users/me`, user `deploy`, a placeholder subnet such as `192.168.1.x`), and `commit-paths.sh` refuses added lines that carry the real ones. `go vet`, `go test` and CI pass on a clone without solco-docs.
- **Publish.** Push only when asked, after the outbound scan and author check (`solco-commit`); solco-docs stays private, no force-push and no tag there. A release still follows `docs/releasing.md`.

## Checks

```sh
GOTOOLCHAIN=auto go vet ./...
GOTOOLCHAIN=auto go test ./...
gofmt -l .                       # prints nothing
GOTOOLCHAIN=auto go tool mygo build   # when packaging or the app entry changed
git diff --check
```

The tests run a real `git` binary against temporary repositories and render headless (`ui.Tester`); they never touch a repository you work in. A change that is visible gets a test that renders it; `SOLCOGIT_EVIDENCE=<dir>` keeps the PNGs for a design comparison (never commit them).

## Scope and ownership

- **No second runtime.** No Workspace, Pane, terminal, agent, Herdr, accounts, cloud or telemetry; nothing is imported from `solco`. The link from Solco is one way (`solcogit://open?repo=&view=&file=`, `external_open.go`).
- **Git only through `solco-gitkit`.** One runner (argv calls, timeouts, `GIT_TERMINAL_PROMPT=0`, per-repository serialization). Never force push, never `git clean`; the runner never fetches on its own — only Fetch and the user-set Auto-fetch interval do. A Git capability that is missing is added to `solco-gitkit` with a real-repository test first.
- **Presentation in `internal/nativeui`.** Reuse the kit (`theme.go`, `design_system.go`, `ds_components.go`, `components.go`, `icons.go`, `tooltip.go`) before styling a one-off; a visual property comes from a token or a shared component default, never an inline override on a feature component (a narrow segment is the named variant `segmentFacePadded`, not a padding override). The sidebar draws nothing of its own — the window's vibrancy shows through — so a control placed on it wears the sidebar's washes (`materialFace` and its `materialSegmentedTrack` / `materialSegmentFacePadded` / `materialTabStrip` / `materialTab`), never the opaque `segmentedTrack` paper that belongs on the canvas.
- **The window owns the workspaces; a workspace is a Shell.** "Workspace" is this app's own list of repositories (one repository each), switched from the pull-down in the sidebar's foot (in the title band while the sidebar is shut) and managed in Settings; it has nothing to do with Herdr's workspaces. The same repository root never opens twice (opening it again selects its workspace); every way of opening a repository (⌘O, Quick Launch, a dropped folder, a `solcogit://` link, `open -a`, Recent) adds the workspace if it is new and shows it. A workspace with a commit draft or a running operation asks before it is removed from the list. Background results reach the UI only through the Shell's `applyOnUI` lane (dropped when the workspace is gone).
- **Preferences are `internal/settings` only** (`settings.json`): paths and switches. No credentials, commit messages, diffs or file contents; a new field is a conscious decision (`TestDocumentKeysAreTheKnownOnes`).
- **Logs** are structured and bounded: operation names, counts, errors, timing. Never repository paths, commit messages, diffs, file contents, credentials, or the parameters of a `solcogit://` link — at the default level; the opt-in `SOLCOGIT_LOG_LEVEL=debug` adds one `gitworkbench` line that names the repository root of a snapshot refresh (`log_privacy_test.go` pins the default level).
- **The kit is a copy.** `scripts/kit-parity.sh` shows drift against `../solco/internal/nativeui`; a kit fix that Solco also needs is made in both, or the drift is named in the commit message.
- Real-window checks run against throwaway repositories created for the check, never the repositories in use.

## Git safety

Do not stage, commit, tag, push, reset or rewrite Git state unless asked. `go.work` is machine-local and stays untracked. Publishing follows `docs/releasing.md`: a gitkit change is tagged in `solco-gitkit` first, then taken here (and in `solco`) with `GOWORK=off go get` + `go mod tidy`; the updater secret key never enters the repository.
