# Reimplement Stacky CLI in Go

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

## Purpose / Big Picture

The goal is to replace the existing Python implementation of the `stacky` command-line tool with a Go implementation that preserves the current user experience for managing stacks of Git branches and GitHub pull requests. After this rewrite, contributors will be able to run a single statically compiled Go binary that offers the same commands (`stacky info`, `stacky stack push`, `stacky inbox`, and so on), honors existing configuration files, and interoperates with git repositories and the GitHub CLI exactly as today. Success means a user can clone a repository with stacked branches, run the Go binary, and observe identical outputs, prompts, and behaviors to the Python tool, with tests covering the new implementation.

## Progress

- [x] Audit the Python CLI to catalog every command, flag, config knob, external dependency, and file it touches, recording the results in this plan (see “Python CLI Audit – 2024-04-07”).
- [x] Design and scaffold the Go module layout, Cobra command tree, and supporting packages for configuration, git access, GitHub integration, and terminal rendering (see “Go Implementation Blueprint – 2024-04-07”).
- [x] Implement core domain packages (stack graph model, repository helpers, config loader, and state persistence) and cover them with unit tests using `testify` (config loader, git repository helpers, stack graph builder, and state persistence implemented with `require`-based tests).
- [x] Port interactive and non-interactive commands (`info`, `stack sync`, `branch` subcommands, commit helpers, and tree printers) to Go, using `go-git` and `lipgloss` where appropriate (`info` scopes render via lipgloss; checkout/commit flows handle CLI flags and interactive prompts; `stacky log`, `stacky continue`, `stacky update`, `stacky stack sync`, `stacky stack push`, and `--pr` annotations go through engine helpers; branch selection menus now share a reusable TUI helper mirroring the Python TerminalMenu flow).
  - 2024-04-07 – Ported `stacky adopt` to Go (engine + Cobra command) with regression tests covering config toggles and stack-parent wiring.
  - 2025-09-27 – Ported `stacky prs` to Go with interactive selection, default editor integration, and regression tests covering menu flows and PR body updates.
  - 2025-09-28 – Ported `stacky land` to Go with engine planning/merge execution, CLI confirmation flow, and tests covering GitHub merge invocation and remote sync validation.
  - 2025-09-29 – Implemented `stacky upstack onto`/`stacky upstack as` in Go, wiring CLI commands to engine reparenting logic with regression tests for restack validation and stack-bottom promotion.
  - 2025-10-02 – Ported `stacky update` to Go with plan/execute workflow, branch deletion parity, confirmation prompts, and engine tests covering merged PR cleanup.
  - 2025-10-05 – Aligned `stacky push --pr` with Python by updating stack comment automation in Go, generating structured PR body summaries for pushed stacks and covering the behavior with engine tests.
  - 2025-10-13 – Consolidated branch-selection prompts around `internal/tui.Select`, enforcing terminal detection and reusing the interactive menu across checkout flows.
- 2025-09-27 – Added fold resume support to Engine/Continue, covering cherry-pick and merge folds with regression tests.
- 2025-10-06 – Persisted stack sync progress to state and taught `stacky continue` to resume rebases/merges after conflicts, bringing Go parity with Python’s sync resume flow.
- [x] Implement GitHub-centric functionality (`inbox`, `prs`, PR creation during `push`) in Go, including wrappers over the `gh` CLI and accompanying tests or fakes.
  - 2025-10-09 – Ported `stacky inbox` and `stacky prs` to Go with lipgloss formatting, `gh` CLI wrappers, and regression coverage for PR creation during push.
