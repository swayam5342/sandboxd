# Security Enforcement & Threat Model

`sandboxd` treats security as a core architectural primitive. It runs untrusted, potentially malicious user submissions concurrently, protecting the host operating system and neighboring processes from process escape, denial-of-service, data leaks, and local elevation of privilege.

---

## 1. Sandbox Jail Isolation (NsJail)

`sandboxd` wraps Google's `NsJail` engine, leveraging low-level Linux kernel namespaces and features:

```
+--------------------------------------------------------------+
| Host OS                                                      |
|   +--------------------------------------------------------+ |
|   | isolated nsjail chroot (/tmp/sandboxd-jails/<req-id>)   | |
|   |   - Runs as Nobody UID 65534 / GID 65534                | |
|   |   - MOUNT Namespace (Read-only System Binds)            | |
|   |   - PID Namespace (Isolates from external PIDs)         | |
|   |   - NET Namespace (iface_no_lo: Network cut off)        | |
|   +--------------------------------------------------------+ |
+--------------------------------------------------------------+
```

### Namespace Hardening
- **Dropping to `UID 65534` / `GID 65534`**: The sandboxed process always ends up running as the unprivileged `nobody` user/group. *How* it gets there depends on the caller's own privilege level, checked at request time (`os.Geteuid() == 0`):
  - **Running as root (the Docker image)**: nsjail is invoked with `--disable_clone_newuser`, dropping privileges via a direct `setuid`/`setgid` rather than a nested user namespace. A nested `CLONE_NEWUSER` here would need broader capabilities (`CAP_SETUID`/`CAP_SETGID`/`CAP_SETPCAP`) than the container is granted (see §5) to write its own uid/gid maps — so it's skipped in favor of the simpler, capability-compatible path. The chroot's sandbox directory is `chown`'d to `65534:65534` so the now-unprivileged process can still read/write its own build output there.
  - **Running unprivileged (local/WSL dev as a regular user)**: nsjail uses its normal, unprivileged `CLONE_NEWUSER` path instead, which needs no special capabilities — any user can create a user namespace mapping their own real UID.

  Either way, even if a process inside the jail exploits a privilege-escalation bug, it remains a standard unprivileged user, never real host root.
- **Mount Namespace (`CLONE_NEWNS`)**: The sandbox is completely isolated inside a restricted directory. The root filesystem is chrooted to this clean sandbox directory.
- **PID Namespace (`CLONE_NEWPID`)**: Prevents the sandboxed process from viewing or signaling any host-level processes.
- **Network Isolation (`iface_no_lo`)**: Disables network interface access inside the jail. The sandboxed code cannot connect to external websites, scan internal networks, or initiate server-side request forgery (SSRF).

---

## 2. Hardened Read-Only Bind Mounts, No Host Secrets

To execute binaries, the jail requires runtime toolchains (compilers, interpreters, libraries). These are bind-mounted into the chroot jail using strict **read-only (`--bindmount_ro`)** settings:

- **`/usr`**, **`/lib`**, **`/lib64`**, **`/bin`**: Holds standard system binaries and library dependencies.
- **`/etc/alternatives`**, and any matching **`/etc/java-*-openjdk`** (only if present on the host): Debian's `update-alternatives` and OpenJDK's own packaging route toolchain entrypoints (`cc`, `javac`, ...) and JVM config (`java.security`, ...) through symlinks into these paths. Just toolchain indirection, mounted only when it actually exists on the host/image — never the whole host `/etc`.
- **`/dev/urandom`**, **`/dev/null`**: A narrowly-scoped entropy source for anything doing crypto/`SecureRandom` init (notably the JVM) — not the whole host `/dev`.

Two things are **deliberately not** host bind-mounts, because bind-mounting them would leak host information into the jail:
- **`/proc`** is mounted fresh (`--mount none:/proc:proc:`), scoped to the jail's own PID namespace — the sandboxed process sees only its own process tree, never the host's (or other tenants' concurrently-running jobs').
- **`/etc/passwd`, `/etc/group`, `/etc/nsswitch.conf`, `/etc/hosts`** are synthetic files written per-request into the sandbox directory itself (just a single `nobody` entry) rather than bind-mounted from the host — so uid/gid lookups (`getpwuid`, used by some toolchains) work without exposing the host's real user list, hostname, or `/etc/hosts` contents.

The chroot root (`/`) is the only read-write folder, uniquely named per request
(`<NSJAIL_BASE_DIR>/<request-id>/`) and owned by `65534:65534` when running as root (see §1)
so the unprivileged jailed process can write its own build output/temp files there.

---

## 3. Defense Against Command & Argument Injection

Standard shell execution is vulnerable to argument injection (e.g., executing `; rm -rf /`). `sandboxd` mitigates this systematically:

1. **No Shell Invocations**: The Go engine uses `exec.CommandContext` to bypass shell expansion (`sh` / `bash`) entirely. 
2. **Strict Slice Argumentation**: Command arguments are passed as native slices (`[]string`) bound directly to executive syscalls, ensuring malicious characters (like `;`, `&`, `|`, `` ` ``) are parsed literally as command arguments, not shell operators.
3. **Compile Flag Allowlist + Denylist**: Users cannot pass arbitrary compiler flags. Flags (e.g. `-O3`) are validated against a strict wildcard allowlist defined per-language (and separately per build/run phase) inside `lang.yaml`. Anything not on the allowlist (like injecting `-specs=`) is automatically rejected before compilation starts.

   Some allowlist wildcards are necessarily broad — a compiler groups dozens of unrelated
   options under one prefix (Rust's `-C*` covers everything from `-C opt-level=3` to
   `-C linker=...`). Where a handful of flags within such a wildcard are genuinely
   dangerous rather than just unwanted, a `flag_denylist` carves them back out — the
   denylist always wins, even over an allowlist wildcard match. For example, Rust's
   `-C linker=*` is denylisted because it lets a submission point rustc's linker at an
   arbitrary binary (including `/bin/sh`, which is bind-mounted into the jail per §2) —
   see [docs/languages.md](languages.md) for the full list and reasoning.

---

## 4. Resource Boundary Limits

Every limit below is applied per-phase (build vs. run), from each language's config in
`lang.yaml` — a client can only *tighten* these via `build.limits`/`run.limits` in the
request payload, never loosen them (see [docs/api.md](api.md)).

### CPU & Time limits
- Enforced via NsJail's `--time_limit` which kills processes using high-precision SIGKILL timers when exceeding the allocation.

### Memory & Process Limits — two layers, always both applied
- **Cgroups v2 (`--cgroup_mem_max`, `--cgroup_pids_max`)**: applied *only* when the host
  actually supports cgroup v2 delegation, detected at request time by checking
  `/proc/version` (rejecting anything WSL2-flavored) and that `memory`/`pids` both appear
  in `/sys/fs/cgroup/cgroup.controllers`. This is the precise, RSS-based enforcement.
- **POSIX RLIMIT fallback (`--rlimit_as`, `--rlimit_nproc`)**: applied **unconditionally**,
  regardless of cgroup availability — not just as a fallback for when cgroups are missing.
  `--rlimit_as` bounds virtual address space rather than resident memory, and interpreters
  like Node's V8 and the JVM reserve large virtual ranges on startup regardless of actual
  usage, so the RLIMIT_AS value is deliberately generous (4x the language's configured
  `memory_kb`, floored at 1536 MB) — a coarse backstop against pathological allocation, not
  a precise cap; precise enforcement is the cgroup layer's job when it's available.
  `--rlimit_nproc` bounds process/thread count and is what actually neutralizes fork bombs
  when cgroups aren't available.
- **`--rlimit_fsize 128`, `--rlimit_nofile 64`, `--rlimit_stack 64`**: max file write size,
  open file descriptors, and stack size — applied unconditionally in all environments.
- **Stdout/Stderr Truncation**: Captured outputs are limited at the stream level by a standard custom `limitedWriter` capping peak buffer allocation to `4 MB` (or custom `MAX_OUTPUT_SIZE`). This completely protects the supervisor against memory exhaust bugs caused by massive logging loops.

### Distinguishing a limit-triggered kill from a normal exit
nsjail reuses exit codes `2`/`3` both for "I killed this for exceeding a limit" and as a
plain passthrough of a child that exited normally with that code — a submission calling
`exit(2)`/`exit(3)` itself would otherwise be misreported as `time_exceeded`/
`memory_exceeded`. `sandboxd` cross-checks the measured wall-clock duration or peak memory
against ≥90% of the configured limit before making that call; see
[docs/api.md](api.md#status-definitions) for the exact rule.

---

## 5. System Environment Hardening

NsJail wipes **all** host environment variables by default to protect host secrets and keys. `sandboxd` injects a highly secure, restricted PATH variable:
```go
"--env", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"
```
This enables the C linker (`ld`) and dynamic compilers to find system binaries in standard locations without exposing host environments or secrets.

---

## 6. Container Capability Model

`docker-compose.yml` runs the service with `cap_drop: ALL` plus an explicit `cap_add` list —
not `privileged: true`. Each granted capability maps to something nsjail specifically needs:

| Capability | Why nsjail needs it |
| :--- | :--- |
| `SYS_ADMIN` | `unshare`/mount/`pivot_root` for namespace and chroot setup |
| `SYS_CHROOT` | `chroot` into the per-request sandbox directory |
| `SYS_RESOURCE` | Set rlimits (`fsize`/`nofile`/`stack`/`as`/`nproc`) |
| `SYS_PTRACE` | nsjail's seccomp-bpf syscall tracing/policy engine |
| `NET_ADMIN` | `iface_no_lo` (isolated, loopback-only network namespace) |
| `SETUID` / `SETGID` | Drop to uid/gid 65534 inside the jail |
| `SETPCAP` | nsjail adjusts its own securebits before dropping privileges |
| `DAC_OVERRIDE` | Read the host runtime directories (`/usr`, `/lib`, ...) it bind-mounts, regardless of file permissions |
| `CHOWN` | Own the per-request sandbox directory as uid/gid 65534 (see §1) |
| `KILL` | Enforce `--time_limit` by killing the jailed process |

`security_opt: [seccomp:unconfined, apparmor:unconfined]` is also required — Docker's default
seccomp/AppArmor profiles block the namespace/mount syscalls nsjail relies on even with the
capabilities above granted. This trades a meaningfully smaller blast radius than
`privileged: true` (no arbitrary device access, no ability to load kernel modules, no
override of every other capability check on the host) for the specific, auditable set nsjail
actually exercises.

---

## 7. API Authentication

`POST /run` (executing submitted code) is gated behind an optional API key: set the
`API_KEY` environment variable, and every `/run` request must then send
`Authorization: Bearer <API_KEY>` or receive `401 Unauthorized` (constant-time compared, to
avoid timing side-channels). If `API_KEY` is left unset, the server logs a startup warning
and accepts unauthenticated requests — acceptable for local development, not for any
deployment reachable outside your own machine, since an anonymous caller who can reach the
port can otherwise execute arbitrary code and consume compute resources without limit.
`/healthz`, `/readyz`, and `/info` remain unauthenticated regardless (see
[docs/api.md](api.md) for what they expose).
