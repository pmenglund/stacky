`stacky` is a homebrewed tool to manage stacks of PRs. This allows developers to easily manage many smaller, more targeted PRs that depend on each other. The primary implementation is now a single statically linked Go binary that mirrors the original Python behaviour.


## Installation

### Build from source (Go)

1. Ensure Go 1.25 or newer is installed.
2. Clone this repository and change into it.
3. Build the binary: `go build -o bin/stacky .`
4. Add the `bin/` directory to your `PATH`, or copy `bin/stacky` to a directory that is already on your `PATH`.
5. Verify the installation with `stacky --help`.

The repository also provides Bazel targets that build and test the Go CLI:

```bash
bazel build //:stacky      # produces bazel-bin/stacky
bazel test  //:go_tests    # runs go test ./...
```

Both commands respect local module caches in `.gocache/` so they can run without network access after the first dependency fetch.

## Releasing

Maintainers can generate release artifacts for multiple platforms with `./tools/package_release.sh <version>`.
This script cross-compiles the Go CLI for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64),
producing `.tar.gz` or `.zip` archives under `dist/` alongside a `SHA256SUMS` file.
Pass the version tag you plan to release (for example `v1.2.3`) to ensure the archive names match the published assets.

GitHub Releases are automated. Pushing a tag matching `v*` triggers the `Act on release created` workflow,
which runs the same cross-compilation matrix, uploads the archives, generates checksums, and attaches everything
as draft assets on the release for final review.

### Legacy Python CLI (deprecated)

The historical Python implementation remains available for short-term migration needs. It is no longer the primary deliverable and will be removed in a future release.

- Editable install: clone the repository and run `pip install -e .`.
- Manual execution: install `asciitree`, `ansicolors`, `simple-term-menu`, and `argcomplete`, then run `./src/stacky/stacky.py`.
- Bazel packaging: `bazel build //:legacy_stacky` (binary) or `bazel build //:legacy_stacky.xar`.
- Uninstall when you migrate: `pip uninstall rockset-stacky`.

Shell completion instructions in the sections below apply only to the legacy Python CLI. The Go CLI will gain native Cobra-powered completion in a later update.

## Tab Completion (legacy Python CLI)

Tab completion is available only for the legacy Python command through `argcomplete`:

```bash
pip3 install argcomplete
activate-global-python-argcomplete  # one-time setup
```

Alternatively, enable it per shell session with `eval "$(register-python-argcomplete stacky)"`.

## Accessing GitHub
Stacky speaks to GitHub directly through the official REST and GraphQL APIs via the `go-gh` client. You still need a working `git` binary, but you no longer need the `gh` executable on your `PATH`. Authentication reuses the GitHub CLI configuration (`~/.config/gh/hosts.yml`), so run `gh auth login` once or manage the file manually, and Stacky will pick up the stored token. For GitHub Enterprise instances, set the usual `GH_HOST` and related environment variables before running Stacky.

## Usage
`stacky` stores all information locally, within your git repository
Syntax is as follows:
- `stacky info`: show all stacks , add `-pr` if you want to see GitHub PR numbers (slows things down a bit)
- `stacky inbox [--compact]`: show all active GitHub pull requests for the current user, organized by status (waiting on you, waiting on review, approved, and PRs awaiting your review). Use `--compact` or `-c` for a condensed one-line-per-PR view with clickable PR numbers.
- `stacky prs`: interactive PR management tool that allows you to select and edit PR descriptions. Shows a simple menu of all your open PRs and PRs awaiting your review, then opens your preferred editor (from `$EDITOR` environment variable) to modify the selected PR's description.
- `stacky branch`: per branch commands (shortcut: `stacky b`)
    - `stacky branch up` (`stacky b u`): move down the stack (towards `master`)
    - `stacky branch down` (`stacky b d`): move down the stack (towards `master`)
    - `stacky branch new <name>`: create a new branch on top of the current one
    - `stacky branch commit <name> [-m <message>] [-a]`: create a new branch and commit changes in one command