- [x] Add parity and regression tests plus end-to-end smoke scripts to ensure the Go binary matches Python behavior on representative repository fixtures.
  - 2025-09-30 – Added an initial CLI smoke test for `stacky stack info` in `internal/e2e`, including helpers to invoke the Cobra root command against temporary git fixtures.
  - 2025-10-01 – Added `TestStackSyncRebasesStack` end-to-end test covering `stacky stack sync` rebase flows and parent ref updates.
  - 2025-10-08 – Added `TestStackPushPushesBranches` end-to-end test verifying `stacky stack push --no-pr --force` pushes stack branches to a bare remote and reports the planned actions.
  - 2025-10-09 – Added `TestStackPushCreatesPullRequests` end-to-end test that stubs the `gh` binary to assert PR creation flow during `stacky stack push --force`.
  - 2025-10-10 – Added `TestContinueResumesStackSyncAfterConflict` end-to-end test to verify `stacky continue` resumes a rebasing stack after conflicts.
  - 2025-10-11 – Added `TestStackUpdateDeletesMergedBranch` end-to-end test covering `stacky update --force` branch deletion, reparenting, and bottom fast-forward parity.
  - 2025-10-14 – Added `TestLogRespectsUseMergeConfig` end-to-end test asserting `stacky log` matches `git log` output and honors the `[GIT].use_merge` configuration toggle.
  - 2025-10-12 – Added `TestLandMergesPullRequest` end-to-end test validating `stacky land` merges the bottom branch via stubbed gh interactions and prompts for confirmation output.
  - 2025-10-14 – Added `TestAdoptWiresExistingBranchIntoStack` end-to-end test to cover adopting existing branches into a stack and verifying git config wiring.
  - 2025-10-15 – Added `TestInboxRendersAuthoredAndReviewSections` end-to-end test covering `stacky inbox --compact` rendering with stubbed gh responses for authored and review queues.
  - 2025-10-16 – Added `TestPrsAllowsEditingPullRequest` end-to-end test validating interactive PR selection and description editing through the fake gh CLI and editor hooks.
  - 2025-10-17 – Added `TestFoldCherryPicksCommitsAndDeletesBranch` end-to-end test validating `stacky fold` cherry-picks, deletes the folded branch, and clears persisted state.
  - 2025-10-18 – Added `TestUpstackOntoRestacksCurrentBranch` and `TestUpstackAsPromotesBranchToBottom` end-to-end tests covering `stacky upstack onto` and `stacky upstack as bottom` restack flows.
  - 2025-10-19 – Added `TestDownstackInfoShowsAncestorsPath` end-to-end test verifying `stacky downstack info` renders the ancestor path without including sibling branches.
  - 2025-10-20 – Added `TestImportSetsParentRefsAndConfig` end-to-end test ensuring `stacky import` wires parent refs and git config from Graphite PR metadata.
  - 2025-10-21 – Added `TestStackCheckoutInteractiveSelection` end-to-end test verifying interactive stack checkout selects the chosen branch and renders the prompt.
  - 2025-10-22 – Added `TestBranchCommitCreatesBranchAndCommitsChanges` end-to-end test verifying `stacky branch commit --add-all` creates the branch, commits changes, and records the parent ref.
  - 2025-09-27 – Added `TestDownMovesToParentBranch`, `TestDownFailsAtBottom`, `TestUpMovesToChildBranch`, and `TestUpFailsWithoutChildren` end-to-end tests covering root navigation commands.
  - 2025-10-23 – Added `tools/smoke.sh` with a Bazel `//:smoke_tests` wrapper to run curated internal/e2e flows as an end-to-end smoke check.
- [x] Replace packaging, entry points, and documentation so the Go binary becomes the primary deliverable while leaving migration notes for any remaining Python components.
  - 2025-10-07 – Added a Bazel `genrule` and `sh_test` that build and exercise the Go CLI, rewrote README installation guidance to centre the Go binary, renamed Python Bazel targets to `legacy_*`, and introduced a runtime deprecation warning for the Python entry point.
  - 2025-10-24 – Updated GitHub Actions workflows to run Go tests, execute Bazel wrappers against the Go binary, and drop Python packaging from CI.
  - 2025-10-24 – Added `tools/package_release.sh` plus README guidance so maintainers can build cross-platform Go release artifacts and checksums locally.

## Surprises & Discoveries

- 2024-04-07 – Local git configuration enables `core.fsmonitor`, which prints `error: daemon terminated` during status checks and tests.
  Mitigation: disable fsmonitor inside test repositories until the engine handles external fsmonitor hooks gracefully.
- 2025-10-07 – Running `bazel` inside the sandboxed environment fails because the dotslash wrapper cannot create directories under `~/Library/Caches`.
  Mitigation: rely on `go build` / `go test` locally, or configure the wrapper to use a writable cache directory before invoking the Bazel targets.

## Decision Log

- Decision: Shell out to the system `git` binary in `internal/gitstore` instead of importing `go-git` for now.
  Rationale: Network restrictions prevent fetching new modules; using `git` keeps progress moving while matching the legacy behaviour, and the abstraction makes it easy to swap in `go-git` later.
  Date/Author: 2024-04-07 / Codex
