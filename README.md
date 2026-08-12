# fasteasyjson

A CLI-compatible, faster drop-in for [mailru/easyjson](https://github.com/mailru/easyjson)'s
`easyjson` command. Same flags, same `//go:generate`/`GOFILE`/`-pkg` behavior,
same generated output, plus one addition: `-check`.

## Problem

easyjson's generator writes a randomly-named temporary launcher file next
to the target on every invocation, to `go run` the actual generator. The
compiler embeds a source file's on-disk path into the debug info of the
binary it produces, so a randomly-suffixed launcher name makes that one
compile+link action permanently un-cacheable: GOCACHE grows by one fresh,
never-reused entry per annotated file on every single run, forever, even
when nothing changed. On a repo with many annotated files, this repeats on
every CI run.

## Design

`fasteasyjson` produces the same output through the same underlying mechanism:
it depends on `mailru/easyjson`'s own `bootstrap`, `parser`, and `gen`
packages rather than reimplementing generation (this tool is just ~500 LoC in total).

- The launcher gets a deterministic name, a hash of its group's target
  paths, instead of a random one, so its compile+link action is a stable,
  reusable GOCACHE entry across repeated runs instead of a new one every
  time.
- Compile stubs are served through `go run -overlay=...` instead of being
  written to disk. The real file is written at most once per run, not
  twice, and only if its content differs - see below.
- Files are batched into as few `go run` invocations as possible:
    - Every file with no `internal/` import constraint batches into one
      group, regardless of how many packages it spans. A launcher can import
      any non-`internal/` package from any location on disk.
    - Files that need an `internal/` package are grouped by their specific
      `internal/`-ancestor directory. A launcher can reach an `internal/`
      package only from somewhere under that package's parent-of-`internal`
      directory.
    - Each `go run` invocation carries a fixed toolchain-startup and
      dependency-freshness-scan cost independent of GOCACHE warmth. CI does
      not benefit from repeated-run cache warmup the way a long-lived dev
      machine does, since every job starts with a cold OS page cache
      regardless of whether the GOCACHE directory itself is restored from a
      persisted artifact. Reducing invocation count is the lever that reduces
      CI wall-clock time.
- A file is written only if its generated content differs from what is on
  disk.
- `-check` performs the same generation but never writes; it reports each
  stale file and exits 1 if any are found.
- Generation runs strictly one group at a time, never concurrently to avoid overlay corruptions.

## Performance

Measured on a large multi-package monorepo (~100 annotated files,
~35 packages):

|                            | full-repo check                                |
|----------------------------|------------------------------------------------|
| easyjson (write-based)     | several minutes; no improvement on repeat runs |
| fasteasyjson, cold GOCACHE | under 30s                                      |
| fasteasyjson, warm GOCACHE | 1-4s                                           |

GOCACHE size stays flat across repeated runs. The original generator's
GOCACHE footprint grows on every run.

## Requirements

Go 1.20+, for both building and running.

- Build: bounded by the `mailru/easyjson` dependency, which declares
  `go 1.20`. Module graph rules require this module's directive to be at
  least that.
- Run: the tool invokes the `go` binary on `PATH` with `-overlay` (added in
  Go 1.16) and `-trimpath` (added in Go 1.13).

## Install

```
go install github.com/vearutop/fasteasyjson@latest
```

## Usage

```go
//go:generate fasteasyjson -all $GOFILE
```

```
fasteasyjson -all file1.go file2.go file3.go
fasteasyjson -pkg ./mypackage
fasteasyjson -check -all file1.go file2.go   # verify only, exit 1 if stale
```

All of `easyjson`'s original flags are supported unchanged: `-build_tags`,
`-gen_build_flags`, `-snake_case`, `-lower_camel_case`, `-no_std_marshalers`,
`-omit_empty`, `-omit_zero`, `-all`, `-byte`, `-leave_temps`, `-stubs`,
`-noformat`, `-output_filename`, `-pkg`, `-disallow_unknown_fields`,
`-disable_members_unescape`, `-version`.

`-check` is the one addition.