- `stacky commit [-m <message>] [--amend] [--allow-empty] [-a]`: wrapper around `git commit` that syncs everything upstack
    - `stacky amend`: will amend currently tracked changes to top commit
- `stacky fold [--allow-empty]`: fold current branch into its parent branch and delete the current branch. Any children of the current branch become children of the parent branch. Uses cherry-pick by default, or merge if `use_merge` is enabled in config. Use `--allow-empty` to allow empty commits during cherry-pick.
- Based on the first argument (`stack` vs `upstack` vs `downstack`), the following commands operate on the entire current stack, everything upstack from the current PR (inclusive), or everything downstack from the current PR:
    - `stacky stack info [--pr]`
    - `stacky stack sync`: sync (rebase) branches in the stack on top of their parents
    - `stacky stack push [--no-pr]`: push to origin, optionally not creating PRs if they don’t exist
- `stacky upstack onto <target>`: restack the current branch (and everything upstack from it) on top of another branch (like `gt us onto`), useful if you’ve made a separate PR that you want to include in your stack
- `stacky continue`: continue an interrupted stacky sync command (because of conflicts)
- `stacky update`: will pull changes from github and update master, and deletes branches that have been merged into master

The indicators (`*`, `~`, `!`) mean:
- `*` — this is the current branch
- `~` — the branch is not in sync with the remote branch (you should push)
- `!` — the branch is not in sync with its parent in the stack (you should run `stacky stack sync`, which will do some rebases)

```
$ stacky --help
usage: stacky [-h] [--color {always,auto,never}]
              {continue,info,commit,amend,branch,b,stack,s,upstack,us,downstack,ds,update,import,adopt,land,push,sync,checkout,co,sco,inbox,prs,fold} ...

Handle git stacks

positional arguments:
  {continue,info,commit,amend,branch,b,stack,s,upstack,us,downstack,ds,update,import,adopt,land,push,sync,checkout,co,sco,inbox,prs,fold}
    continue            Continue previously interrupted command
    info                Stack info
    commit              Commit
    amend               Shortcut for amending last commit
    branch (b)          Operations on branches
    stack (s)           Operations on the full current stack
    upstack (us)        Operations on the current upstack
    downstack (ds)      Operations on the current downstack
    update              Update repo
    import              Import Graphite stack
    adopt               Adopt one branch
    land                Land bottom-most PR on current stack
    push                Alias for downstack push
    sync                Alias for stack sync
    checkout (co)       Checkout a branch
    sco                 Checkout a branch in this stack
    inbox               List all active GitHub pull requests for the current user
    prs                 Interactive PR management - select and edit PR descriptions
    fold                Fold current branch into parent branch and delete current branch

optional arguments:
  -h, --help            show this help message and exit
  --color {always,auto,never}
                        Colorize output and error
```

### Sample Workflow 
1. Create a new working branch with `stacky branch new <branch_name>`. 
2. Update files and add files to git tracking like normal (`git add`)
3. Commit updates with `stacky commit -m <commit_message>`
4. Create a stacked branch with `stacky branch new <downstack_branch_name>`
5. Update files and add files in downstack branch (`git add`)
6. `stacky push` will create 2 PRs. Top branch will have a PR against master and bottom branch will have a PR against the top branch.
7. Update the upstack branch and run `stacky commit`. This will rebase changes in the upstack branch to the downstack branch
8. `stacky push` will update both the PRs.