- Decision: Wrap the `gh` CLI via an injectable runner rather than importing GitHub SDKs.
  Rationale: Maintains parity with the Python tool, avoids new dependencies under network restrictions, and keeps tests hermetic by faking command output.
  Date/Author: 2024-04-07 / Codex

- Decision: Build the Go binary through a Bazel `genrule` that shells out to `go build` instead of introducing `rules_go`.
  Rationale: Existing sandbox and network restrictions prevent downloading additional Bazel rulesets; invoking the Go toolchain directly keeps packaging reproducible with the dependencies already vendored locally.
  Date/Author: 2025-10-07 / Codex

## Outcomes & Retrospective

This section will be completed once significant milestones are reached, contrasting the delivered Go CLI with the Purpose section, noting any gaps, and highlighting lessons learned for future refactors.

## Context and Orientation

The current CLI lives in `src/stacky/stacky.py` and orchestrates git and GitHub operations by spawning `git`, `gh`, and helper utilities. It stores persistent state in `~/.stacky.state`, reads configuration from `~/.stackyconfig` and `<repo>/.stackyconfig`, uses `colors` for ANSI output, and relies on `argcomplete`, `asciitree`, and `simple_term_menu` for parsing, tree rendering, and interactive menus. Tests reside in `src/stacky/stacky_test.py`. Packaging metadata is defined in `setup.py`, `pyproject.toml`, and Bazel build files. `stacky` models a stack as a series of git branches linked by parent-child relationships, tracks parent commits through refs under `refs/stack-parent/`, and marks stack bottoms under `refs/stacky-bottom-branch/`. Commands fall into categories: informational (`info`, `stack info`), structural (`branch new`, `fold`), synchronization (`stack sync`, `continue`, `update`), GitHub integration (`push`, `prs`, `inbox`, `land`), and workflow helpers (`commit`, `amend`, `checkout`). The rewrite must preserve these behaviors while translating shell interactions into Go code that uses `go-git` for repository access, `cobra` for CLI parsing, and `lipgloss` for styling. We assume Go 1.21 or newer is available, and that the environment already has the `gh` binary configured with credentials. Terms: a "stack" is the ordered set of related branches; "upstack" refers to descendants of the current branch; "downstack" refers to ancestors toward the base (usually `master` or `main`).

## Python CLI Audit – 2024-04-07

The Python entry point builds an `argparse` tree with three global options: `--log-level` (values map to Python’s logging levels), `--color` (forces ANSI colouring on or off), and `--remote-name`/`-r` (sets the git remote used by push-like commands). The CLI registers subcommands for every workflow surfaced in the README. `continue` resumes a previously interrupted sync or fold by deserialising `~/.stacky.state`; `down` and `up` are shortcuts for moving the working tree one branch toward or away from the stack base; `info` renders the entire forest of stacks and optionally augments each branch with live PR metadata fetched from GitHub. `log` shells to `git log`, suppressing merges when `StackyConfig.use_merge` is true to mirror the user’s chosen sync style. Root-level `commit` and `amend` wrap `git commit`, enforce that the branch is synced with its parent, and then rebalance the upstack via `do_sync`.

The `branch` group (aliases `b`) exposes navigation and creation helpers: `branch up|down` reuse the same movement logic as the shortcuts, `branch new|create` checks out a tracking branch and writes `refs/stack-parent/<child>` to the parent’s current commit, `branch commit` combines new-branch creation with an immediate commit, and `branch checkout` optionally opens a terminal menu to pick any branch from the stack forest. The standalone `checkout` and `sco` commands mirror these behaviours at the root, with `sco` limiting choices to the current stack.

The `stack`, `upstack`, and `downstack` families wrap the same primitives around different branch subsets. Their `info` verbs delegate to `print_forest`, `push` plans pushes for each branch that is out of sync with its remote and, when PR creation is enabled, either edits existing PR bases or calls `gh pr create` via `create_gh_pr`. Sync verbs (`stack sync`, `upstack sync`, `downstack sync`) compute a depth-first traversal, then call `inner_do_sync`, which rebases or merges depending on `StackyConfig.use_merge`. `upstack onto` restacks the current branch and its descendants onto a new parent, while `upstack as` re-marks a branch as a new stack bottom. All of these flows persist progress to `~/.stacky.state` via an atomic temp file so `stacky continue` can recover from conflicts.

