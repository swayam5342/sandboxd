# Security Enforcement & Threat Model

`sandboxd` treats security as a core architectural primitive. It runs untrusted, potentially malicious user submissions concurrently, protecting the host operating system and neighboring processes from process escape, denial-of-service, data leaks, and local elevation of privilege.

---

## 1. Sandbox Jail Isolation (NsJail)

`sandboxd` wraps Google's `NsJail` engine, leveraging low-level Linux kernel namespaces and features:

```
+--------------------------------------------------------------+
| Host OS                                                      |
|   +--------------------------------------------------------+ |
|   | isolated nsjail chroot (/tmp/sandboxd-jails/...)       | |
|   |   - USER Namespace (Nobody UID 65534 / GID 65534)      | |
|   |   - MOUNT Namespace (Read-only System Binds)           | |
|   |   - PID Namespace (Isolates from external PIDs)        | |
|   |   - NET Namespace (iface_no_lo: Network cut off)       | |
|   +--------------------------------------------------------+ |
+--------------------------------------------------------------+
```

### Namespace Hardening
- **User Namespace (`CLONE_NEWUSER`)**: Maps the sandboxed process exclusively to an unprivileged guest user (Nobody: `UID 65534` / `GID 65534`). Even if a process succeeds in running a privilege escalation exploit, it remains a standard unprivileged user on the host.
- **Mount Namespace (`CLONE_NEWNS`)**: The sandbox is completely isolated inside a restricted directory. The root filesystem is chrooted to this clean sandbox directory.
- **PID Namespace (`CLONE_NEWPID`)**: Prevents the sandboxed process from viewing or signaling any host-level processes.
- **Network Isolation (`iface_no_lo`)**: Disables network interface access inside the jail. The sandboxed code cannot connect to external websites, scan internal networks, or initiate server-side request forgery (SSRF).

---

## 2. Hardened Read-Only Bind Mounts

To execute binaries, the jail requires runtime toolchains (compilers, interpreters, libraries). These are bind-mounted into the chroot jail using strict **read-only (`--bindmount_ro`)** settings:

- **`/usr`**, **`/lib`**, **`/lib64`**, **`/bin`**: Holds standard system binaries and library dependencies.
- **`/proc`**: Read-only mount allowing processes (like JVM or Node.js) to query system memory structure safely without process leaks.
- **`/etc`**: Read-only mount required for core symbolic links (e.g. `/etc/alternatives/ld` or dynamic runtime links).

The only read-write folder is the ephemeral chroot root (`/`) which has restricted `0700` directory permissions mapped strictly to the unique session ID.

---

## 3. Defense Against Command & Argument Injection

Standard shell execution is vulnerable to argument injection (e.g., executing `; rm -rf /`). `sandboxd` mitigates this systematically:

1. **No Shell Invocations**: The Go engine uses `exec.CommandContext` to bypass shell expansion (`sh` / `bash`) entirely. 
2. **Strict Slice Argumentation**: Command arguments are passed as native slices (`[]string`) bound directly to executive syscalls, ensuring malicious characters (like `;`, `&`, `|`, `` ` ``) are parsed literally as command arguments, not shell operators.
3. **Compile Flag Allowlist**: Users cannot pass arbitrary compiler flags. Flags (e.g. `-O3`) are validated against a strict wildcard allowlist defined inside `lang.yaml`. Anything else (like injecting `-specs=`) is automatically rejected before compilation starts.

---

## 4. Resource Boundary Limits

### CPU & Time limits
- Enforced via NsJail's `--time_limit` which kills processes using high-precision SIGKILL timers when exceeding the allocation.

### Memory & Process Limits
- **Cgroups v2 Integration**: Uses Linux Control Groups memory limits (`--cgroup_mem_max`) and thread counts (`--cgroup_pids_max`) to securely bound execution and completely neutralize Fork Bomb attacks.
- **POSIX RLIMIT Fallback**: If running in non-delegated environments (like WSL2), `sandboxd` gracefully falls back to strict RLIMIT bounds (`--rlimit_fsize`, `--rlimit_nofile`, `--rlimit_stack`) ensuring reliable safety limits across all architectures.
- **Stdout/Stderr Truncation**: Captured outputs are limited at the stream level by a standard custom `limitedWriter` capping peak buffer allocation to `4 MB` (or custom `MAX_OUTPUT_SIZE`). This completely protects the supervisor against memory exhaust bugs caused by massive logging loops.

---

## 5. System Environment Hardening

NsJail wipes **all** host environment variables by default to protect host secrets and keys. `sandboxd` injects a highly secure, restricted PATH variable:
```go
"--env", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"
```
This enables the C linker (`ld`) and dynamic compilers to find system binaries in standard locations without exposing host environments or secrets.