```
$> stacky branch new change_part_1
branch 'change_part_1' set up to track 'master'.
$> touch adding_new_file
$> git add adding_new_file
$> stacky commit -m "Added new file"
[change_part_1 23b102a] Added new file
 1 file changed, 0 insertions(+), 0 deletions(-)
 create mode 100644 adding_new_file
~* change_part_1
✓ Not syncing branch change_part_1, already synced with parent master
$> stacky branch new change_part_2
branch 'change_part_2' set up to track 'change_part_1'.
$> touch second_file
$> git add second_file
$> stacky commit -m "Added second file"
[change_part_2 0805f57] Added second file
 1 file changed, 0 insertions(+), 0 deletions(-)
 create mode 100644 second_file
~* change_part_2
✓ Not syncing branch change_part_2, already synced with parent change_part_1
$> stacky info
 │   ┌── ~* change_part_2
 ├── ~ change_part_1
master
$> stacky push
     ┌── ~* change_part_2
 ┌── ~ change_part_1
master
✓ Not pushing base branch master
- Will push branch change_part_1 to origin/change_part_1
- Will create PR for branch change_part_1
- Will push branch change_part_2 to origin/change_part_2
- Will create PR for branch change_part_2

Proceed? [yes/no] yes
Pushing change_part_1
Creating PR for change_part_1
? Title change part 1
? Body <Received>
? What's next? Submit as draft
https://github.com/rockset/stacky/pull/2
Pushing change_part_2
Creating PR for change_part_2
? Title Added second file
? Body <Received>
? What's next? Submit as draft
https://github.com/rockset/stacky/pull/3
$> git co change_part_1
$> vim adding_new_file
$> git add adding_new_file
$> stacky commit -m "updated new file"
[change_part_1 aa06f71] updated new file
 1 file changed, 1 insertion(+)
 ┌── !~ change_part_2
~* change_part_1
✓ Not syncing branch change_part_1, already synced with parent master
- Will sync branch change_part_2 on top of change_part_1
```

## Tuning

The behavior of `stacky` allow some tuning. You can tune it by creating a `.stackyconfig`
the file has to be either at the top of your repository (ie. next to the `.git` folder) or in the `$HOME` folder.

If both files exists the one in the home folder takes precedence.
The format of that file is following the `ini` format and has the same structure as the `.gitconfig` file.

In the file you have sections and each sections define some parameters.

We currently have the following sections:
 * UI
 * GIT

List of parameters for each sections:

### UI
 * skip_confirm, boolean with a default value of `False`, set it to `True` to skip confirmation before doing things like reparenting or removing merged branches.
 * change_to_main: boolean with a default value of `False`, by default `stacky` will stop doing action is you are not in a valid stack (ie. a branch that was created or adopted by stacky), when set to `True` `stacky` will first change to `main` or `master` *when* the current branch is not a valid stack.
 * change_to_adopted: boolean with a default value of `False`, when set to `True` `stacky` will change the current branch to the adopted one.
 * share_ssh_session: boolean with a default value of `False`, when set to `True` `stacky` will create a shared `ssh` session to the `github.com` server. This is useful when you are pushing a stack of diff and you have some kind of 2FA on your ssh key like the ed25519-sk.
 * compact_pr_display: boolean with a default value of `False`, when set to `True` `stacky info --pr` will show a compact format displaying only the PR number and status emoji (✅ approved, ❌ changes requested, 🔄 waiting for review, 🚧 draft) without the PR title. Both compact and full formats include clickable links to the PRs.

### GIT
 * use_merge: boolean with a default value of `False`, when set to `True` `stacky` will use `git merge` instead of `git rebase` for sync operations and `stacky fold` will merge the child branch into the parent instead of cherry-picking individual commits.
 * use_force_push: boolean with a default value of `True`, controls whether `stacky` can use force push when pushing branches.

### Example Configuration

Here's a complete example of a `.stackyconfig` file with all available options:

```ini
[UI]
# Skip confirmation prompts (useful for automation)
skip_confirm = False

# Automatically change to main/master when not in a valid stack  
change_to_main = False

# Change to the adopted branch after running 'stacky adopt'
change_to_adopted = False

# Create shared SSH session for multiple operations (helpful with 2FA)
share_ssh_session = False

# Show compact format for 'stacky info --pr' (just number and emoji)
compact_pr_display = False

[GIT]
# Use git merge instead of rebase for sync operations
use_merge = False

# Allow force push when pushing branches
use_force_push = True
```

## License

- [MIT License](https://github.com/rockset/stacky/blob/master/LICENSE.txt)