Lifecycle helpers extend the model. `update` fetches from the remote, fast-forwards every stack bottom to match its remote tracking branch, detects merged PRs beneath those bases, and deletes local branches plus their `refs/stack-parent/*` entries after user confirmation. `import` walks Graphite-generated PR chains (using `gh pr list --json commits`) to recreate parent relationships and ref markers locally. `adopt` attaches an existing branch onto the current stack bottom after validating ancestry. `land` merges the bottom-most PR in the active stack via `gh pr merge` with `--match-head-commit`, enforcing that the branch is pushed and mergeable, and optionally auto-merging when requested.

Interactive GitHub tooling appears in `inbox` and `prs`. Both commands query GitHub via the `gh` CLI for authored PRs and review requests, partition the results into “waiting on you”, “waiting on review”, “approved”, and “awaiting your review”, and colourise display with the `colors` package. `inbox --compact` renders one-line, hyperlink-friendly entries, while the default view shows checklist status derived from `statusCheckRollup`. `prs` requires an interactive terminal; it offers a simple selection menu through `simple_term_menu.TerminalMenu`, launches the user’s `$EDITOR` to tweak a PR body in a temp file, and applies the edits through `gh pr edit`.

State management underpins every command. `StackyConfig` merges options from `~/.stackyconfig` and `<repo>/.stackyconfig`, exposing toggles for confirmation prompts (`skip_confirm`), automatic checkout of the main branch (`change_to_main`), adopt behaviour (`change_to_adopted`), SSH control master reuse (`share_ssh_session`), sync strategy (`use_merge`), force pushing (`use_force_push`), and compact PR display (`compact_pr_display`). Persistent state lives in `~/.stacky.state` (with a `*.tmp` helper for atomic writes) and encodes the active branch, pending sync queue, or fold parameters. The tool maintains custom refs `refs/stack-parent/<branch>` to remember each child’s last-known parent commit and marks base branches under `refs/stacky-bottom-branch/<branch>`.

External execution is concentrated in helper functions `run`, `run_multiline`, and `run_always_return`, which wrap `subprocess.run` to call `git`, `gh`, and occasionally `ssh` commands. `start_muxed_ssh` and `stop_muxed_ssh` manage shared SSH control sockets when `share_ssh_session` is enabled. Other Python-only dependencies include `argcomplete` for shell completion, `asciitree` for tree printing, `colors` for formatting, and `simple_term_menu` for interactive menus. Outside the user’s home directory the program writes only git refs within the current repository’s `.git` directory and relies on `git` hooks (`pre-push`) when present.

## Go Implementation Blueprint – 2024-04-07

The Go rewrite will live at the repository root with module path `github.com/pmenglund/stacky` and Go 1.21 as the target toolchain. The `cmd` tree hosts the binary entry point: `main.go` wires `Execute()` on a root Cobra command defined in `cmd/stacky/root.go`, while dedicated files such as `cmd/stacky/branch.go`, `cmd/stacky/stack.go`, `cmd/stacky/upstack.go`, `cmd/stacky/downstack.go`, `cmd/stacky/github.go`, and `cmd/stacky/workflow.go` register subcommands. Root-level persistent flags mirror Python: `--log-level` adjusts a `slog` handler, `--color` toggles lipgloss styling, and `--remote-name` sets a default remote stored in a context struct shared with subcommands. Each Cobra command’s `PreRunE` performs input validation (for example, ensuring a branch argument resolves before the `RunE` touches git state), keeping `RunE` bodies focused on orchestrating package calls.

Shared state and feature logic move into `internal/` packages so the CLI remains thin. `internal/config` exposes `type Config struct` with fields `SkipConfirm`, `ChangeToMain`, `ChangeToAdopted`, `ShareSSHSession`, `UseMerge`, `UseForcePush`, and `CompactPRDisplay`; its `Load(ctx context.Context, repoRoot string) (Config, error)` merges `$HOME/.stackyconfig` with `<repo>/.stackyconfig` in priority order. `internal/gitstore` wraps `go-git` primitives to open repositories, enumerate branches, update refs, and spawn lightweight `git` commands when `go-git` lacks porcelain (for example rebases). This package owns methods such as `CurrentBranch() (string, error)`, `ListBranches() ([]Branch, error)`, `UpdateRef(name string, new plumbing.Hash, old *plumbing.Hash) error`, `Checkout(branch string) error`, and `RunHook(name string, args ...string) error`. SSH multiplexing support lives beside it as `gitstore/ssh_mux.go`, invoking the system `ssh` binary when `ShareSSHSession` is true.

