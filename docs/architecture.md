# Architecture

This document describes how `goboxd` is put together: the moving parts, the
lifecycle of a single request, and the configuration model that drives
per-language behaviour.

---

## High-Level Picture

The server is a Go HTTP service that fans each request out to one or more
`nsjail` subprocesses. The Go layer never executes user code directly — it is
responsible only for parsing the request, validating inputs, laying out a
per-job sandbox directory, building `nsjail` argument lists from the language
registry, and aggregating the results.

```text
                ┌─────────────────────────────────────────────────┐
 HTTP POST  ──▶│  chi Router (internal/api)                      │
   /run         │   └── Handler.Run()                             │
                │         ├── ValidateRunRequest()                │
                │         └── runner.Run()                        │
                │               ├── semaphore acquire             │
                │               ├── createSandboxDir()            │
                │               ├── os.WriteFile() source         │
                │               ├── runPhase() [build, if any]    │
                │               ├── runPhase() × N [per testcase] │
                │               └── os.RemoveAll() cleanup        │
                └──────────────────────┬──────────────────────────┘
                                       │ exec.Command
                                       ▼
                       ┌──────────────────────────────────┐
                       │  nsjail (Linux namespaces)        │
                       │   ├── user / pid / mount / net ns │
                       │   ├── chroot to sandbox dir       │
                       │   ├── rlimit_fsize / nofile /     │
                       │   │   stack + optional cgroup v2  │
                       │   └── exec compiler / runtime     │
                       └──────────────────────────────────┘
```

---

## Components

### `cmd/goboxd/`

Entry point. `main.go` does four things in order:

1. Loads `configs/languages.yaml` via `config.LoadFile()`.
2. Runs startup probes — nsjail binary present, every language toolchain
   reachable. Exits immediately with a clear error if anything is missing.
3. Calls `runner.SweepOrphanDirs()` to clean up sandbox directories left by
   any previous crashed run.
4. Starts the HTTP server with graceful shutdown on `SIGINT`/`SIGTERM`.

### `internal/api/`

The HTTP layer. Three files:

- **`router.go`** — Wires routes to handlers. Applies middleware in order:
  `RecoverPanic → RequestID → Logger → CleanPath → MaxBodySize`.
- **`handler.go`** — Four handlers: `Healthz`, `Readyz`, `Info`, `Run`.
  Handlers decode JSON, call the validator, build a `runner.Job`, and write
  the response. They never touch nsjail directly.
- **`middleware.go`** — `RequestID` injects a UUID per request.
  `MaxBodySize` wraps the request body with `http.MaxBytesReader` (512 KiB)
  before JSON decoding begins — this closes security hole #4 at the HTTP
  layer. `Logger` writes one structured JSON line per request via `log/slog`.
  `RecoverPanic` catches any handler panic and returns a 500 without leaking
  internals.

### `internal/validator/`

Pure functions with no side effects. Called by the handler before any
execution begins. Covers:

- Language existence check against `cfg.KnownLanguages`.
- Source size cap (`MaxSourceBytes = 256 KiB`).
- Filename safety: `filepath.Base` check, no path separators, no leading
  dot, character allowlist, length cap. Closes security hole #1.
- Flag allowlist: per-language list from YAML, wildcard suffix support
  (`-std=*`). Closes security hole #3.
- Test count cap (`MaxTests = 50`) and per-stdin size cap.
- `CompareOutput`: exact match → `accepted`; match after `strings.TrimSpace`
  → `output_whitespace_mismatch`; otherwise → `wrong_output`.
- `TopLevelStatus`: derives the envelope status from build + test results
  following the spec rule (first non-accepted test, or `build_failed`).

### `internal/config/`

Language registry. `LoadFile` reads `configs/languages.yaml`, unmarshals it
into `[]Language`, validates each entry (required fields, strategy values,
cmd presence), and builds three derived maps used at request time:

- `LanguagesByID map[string]*Language` — O(1) lookup by id.
- `KnownLanguages map[string]bool` — fed to the validator.
- `AllowedFlags map[string][]string` — fed to the validator, merged from
  both the build and run phase allowlists.

`ProbeLanguage` and `ProbeNsjail` run `--version` on each binary at startup
and when `/readyz` is called.

### `internal/runner/`

Two files:

**`runner.go`** — `Runner` owns the semaphore (`chan struct{}`), the
in-flight counter (`atomic.Int32`), and the nsjail path. `Run()` acquires a
semaphore slot (blocking if at capacity, aborting on context cancellation),
increments the counter, calls `execute()`, and releases the slot in a
`defer`.

**`sandbox.go`** — `execute()` and all sandbox mechanics. See the request
lifecycle section below for the step-by-step.

### `internal/models/`

Shared structs and string constants only. No logic. Every other package
imports this; keeping it logic-free prevents circular imports. All status
strings (`accepted`, `build_failed`, `runtime_error`, …) are declared as
constants here — never as raw strings elsewhere.

