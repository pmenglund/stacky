# stacky

`stacky` manages stacks of Git branches and their corresponding GitHub pull requests.

## Highlights

- Builds and maintains branch parents using `refs/stack-parent/*`, so you can split large changes
  into review-sized PRs without losing track of dependencies.
- Works entirely through the Go toolchain and `go-git`; only a working `git` binary is required in
your PATH.
- Talks to GitHub via the official `go-gh` clients and reuses the credentials stored by the `gh`
  command. When `go-gh` cannot be initialised you can fall back to the actual `gh` executable.
- Provides interactive and non-interactive flows for inspecting stacks, pushing branches, editing
  PR descriptions, and landing changes.
- Ships with end-to-end tests, helper scripts, and release tooling that generate cross-platform
  archives from a single command.

## Requirements

- Go 1.25 or newer (for building from source).
- Git 2.x on your PATH.
- GitHub authentication set up through `gh auth login`, or a manually maintained
  `~/.config/gh/hosts.yml`.
- Optional: the `gh` CLI. `stacky` prefers `go-gh`; set `STACKY_GITHUB_CLIENT=cli` to force the CLI
  transport or leave it unset and Stacky will automatically fall back to `gh` when the API clients
  cannot be constructed. Override the binary path with `STACKY_GH_BIN` if needed.

## Installation

### Build from source

```bash
# Clone and build into ./bin/stacky
git clone https://github.com/pmenglund/stacky.git
cd stacky
go build .

# Optional: install straight into $GOBIN (defaults to $GOPATH/bin)
go install ./...
```

Add the resulting directory to your `PATH` and verify the installation with `stacky --help`.

### Create release archives

Use `tools/package_release.sh` to cross-compile and package the CLI for common platforms:

```bash
./tools/package_release.sh v1.2.3           # archives land in ./dist by default
./tools/package_release.sh v1.2.3 staging   # choose a different output directory
```

The script emits `.tar.gz` archives for Unix platforms, `.zip` files for Windows, and a matching
`SHA256SUMS` manifest.

## GitHub authentication

`stacky` reuses the credentials stored by the GitHub CLI. Run `gh auth login` (or maintain
`~/.config/gh/hosts.yml`) before using commands that interact with pull requests. To control which
transport is used:

- `STACKY_GITHUB_CLIENT=cli` forces Stacky to shell out to the `gh` binary.
- `STACKY_GITHUB_CLIENT=rest` disables the CLI fallback.
- `STACKY_GH_BIN=/path/to/gh` selects a specific executable when the CLI transport is active.

## Quick start

```bash
# Start from your feature branch and inspect the current stack
stacky info

# Create a new branch on top of the current one and commit staged changes
stacky branch new feature/add-logging
stacky commit -m "Add request logging"

# Push your stack and open PRs for anything that does not have one yet
stacky stack push --force

# Review outstanding work that needs your attention
stacky inbox --compact

# Land the bottom-most PR once it is approved and up to date
stacky land --auto
```

Colors are enabled automatically when standard output is a terminal. Set `--color=never` or export
`NO_COLOR=1` to disable styling.

## Command tour

The CLI exposes a tree of commands; the most frequently used operations are listed below. See
`stacky --help` for the full reference.

- **Stack inspection and navigation**
  - `stacky info [--pr]` renders the current stack as a tree.
  - `stacky stack info`, `stacky upstack info`, and `stacky downstack info` scope the tree to part of
    the stack.
  - `stacky up`, `stacky down`, and `stacky branch checkout` move between branches.
  - `stacky log` prints a Git log that is aware of stack parents.
- **Working with branches**
  - `stacky branch new <name>` creates a child branch on top of the current one.
  - `stacky branch commit <name> [flags]` creates a branch and immediately commits your work.
  - `stacky commit [flags]` and `stacky amend` wrap common `git commit` scenarios while keeping parent
    pointers in sync.
  - `stacky fold [--allow-empty]` squashes the current branch into its parent and updates children.
  - `stacky adopt <branch>` rewrites parentage to bring an existing branch into the stack.
- **Synchronising with remotes**
  - `stacky stack push [--no-pr] [--force]` pushes branches (and opens PRs when required).
  - `stacky stack sync` rebases the current stack on top of its recorded parents.
  - `stacky update [--force]` fast-forwards tracked base branches and cleans up merged branches.
  - `stacky continue` resumes an interrupted rebase or push workflow.
  - `stacky push` is an alias for `stacky downstack push`; `stacky sync` aliases `stacky stack sync`.
- **GitHub-focused commands**
  - `stacky inbox [--compact]` prints your PRs grouped by status and the PRs awaiting your review.
  - `stacky prs` shows a selectable list of PRs, opens your editor for the body, and updates it via the
    GitHub API.
  - `stacky land [--auto]` merges the bottom-most PR in the stack.
  - `stacky import <branch>` follows PR ancestry (e.g. stacks created with Graphite) and records parent
    refs locally.

Many commands offer interactive prompts. If they detect a non-interactive environment they fall back
to explicit flags or exit with explanatory messages.

## Configuration

`stacky` reads `.stackyconfig` from the repository root and the current user’s home directory
(home takes precedence). The file uses INI syntax with two sections:

- `[UI]`
  - `skip_confirm` — skip confirmation prompts when true.
  - `change_to_main` — checkout the default branch after certain operations.
  - `change_to_adopted` — checkout adopted branches automatically.
  - `share_ssh_session` — reuse SSH sessions when interacting with remotes.
  - `compact_pr_display` — default to the compact inbox layout.
- `[GIT]`
  - `use_merge` — use merge instead of cherry-pick when folding branches.
  - `use_force_push` — allow force pushes during stack sync/push (defaults to true).

## Development

- Run the full test suite with `go test ./...`.
- `tools/smoke.sh` runs a reduced set of end-to-end tests and manages a private build cache.
- `tools/go_test.sh` mirrors the CI `go test ./...` invocation.
- Use `go fmt ./...` (or `gofmt -w`) and `go mod tidy` to keep the tree tidy.
- `tools/package_release.sh` and the GitHub Actions workflows in `.github/workflows/` produce release
  artefacts.

## License

The project is released under the MIT license. See `LICENSE.txt` for details.