Stack modelling is centralised in `internal/stackgraph`. Define `type Branch struct` with fields `Name string`, `Parent *Branch`, `ParentCommit plumbing.Hash`, `Commit plumbing.Hash`, `Remote RemoteInfo`, `Children []*Branch`, and cached PR metadata. `type Graph struct` maintains maps of branch name to `*Branch`, `Bottoms []*Branch`, and `Tops []*Branch`, recreating the behaviour of Python’s `StackBranchSet`. `Graph` exposes methods such as `Load(ctx context.Context, repo gitstore.Repository) error`, `CurrentStack(branch string) ([]*Branch, error)`, `Forest() []*Branch`, `MarkParent(branch, parent string, parentCommit plumbing.Hash) error`, and `MarkBottom(branch string, isBottom bool) error`. Utility helpers translate the graph to trees that the UI layer can render, and enforce invariants like “every non-bottom node has a parent ref”. (Implemented: `Load`, `Branch`, `Bottoms`, `Upstack`, and `Downstack` now shell out through `gitstore` with tests covering stacked branches and error handling.)

Terminal presentation is handled by `internal/ui`, which defines lipgloss styles and printers. A `Renderer` struct offers `RenderForest(forest []*stackgraph.Branch) string`, `RenderPushPlan(plan []PushAction) string`, and `PromptConfirm(message string) (bool, error)` that respect `Config.SkipConfirm`. For interactive menus, introduce `internal/tui/menu` wrapping a simple selection widget (either a minimal custom implementation using termios or a dependency like `github.com/charmbracelet/bubbles/list`; document the choice in the Decision Log when finalised). The UI package also centralises hyperlink formatting for compact PR displays to maintain parity with the Python ANSI escape sequences.

GitHub integration sits behind `internal/githubcli`. This package shells out to `gh` but exposes an interface:

    type Client interface {
        ListPRs(ctx context.Context, params ListParams) ([]PullRequest, error)
        EditPRBase(ctx context.Context, number int, base string) error
        CreatePR(ctx context.Context, input CreateInput) (PullRequest, error)
        EditPRBody(ctx context.Context, number int, body string) error
        MergePR(ctx context.Context, input MergeInput) error
    }

The concrete `ghClient` builds `exec.CommandContext` calls (with configurable path lookups for tests), decodes JSON into Go structs equivalent to Python’s `PRInfo`, and captures stdout/stderr for error reporting. Tests can substitute a fake by satisfying the interface.

Long-running operations need durable state. `internal/state` reads and writes `~/.stacky.state` with `type State struct { Branch string; Sync []string; Fold *FoldState; MergeFold *MergeFoldState }`, employing `os.Rename` for atomic updates and using `json.MarshalIndent` for readability. Commands that might pause (sync, fold, merge-fold) call `state.Write()` before mutating git, and `stacky continue` consults this package to resume work. Errors identify malformed states so the CLI can inform users how to recover.

High-level orchestration resides in `internal/engine`. This layer composes `gitstore`, `stackgraph`, `githubcli`, `state`, and `ui` to deliver behaviours like `Sync(ctx, scope Scope)`, `Push(ctx, scope Scope, opts PushOptions)`, `Commit(ctx, message string, opts CommitOptions)`, `Land(ctx, opts LandOptions)`, `ImportGraphite(ctx, top string, opts ImportOptions)`, and `Inbox(ctx, opts InboxOptions)`. `Scope` encapsulates “current stack”, “upstack”, or “downstack” to keep command handlers trivial. Each method validates preconditions, updates state files, invokes git operations, and returns structured results (for example `SyncResult` listing branches seen and conflicts) so the CLI can print user-friendly summaries.

Testing strategy aligns with this architecture. Every package gets a `*_test.go` suite using `testify/require` and `testify/assert`. `internal/gitstore` tests spin up temporary repositories via `os.MkdirTemp` and `git.PlainInit`, covering branch creation, ref updates, and sync preconditions. `internal/stackgraph` tests verify topological loading from mocked refs and ensure equality with sample Python outputs stored under `testdata/python-fixtures.json`. `internal/githubcli` tests replace the command runner with a fake capturing arguments; `internal/state` tests confirm atomic writes. Integration tests under `internal/e2e` drive the Cobra command against fixture repos, stubbing `gh` via an environment variable that points to a mock script.

Scaffolding tasks for this blueprint include creating directories, adding placeholder files with package doc comments, and wiring a minimal `stacky --help` command that lists every subcommand (even if bodies panic with `TODO`). Once committed, subsequent milestones can replace `TODO`s with real logic without restructuring the CLI.

