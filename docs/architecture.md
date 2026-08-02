# Architecture

This document describes how `sandboxd` is put together: the moving parts, the
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
 HTTP POST  ──> │  chi Router (internal/api)                      │
   /run         │   └── RequireAPIKey() [if API_KEY is set]       │
                │         └── Handler.Run()                       │
                │               ├── ValidateRunRequest()          │
                │               └── runner.Run()                  │
                │                     ├── semaphore acquire       │
                │                     ├── createSandboxDir()      │
                │                     ├── os.WriteFile() source   │
                │                     ├── runPhase() [build, if any] │
                │                     ├── runPhase() × N [per testcase] │
                │                     └── os.RemoveAll() cleanup  │
                └──────────────────────┬──────────────────────────┘
                                       │ exec.Command
                                      \ /
                       ┌──────────────────────────────────┐
                       │  nsjail (Linux namespaces)       │
                       │   ├── pid / mount / net ns       │
                       │   │   (+ user ns, if unprivileged)│
                       │   ├── chroot to sandbox dir      │
                       │   ├── rlimit_fsize / nofile /    │
                       │   │   stack / as / nproc         │
                       │   │   + optional cgroup v2       │
                       │   └── exec compiler / runtime    │
                       └──────────────────────────────────┘
```

---

## Components

### `cmd/sandbox/`

Entry point. `main.go` does the following, in order:

1. Sets up logging (`log/slog` via `internal/loger`), writing to both stdout
   and `log/app.log` (falling back to stdout-only, with a warning, if the log
   directory/file can't be created).
2. Loads `config/lang.yaml` (path overridable via `LANG_CONFIG`) via
   `config.LoadFile()`.
3. Reads `API_KEY` from the environment; logs a startup warning if it's
   unset, since that leaves `POST /run` unauthenticated.
4. Runs startup probes — nsjail binary present, every language toolchain
   reachable. Exits immediately with a clear error if anything is missing.
5. Calls `runner.SweepOrphanDirs()` to clean up sandbox directories left by
   any previous crashed run.
6. Starts the HTTP server with graceful shutdown on `SIGINT`/`SIGTERM`.

### `internal/api/`

The HTTP layer. Three files:

- **`router.go`** — Wires routes to handlers. Global middleware, in order:
  `RecoverPanic → RequestID → Logger → CleanPath → MaxBodySize`. `RequireAPIKey`
  is applied only to `POST /run`, not globally — `/healthz`, `/readyz`, and
  `/info` stay unauthenticated.
- **`handlers.go`** — Four handlers: `Healthz`, `Readyz`, `Info`, `Run`.
  Handlers decode JSON, call the validator, build a `runner.Job`, and write
  the response. They never touch nsjail directly.
- **`middleware.go`** — `RequestID` injects a UUID per request (used as the
  sandbox directory name, see below). `RequireAPIKey` is a no-op when
  `API_KEY` is empty; otherwise it requires `Authorization: Bearer <API_KEY>`
  (constant-time compared) and returns `401` with `{"error":{"code":"unauthorized",...}}`
  on a mismatch. `MaxBodySize` wraps the request body with
  `http.MaxBytesReader` (512 KiB) before JSON decoding begins. `Logger` writes
  one structured JSON line per request via `log/slog`. `RecoverPanic` catches
  any handler panic and returns a 500 without leaking internals.

### `internal/validator/`

Pure functions with no side effects. Called by the handler before any
execution begins. Covers:

- Language existence check against `cfg.KnownLanguages`.
- Source size cap (`MaxSourceBytes = 256 KiB`).
- Filename safety: `filepath.Base` check, no path separators, no leading
  dot, character allowlist, length cap.
- Flag allowlist **and** denylist, checked separately for the build and run
  phases (`ValidateFlags(flags, allowlist, denylist)` — a deny match always
  wins, even over an allowlist wildcard match). Wildcard suffix support
  (`-std=*`, `-C*`, ...) for both lists via a shared `matchesAny` helper.
- Test count cap (`MaxTests = 50`) and per-stdin size cap.
- `CompareOutput`: exact match → `accepted`; match after `strings.TrimSpace`
  → `output_whitespace_mismatch`; otherwise → `wrong_output`.
- `TopLevelStatus`: derives the envelope status from build + test results
  following the spec rule (first non-accepted test, or `build_failed`).

### `internal/config/`

Language registry. `LoadFile` reads `config/lang.yaml`, unmarshals it into
`[]Language`, validates each entry (required fields, strategy values, cmd
presence), and builds five derived maps used at request time:

- `LanguagesByID map[string]*Language` — O(1) lookup by id.
- `KnownLanguages map[string]bool` — fed to the validator.
- `AllowedBuildFlags` / `AllowedRunFlags map[string][]string` — each
  language's build-phase and run-phase `flag_allowlist`, kept **separate**
  (a flag allowed for build is not automatically allowed for run).
- `DeniedBuildFlags` / `DeniedRunFlags map[string][]string` — the
  corresponding `flag_denylist` pair.

`mergeLimits`/`clampOverride` apply a client's `build.limits`/`run.limits`
override on top of a language's YAML defaults: any override is clamped into
`[1, default]` — a client can only tighten a limit, never loosen or disable
one (sending `0`/negative clamps to `1`; sending above the default clamps
down to the default).

`ProbeLanguage` and `ProbeNsjail` run `--version` (or the language's
configured `check` argument) on each binary, bounded by a 5-second timeout
per call, at startup and whenever `/info`/`/readyz` is called.

### `internal/runner/`

Two files:

**`runner.go`** — `Runner` owns the semaphore (`chan struct{}`), the
in-flight counter (`atomic.Int32`), and the nsjail path. `Run()` acquires a
semaphore slot (blocking if at capacity, aborting on context cancellation),
increments the counter, calls `execute()`, and releases the slot in a
`defer`.

**`sandbox.go`** — `execute()` and all sandbox mechanics: `createSandboxDir`,
`NsjailArgs`, `expandArgs`, `mapExitStatus`, `SweepOrphanDirs`. See the
request lifecycle section below for the step-by-step.

### `internal/models/`

Shared structs and string constants only. No logic. Every other package
imports this; keeping it logic-free prevents circular imports. All status
strings (`accepted`, `build_failed`, `runtime_error`, …) are declared as
constants here — never as raw strings elsewhere.

### `config/lang.yaml`

The single file that defines every supported language. Adding a language
requires editing only this file — no Go code change. See
[docs/languages.md](languages.md) for the full format and a walkthrough.

### `internal/sandbox/` (not yet wired in)

A separate, provider-agnostic sandbox abstraction (`Provider`/`Sandbox`
interfaces, a factory/registry for selecting an implementation by name, and
a fully in-memory `providers/mock` reference implementation). It exists as a
foundation for future pluggable execution backends and is currently
self-contained — nothing in `internal/api`/`internal/runner` depends on it,
and it has no knowledge of nsjail. See the package's own doc comment
(`internal/sandbox/doc.go`) for the design.

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
`createSandboxDir()` creates `<NSJAIL_BASE_DIR>/<request-id>/`, where
`request-id` is the per-request UUID assigned by the `RequestID` middleware —
unique across the process, no retries, no races. If `createSandboxDir` fails
partway through, it cleans up the partially-built directory itself before
returning (the caller's own `defer os.RemoveAll` only gets registered after
a *successful* return). When the server runs as root (the Docker image), the
directory is `chown`'d to the jailed uid/gid (65534) so the sandboxed
process — which drops privileges via direct `setuid`/`setgid` rather than a
user namespace in that case, see [docs/security.md](security.md) — can write
directly into its own chroot root; when running unprivileged, no chown is
needed or attempted (would fail with `EPERM` anyway). A `tmp/` subdirectory
is created inside for compilers that need it (`javac`, `rustc`, `iverilog`),
explicitly `chmod`'d to `0777` since `os.Mkdir`'s mode argument is masked by
the process umask. A synthetic `/etc` (`passwd`, `group`, `nsswitch.conf`,
`hosts`) is also written here — see [docs/security.md](security.md) for why.
Closes security hole #5.

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
each). Closes security hole #6. `mapExitStatus` classifies the result —
including a duration/memory cross-check to disambiguate nsjail's own
limit-triggered exit codes from a program's own `exit(2)`/`exit(3)`, see
[docs/api.md](api.md#status-definitions). If accepted-by-exit-code, the
result is then passed to `CompareOutput` to assign the final test status.

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

`NsjailArgs` assembles the nsjail invocation for each phase. Key flags:

| Flag | Purpose |
|---|---|
| `--mode o` | One-shot: run one command then exit |
| `--chroot sandboxDir` | Jail root is the per-request directory |
| `--rw` | Read-write chroot (needed to write compiled artifact) |
| `--cwd /` | Working directory inside the jail |
| `--user 65534 --group 65534` | Run as unprivileged `nobody` |
| `--disable_clone_newuser` | Only added when nsjail runs as euid 0 (Docker) — see [docs/security.md](security.md) §1 |
| `--iface_no_lo` | No loopback — no network access |
| `--time_limit N` | Wall-clock cap; nsjail sends SIGKILL on expiry |
| `--rlimit_fsize 128` | Max file write: 128 MB |
| `--rlimit_nofile 64` | Max open file descriptors |
| `--rlimit_stack 64` | Max stack size: 64 MB |
| `--rlimit_as N` | Max virtual address space, in MB — always applied (not just as a cgroup fallback); see `rlimitASMB` and [docs/security.md](security.md) §4 |
| `--rlimit_nproc N` | Max processes/threads — always applied |
| `--bindmount_ro /usr /lib /lib64 /bin` | Runtimes, compilers, headers, libc (read-only) |
| `--bindmount_ro /etc/alternatives` | Debian toolchain-entrypoint symlinks (only if present) |
| `--bindmount_ro /etc/java-*-openjdk` | OpenJDK config symlinks, version-glob'd (only if present) |
| `--bindmount_ro /dev/urandom /dev/null` | Narrowly-scoped entropy source (JVM `SecureRandom`, etc.) |
| `--mount none:/proc:proc:` | Fresh procfs scoped to the jail's own PID namespace — **not** a bind-mount of host `/proc` |
| `--env PATH=…` | Minimal safe PATH inside the jail |
| `--cgroup_mem_max` | Memory cap (only when cgroup v2 delegation is detected as working) |
| `--cgroup_pids_max` | PID cap (only when cgroup v2 delegation is detected as working) |

Note what's *not* here: the host's real `/etc` is never bind-mounted (a synthetic one is
written per-request instead, see [docs/security.md](security.md) §2), and `/proc` is never
a bind-mount of the host's.

All paths passed to the compiler or runtime inside the jail are
jail-relative (`/solution.py`, `/solution`) — not host absolute paths.
`expandArgs` handles the `{{source}}`, `{{artifact}}`, `{{artifact_name}}`,
`{{flags}}`, and `{{workdir}}` substitutions from YAML.

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

`config/lang.yaml` uses double-brace placeholders substituted at
request time by `expandArgs` and `runPhase`:

| Placeholder | Value |
|---|---|
| `{{source}}` | Jail-relative source path, e.g. `/solution.cpp` |
| `{{artifact}}` | Jail-relative artifact **path**, e.g. `/solution` |
| `{{artifact_name}}` | Bare artifact **filename**, no leading `/`, e.g. `Solution` (used by Java `-cp <dir> <class>`, which needs a name, not a path) |
| `{{flags}}` | Client-supplied flags after allowlist/denylist check (spliced as separate args) |
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
- The atomic counters (`jobsTotal`, `jobsFailedInternal`, `inFlight`).

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
| `time_exceeded` | nsjail exit code 2, cross-checked against measured duration (see below) |
| `memory_exceeded` | nsjail exit code 3, cross-checked against measured peak memory (see below) |
| `runtime_error` | Non-zero exit or killed by signal, not attributable to a resource limit |
| `not_executed` | Build failed; this test was never run |
| `internal_error` | OS-level failure running nsjail |

### Top-level envelope

`accepted` only when build is `ok` and every test is `accepted`. Otherwise
the status of the first non-accepted test, or `build_failed` if compilation
failed.

---

## Languages

Languages are declared in `config/lang.yaml`. At time of writing:

| ID | Toolchain | Compiled |
|---|---|---|
| `py3` | `/usr/bin/python3` | no |
| `bash` | `/bin/bash` | no |
| `node` | `/usr/bin/node` | no |
| `c` | `/usr/bin/gcc` | yes |
| `cpp` | `/usr/bin/g++` | yes |
| `java` | OpenJDK (`javac` + `java`) | yes |
| `verilog` | Icarus Verilog (`iverilog` + `vvp`) | yes |
| `rust` | `/usr/bin/rustc` | yes |

Adding a language is a configuration-only change. See
[docs/languages.md](languages.md) for the full procedure.

---

## Security Holes Closed

| # | Hole | Fix |
|---|---|---|
| 1 | Path traversal via filename | `ValidateFilename`: `filepath.Base` check, character allowlist, no leading dot |
| 2 | Shell-style directory commands | All filesystem ops use `os` package APIs — no shell ever runs |
| 3 | Compiler flag injection | Per-language, per-phase allowlist in YAML with wildcard suffix support (`-std=*`); a separate denylist overrides dangerous flags within an otherwise-broad wildcard |
| 4 | No request size limits | `http.MaxBytesReader` at 512 KiB; source cap 256 KiB; stdin cap 64 KiB; test count cap 50 |
| 5 | UID collisions under load | Per-request UUID from the `RequestID` middleware — unique, no retries, no races |
| 6 | Unbounded child output | `limitedWriter` caps stdout and stderr at 4 MiB each; excess truncated with a marker appended |
| 7 | Stale jail directories | `defer os.RemoveAll` on every exit path; `SweepOrphanDirs` at startup |
| 8 | Unauthenticated code execution | Optional `API_KEY` + `RequireAPIKey` middleware on `POST /run` (constant-time compared) |
| 9 | Host information leak into the jail | Fresh, namespace-scoped `/proc` mount and a synthetic `/etc` instead of bind-mounting the host's |