### `configs/languages.yaml`

The single file that defines every supported language. Adding a language
requires editing only this file — no Go code change. See
[docs/languages.md](languages.md) for the full format and a walkthrough.

---

## Request Lifecycle

The server receives one HTTP request per code submission. For a compiled
language (C++) with two test cases:

**1. Decode and validate.**
`json.NewDecoder(r.Body).Decode(&req)` — the body is already capped at
512 KiB by `MaxBytesReader`. `ValidateRunRequest` runs all field checks.
Any failure returns HTTP 400 before touching the filesystem.

**2. Resolve language config.**
`cfg.LanguagesByID[req.Language]` returns a pointer into the loaded config.
`EffectiveSourceFilename` / `EffectiveArtifactFilename` apply the
`fixed`/`from_request` strategy. `EffectiveBuildLimits` /
`EffectiveRunLimits` merge client-supplied overrides on top of YAML defaults
using pointer-typed `*int` fields so absent fields are distinguishable from
zero.

**3. Semaphore acquire.**
`r.sem <- struct{}{}` blocks until a concurrency slot is free. If the
client disconnects while waiting, `ctx.Done()` fires and the goroutine exits
without consuming a slot.

**4. Create sandbox directory.**
`createSandboxDir()` creates `/tmp/goboxd-jails/<pid>-<counter>/` at mode
`0700`. The counter is an `atomic.Uint64` — unique within the process,
no retries, no races. A `tmp/` subdirectory is created inside for compilers
that need it (`javac`, `rustc`). Closes security hole #5.

**5. Write source file.**
`os.WriteFile(filepath.Join(sandboxDir, sourceFilename), …, 0644)`.
No shell is involved. The resolved path is prefix-checked against
`sandboxDir` as a belt-and-suspenders check after the validator already
confirmed the filename is safe. Closes security holes #1 and #2.

**6. Build phase (compiled languages only).**
`runPhase()` is called with `isBuild: true`. It expands YAML arg templates,
constructs the nsjail command line, and runs `exec.CommandContext`. If the
compiler exits non-zero, all test results are set to `not_executed` and the
response is returned immediately with `build_failed`.

**7. Run phase — one nsjail invocation per test case.**
`runPhase()` is called once per `TestInput` with the test's `Stdin` piped
in. stdout and stderr are captured through `limitedWriter` (capped at 4 MiB
each). Closes security hole #6. The result is passed to `CompareOutput`
to assign the test status.

**8. Aggregate.**
`TopLevelStatus` derives the envelope status: `accepted` only if every test
is accepted; otherwise the status of the first non-accepted test.

**9. Cleanup.**
`defer os.RemoveAll(sandboxDir)` runs on every exit path including panics.
Closes security hole #7.

**10. Release semaphore.**
`defer func() { <-r.sem; r.inFlight.Add(-1) }()` runs after cleanup.

---

## nsjail Command Construction

`buildNsjailArgs` assembles the nsjail invocation for each phase. Key flags:

| Flag | Purpose |
|---|---|
| `--mode o` | One-shot: run one command then exit |
| `--chroot sandboxDir` | Jail root is the per-request directory |
| `--rw` | Read-write chroot (needed to write compiled artifact) |
| `--cwd /` | Working directory inside the jail |
| `--user 65534 --group 65534` | Run as unprivileged `nobody` |
| `--iface_no_lo` | No loopback — no network access |
| `--time_limit N` | Wall-clock cap; nsjail sends SIGKILL on expiry |
| `--rlimit_fsize 128` | Max file write: 128 MB |
| `--rlimit_nofile 64` | Max open file descriptors |
| `--rlimit_stack 64` | Max stack size: 64 MB |
| `--bindmount_ro /usr` | Runtimes, compilers, headers, libc (read-only) |
| `--bindmount_ro /bin` | `/bin/sh` and shell utilities |
| `--bindmount_ro /proc` | Required by JVM and some runtimes |
| `--bindmount_ro /etc` | Dynamic linker cache (`ld.so.cache`) |
| `--bindmount_ro /lib /lib64` | ELF interpreter path (hardcoded in binaries) |
| `--env PATH=…` | Minimal safe PATH inside the jail |
| `--cgroup_mem_max` | Memory cap (only when cgroup v2 is available) |
| `--cgroup_pids_max` | PID cap (only when cgroup v2 is available) |

All paths passed to the compiler or runtime inside the jail are
jail-relative (`/solution.py`, `/solution`) — not host absolute paths.
`expandArgs` handles the `{{source}}`, `{{artifact}}`, `{{flags}}`, and
`{{workdir}}` substitutions from YAML.

---

## WSL2 and Container Compatibility

nsjail relies on cgroup v2 to enforce memory and PID limits.
Under WSL2 and some Docker configurations, cgroup delegation is not
available and nsjail exits immediately when those flags are passed.