## Core Domain Implementation Roadmap – 2024-04-07

Start with configuration because every command consumes user preferences. Implement `internal/config/config.go` with a `Loader` struct that accepts an `fs.FS` abstraction so tests can inject in-memory files; expose `func Load(ctx context.Context, homedir, repoRoot string) (Config, error)` that: (1) initialises defaults, (2) reads `$homedir/.stackyconfig`, (3) if `repoRoot` non-empty, reads `<repoRoot>/.stackyconfig`, coercing booleans with sensible fallbacks, and (4) returns a fully populated `Config`. Add unit tests `config/config_test.go` using `testify` with `fstest.MapFS` fixtures covering partial files, missing sections, and precedence.

Next, create `internal/gitstore/repo.go` that wraps `go-git`’s `Repository` plus exec fallbacks. Define `type Repository struct { git *git.Repository; worktree *git.Worktree; runner Runner }` where `Runner` is an interface for spawning porcelain commands (`Run(ctx context.Context, name string, args ...string) (string, error)`). Provide constructors `Open(path string)` and `Init(path string)`. Implement helpers: `CurrentBranch`, `ListBranches`, `ReadRef(name string)`, `WriteRef(name string, new plumbing.Hash, old *plumbing.Hash)`, `Checkout(branch string)`, and `RebaseOnto(child, onto, from plumbing.Hash)` — the last can delegate to `runner` to call `git rebase` when `go-git` lacks built-in support. Add `ssh_mux.go` with methods `EnsureMux(remote string)` and `CloseMux(remote string)` that inspect `Config.ShareSSHSession`. Unit tests should run against temporary directories (`t.TempDir()`), verifying ref updates, branch creation, and that `Runner` receives expected commands.

Implement stack modelling in `internal/stackgraph`. Create `graph.go` defining `type Graph struct { repo gitstore.Repository; branches map[string]*Branch; bottoms []*Branch; tops []*Branch }` and loading logic: iterate through `gitstore.ListBranches`, call `gitstore.ReadRef("refs/stack-parent/<branch>")`, construct parent/child relations, and populate remote metadata via `gitstore.Remote(branch)` (to be added to `Repository`). Provide traversal helpers `Forest() []*Branch`, `CurrentStack(branch string) ([]*Branch, error)`, `Upstack(branch string)`, `Downstack(branch string)`, status checks `IsSyncedWithParent`, `IsSyncedWithRemote`, and mutators `MarkParent`, `MarkBottom`. Validate invariants (e.g., missing parent refs) and surface descriptive errors. Tests should mock `gitstore.Repository` with an in-memory implementation to cover single-branch, multi-stack, and invalid ref scenarios.

Recreate state handling in `internal/state/state.go`. Define serialisable structs mirroring Python’s JSON layout: `type State struct { Branch string `json:"branch"`; Sync []string `json:"sync,omitempty"`; Fold *FoldState `json:"fold,omitempty"`; MergeFold *MergeFoldState `json:"merge_fold,omitempty"` }`. Provide `Load(path string) (State, error)` that tolerates `os.ErrNotExist`, `Save(path string, state State) error` writing to `<path>.tmp` then `os.Rename`, and `Clear(path string) error`. Tests should purposely corrupt temp files to ensure errors propagate with actionable messages.

Introduce PR metadata types in `internal/github`. Mirror Python’s `PRInfo` as `type PullRequest struct { ID string; Number int; State string; Mergeable string; URL string; Title string; BaseRef string; HeadRef string; ... }`. Implement `Client.ListPRs`, `Client.GetPRForBranch`, and `Client.UpdateReviewers` by composing `gh` calls defined in the blueprint. Provide fixtures under `internal/github/testdata` showing JSON payloads; tests confirm parsing and error surfacing when `gh` returns non-zero.

Finally, wire these packages together inside `internal/engine`. Begin with `engine/context.go` exporting `type Context struct { Repo gitstore.Repository; Config config.Config; StatePath string; GitHub github.Client; UI ui.Renderer }`, plus `func New(ctx context.Context, opts Options) (*Context, error)`. Implement initial orchestration methods `Engine.Sync`, `Engine.Push`, `Engine.Commit`, and `Engine.Continue`, each calling into `stackgraph`, `state`, and `github` as appropriate and returning summary structs (`SyncSummary`, `PushSummary`). Tests should mock dependencies using small in-memory implementations to assert decision paths (e.g., refusal to commit on unsynced branches). Document each method with preconditions and expected side effects so contributors understand sequencing.

