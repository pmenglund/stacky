# Agent Instructions

This repo contains a Golang rewrite of a python tool, called `stacky`.

It is used to manage stacks of git branches, which correspond to GitHub pull requests (PR), so you can break up what would otherwise be one large PR into smaller discrete PRs, to make it easier for the code reviewer.  

## Plan

The file `.agent/PLAN.md` contains the execution plan, for what must be done next.

## Code Style

The go version used is v1.25, and the code should be written in a a way so it is easily testable.

The CLI is implemented using spf13/cobra, where the `RunE` just invokes a function which has minimal logic. If any validation is needed, it should be done in `PreRunE`.

Terminal coloring is done using the `charmbracelet/lipgloss` module.

Git manipulation is done using the `go-git` module, and GitHub interactions using `google/go-github`.

## Testing

Tests should be written using the testify module, with `require` and `assert`