`nsjailCgroupWorks()` applies three checks before adding cgroup flags:

1. `/proc/version` must not contain `microsoft` (WSL kernel string).
2. `/sys/fs/cgroup/cgroup.controllers` must be readable.
3. Both `memory` and `pids` must appear in the controllers file.

If any check fails, `--cgroup_mem_max` and `--cgroup_pids_max` are omitted.
The time limit (`--time_limit`) and rlimit-based caps remain active in all
environments.

---

## Configuration Templating

`configs/languages.yaml` uses double-brace placeholders substituted at
request time by `expandArgs` and `runPhase`:

| Placeholder | Value |
|---|---|
| `{{source}}` | Jail-relative source path, e.g. `/solution.cpp` |
| `{{artifact}}` | Jail-relative artifact path, e.g. `/solution` |
| `{{flags}}` | Client-supplied flags after allowlist check (spliced as separate args) |
| `{{workdir}}` | Jail working directory `/` (used by Java `-cp`) |

`{{flags}}` splices into multiple separate arguments — not one joined
string. This is intentional: the shell would normally split them, but since
no shell is involved, `expandArgs` must do the splitting itself.

Per-request resource limit overrides use `*int` pointer fields in
`models.LimitOverride`. A nil pointer means "client did not send this field;
use the language default". A non-nil pointer pointing at zero means "client
explicitly sent 0". This distinction is impossible with plain `int`.

---

## Concurrency Model

Concurrency is controlled at one level: a buffered channel semaphore in
`Runner`.

```go
sem: make(chan struct{}, maxConcurrent)
```

`maxConcurrent` defaults to `runtime.NumCPU()` and is overridable via the
`MAX_CONCURRENT` environment variable. When all slots are occupied,
`Run()` blocks on the channel send. The client's request context is
respected — if the client disconnects, `ctx.Done()` fires and the goroutine
exits without consuming a slot.

Within a single request, test cases run sequentially. Each test case is its
own nsjail invocation. There is no shared mutable state between requests.
The only shared state per process is:

- The config (read-only after startup).
- The semaphore channel.
- The atomic counters (`jobsTotal`, `jobsFailedInternal`, `inFlight`,
  `jobCounter`).

All of these are safe for concurrent access without a mutex.

---

## Status Vocabulary

### Build phase

| Status | Meaning |
|---|---|
| `ok` | Compiler exited 0 |
| `failed` | Compiler exited non-zero (syntax error, link error) |
| `internal_error` | nsjail binary not found or OS-level failure |

### Test phase

| Status | Meaning |
|---|---|
| `accepted` | stdout matches expected exactly |
| `output_whitespace_mismatch` | stdout matches after `TrimSpace` |
| `wrong_output` | stdout does not match |
| `time_exceeded` | nsjail exit code 2 (wall-clock limit hit) |
| `memory_exceeded` | nsjail exit code 3 (cgroup memory limit hit) |
| `runtime_error` | Non-zero exit or killed by signal |
| `not_executed` | Build failed; this test was never run |
| `internal_error` | OS-level failure running nsjail |

### Top-level envelope

`accepted` only when build is `ok` and every test is `accepted`. Otherwise
the status of the first non-accepted test, or `build_failed` if compilation
failed.

---

## Languages

Languages are declared in `configs/languages.yaml`. At time of writing:

| ID | Toolchain | Compiled |
|---|---|---|
| `py3` | `/usr/bin/python3` | no |
| `bash` | `/bin/bash` | no |
| `node` | `/usr/bin/node` | no |
| `c` | `/usr/bin/gcc` | yes |
| `cpp` | `/usr/bin/g++` | yes |
| `java` | OpenJDK (`javac` + `java`) | yes |
| `verilog` | Icarus Verilog (`iverilog` + `vvp`) | yes |

Adding a language is a configuration-only change. See
[docs/languages.md](languages.md) for the full procedure.

---

## Security Holes Closed

| # | Hole | Fix |
|---|---|---|
| 1 | Path traversal via filename | `ValidateFilename`: `filepath.Base` check, character allowlist, no leading dot |
| 2 | Shell-style directory commands | All filesystem ops use `os` package APIs — no shell ever runs |
| 3 | Compiler flag injection | Per-language allowlist in YAML; wildcard suffix support for `-std=*` |
| 4 | No request size limits | `http.MaxBytesReader` at 512 KiB; source cap 256 KiB; stdin cap 64 KiB; test count cap 50 |
| 5 | UID collisions under load | `pid + atomic.Uint64` counter — unique, no retries, no races |
| 6 | Unbounded child output | `limitedWriter` caps stdout and stderr at 4 MiB each; excess dropped silently |
| 7 | Stale jail directories | `defer os.RemoveAll` on every exit path; `SweepOrphanDirs` at startup |