Throughout this roadmap, record deviations from the Python behaviour in the Decision Log and extend test fixtures whenever new cases appear. Keep incremental commits small so reviewers can verify parity at each step.

## Plan of Work

Begin by inventorying the Python implementation: for each command and helper function in `src/stacky/stacky.py`, note the inputs it expects, the git refs it reads or writes, and examples of its output. Document any global state (such as shared SSH sessions) and configuration options so that nothing is lost in translation. Use this audit to produce a command-to-feature matrix attached to this plan.

Next, establish the Go project structure at the repository root. Create `go.mod` with module path `github.com/pmenglund/stacky` (adjust if the canonical import path differs). Under `cmd`, define `main.go` that wires a Cobra root command. Mirror the Python CLI hierarchy by adding Cobra subcommands for `branch`, `stack`, `upstack`, `downstack`, `checkout`, `info`, `commit`, `amend`, `push`, `sync`, `inbox`, `prs`, `fold`, `import`, `adopt`, `land`, and `continue`. Keep `RunE` functions minimal by delegating to package-level functions, and use `PreRunE` for argument validation, following the instructions for Go style in this repository.

Create internal packages to separate concerns. For git access, add `internal/gitstore` with a `Repository` struct that wraps `go-git` objects (`git.PlainOpen`, `Worktree`, refs management) and helpers for reading and writing the stack-specific refs, querying branch tracking information, and performing rebases or cherry-picks. For stack modeling, add `internal/stackgraph` with types representing nodes, parents, and up/down traversals. For configuration, add `internal/config` to load and merge `~/.stackyconfig` and project-level overrides, parsing sections into a strongly typed struct matching the Python options (skip confirmations, force push, merge vs cherry-pick, etc.). For terminal output, add `internal/ui` that centralizes `lipgloss` styles for tree displays, status markers, prompts, and compact PR lines. For GitHub operations, add `internal/githubcli` that shells out to `gh` with controlled command builders, capturing JSON into Go structs that match the Python `PRInfo` expectations while allowing unit tests to inject fakes.

With the scaffolding in place, implement core behaviors incrementally. Start with command-independent utilities: functions to discover the repository root, determine the current branch, read stack metadata, and compute stack status markers (`*`, `~`, `!`). Port the logic that maintains `refs/stack-parent/*` and bottom markers, ensuring idempotent updates. Implement rebasing and syncing using `go-git`'s plumbing or by invoking `git` when `go-git` lacks a feature, documenting any fallbacks. Add state persistence analogous to the Python `~/.stacky.state`, including concurrency-safe writes and temp file usage.

After the foundations are tested, port each command group. Start with read-only commands (`info`, `stack info`, `inbox` compact display) to validate tree rendering and GitHub querying. Move to branch management commands (`branch new`, `branch up/down`, `checkout`, `fold`), ensuring they modify stack refs and git branches exactly as the Python tool does. Then handle synchronization commands (`stack sync`, `continue`, `update`, `upstack onto`) by implementing dependency-aware rebases and conflict continuation logic. Finally, implement PR-affecting commands (`push`, `land`, `prs`) so they push branches via `go-git` or `git` wrappers, construct PR metadata, and interact with `gh` to open or update pull requests. Throughout, keep interactive prompts aligned with existing behavior, using Cobra flags and `lipgloss` to reproduce menus (consider reusing Go libraries for terminal menus if needed, or reimplement simple selection logic).

Parallel to feature work, write tests. For pure logic (config merging, stack graph operations, command argument validation), create Go unit tests in the corresponding package directories using `testing` and `testify` assertions. For git-heavy behavior, set up test fixtures under `testdata/` with temporary repositories created via `go-git` in tests, ensuring operations like stacking, rebasing, and state persistence are covered. For GitHub interactions, design interfaces that can be mocked so tests verify command behavior without making network calls. Add high-level integration tests under `internal/e2e` or similar that run the Cobra command against a fixture repository and inspect outputs, guarded so they can run in CI without network access by stubbing `gh`.

Once functionality is complete, replace Python packaging. Update `README.md` to describe building the Go binary, adjust Bazel and any CI workflows to build and test Go code instead of Python, and add release instructions that produce cross-platform binaries. Preserve the Python code either archived or marked deprecated until migration is complete, and ensure users know how to uninstall the Python package.

## Concrete Steps

Work from the repository root unless stated otherwise. Run commands exactly as written to avoid environmental drift.

    ~/stacky$ go mod init github.com/pmenglund/stacky

Expect `go.mod` to appear with Go 1.25 declared. Next, fetch dependencies as they become necessary:

    ~/stacky$ go get github.com/spf13/cobra@latest
    ~/stacky$ go get github.com/charmbracelet/lipgloss@latest
    ~/stacky$ go get github.com/go-git/go-git/v5@latest
    ~/stacky$ go get github.com/stretchr/testify@latest

After creating command packages and internal modules, tidy the module to lock versions:

    ~/stacky$ go mod tidy

During development, format and vet code regularly:

    ~/stacky$ gofmt -w ./
    ~/stacky$ go test ./...
    ~/stacky$ go build .

For integration testing, create fixture repositories in a temp directory. Example script to initialize a stack fixture:

    ~/stacky$ go test ./internal/e2e -run TestStackyInfo -v

When replacing documentation, build the binary and confirm help text:

    ~/stacky$ ./bin/stacky --help

The expected output should list all subcommands, matching the README. Capture transcripts for regression comparison in the `Artifacts and Notes` section.

## Validation and Acceptance

Validation requires demonstrating that the Go binary replicates key workflows. Acceptance criteria include: running `go test ./...` yields all passing tests, with new tests covering stack graph operations, config parsing, and command handlers; executing `./bin/stacky info` inside a fixture repository prints the same tree and status markers as the Python version; executing `./bin/stacky stack sync` rebases branches without errors, honoring `refs/stack-parent/`; running `./bin/stacky push --no-pr` pushes branches to the configured remote without spawning the Python script; and `./bin/stacky inbox --compact` lists PRs with the same grouping and lipgloss-styled output. For GitHub-dependent commands, acceptance can be shown through mocked `gh` responses in tests plus a manual dry run against a personal repository documented in `Artifacts and Notes`.

## Idempotence and Recovery

Most steps are idempotent: rerunning `go mod tidy`, `gofmt`, or Cobra scaffold generators has no side effects beyond formatting. Repository mutations must guard against partially written refs: whenever updating `refs/stack-parent/*`, write to a temporary ref and atomically move it, mirroring the Python strategy with temp files. For operations like `stack sync` that can stop mid-rebase, persist state in `~/.stacky.state` before starting and provide a `continue` path that resumes safely. For GitHub interactions, detect failures from `gh` invocations and present actionable retries without leaving branches half-pushed.

## Artifacts and Notes

Maintain short transcripts of parity checks, such as the output of `stacky info` before and after the rewrite on the same repository, and store them under `docs/parity/` for reviewers. Capture representative JSON fixtures from `gh` responses to drive tests and document them here when added. Log any helper scripts or fixtures introduced for testing so future contributors know how to regenerate them.

## Interfaces and Dependencies

Primary dependencies are `github.com/spf13/cobra` for CLI parsing, `github.com/charmbracelet/lipgloss` for terminal styling, `github.com/go-git/go-git/v5` for git repository access, and `github.com/stretchr/testify` for testing. Internal interfaces should encapsulate external interactions: define `type GitHubClient interface { ListUserPRs(ctx context.Context) ([]PRInfo, error); CreateOrUpdatePR(...) (PullRequest, error) }` so commands can operate on abstractions; define `type StackRepository interface { CurrentBranch() (string, error); LoadStack() (*StackGraph, error); Sync(branch string, opts SyncOptions) error; }` within `internal/stackgraph`; and expose UI helpers like `func RenderStack(tree StackGraph, opts RenderOptions) string` from `internal/ui`. Keep Cobra command constructors in `cmd` trivial by delegating to these interfaces, facilitating testing and future extensions. Ensure all public functions are documented with comments describing behavior, inputs, and side effects for novice maintainers.

Initial version – 2024-04-07 (Codex): Created ExecPlan for the Go rewrite of stacky.
Update – 2024-04-07 (Codex): Documented the Python CLI audit and advanced Progress.
Update – 2024-04-07 (Codex): Added Go Implementation Blueprint and marked design milestone complete.
Update – 2024-04-07 (Codex): Outlined core domain implementation roadmap to guide next milestone.
Update – 2024-04-07 (Codex): Bootstrapped Go module, CLI skeleton, and internal package stubs to start implementation.

Update – 2025-10-06 (Codex): Documented stack sync resume support in Progress after implementing state persistence and continue handling.
