# Language Registry Configuration

`sandboxd` features a dynamic, plug-and-play language registry. Runtimes, compilers, default resource thresholds, and security arguments are managed entirely inside a descriptive YAML file.

---

## Configuration Location

The service loads its configuration at boot from:
```
config/lang.yaml
```
*(Override this path at startup by setting the `LANG_CONFIG` environment variable)*.

---

## Configuration Schema

Every registered language configuration follows a structured format:

```yaml
languages:
  - id: py3
    name: Python 3
    source_filename: solution.py
    source_filename_strategy: fixed
    check: --version
    run:
      cmd: /usr/bin/python3
      args: ["{{source}}"]
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 100
```

### Schema Parameters
- **`id`**: Unique language identifier sent in requests (e.g. `py3`, `c`, `rust`).
- **`name`**: Descriptive display name of the runtime.
- **`source_filename`**: Default name given to the code written inside the sandbox directory.
- **`source_filename_strategy`**: 
  - `fixed`: Uses the static `source_filename` definition.
  - `from_request`: Extracted dynamically from requests (required for main-class dependent compilers like Java).
- **`artifact_filename`**: Name given to the compiled executable/binary generated during compilation.
- **`artifact_filename_strategy`**:
  - `fixed`: Uses the static `artifact_filename` definition.
  - `from_request`: Extracted dynamically from requests.
- **`check`**: CLI argument passed to the executable to verify its availability during initial startup checks (e.g. `--version`).
- **`build`** (Optional): Specifies compilation rules:
  - `cmd`: The absolute path of the compiler executable on the host.
  - `args`: Compile flags and argument template slices.
  - `limits`: Strict CPU, memory, and process thresholds enforced *during the build step*.
  - `flag_allowlist`: List of strings or wildcards specifying compile arguments that the compiler will accept without throwing a validation error.
  - `flag_denylist` (Optional): List of strings or wildcards that are rejected even if they
    also match `flag_allowlist`. This exists to carve dangerous exceptions out of allowlist
    wildcards that are necessarily broad — see [Flag Denylist](#flag-denylist) below.
- **`run`**: Specifies execution rules. Takes the same properties as `build`
  (`cmd`, `args`, `limits`, `flag_allowlist`, `flag_denylist`) — the build and run phases
  each validate `build.flags`/`run.flags` against their *own* allowlist/denylist pair, so a
  flag permitted for one phase is not automatically permitted for the other.

---

## Template Placeholders

When invoking compilers and sandboxed processes, the arguments slices support string replacement placeholders:

| Placeholder | Substituted With |
| :--- | :--- |
| `{{source}}` | The sandbox-relative path of the source code file (e.g. `/solution.c` or `/Solution.java`). |
| `{{artifact}}` | The sandbox-relative *path* of the compiled executable/binary (e.g. `/solution` or `/Solution`). |
| `{{artifact_name}}` | The *bare* artifact filename, no leading `/` (e.g. `solution` or `Solution`) — for arguments that need a name rather than a path, such as Java's `java -cp <dir> <class>`, where the class name must not be prefixed with a slash. |
| `{{workdir}}` | The sandbox-relative working directory (always `/`). |
| `{{flags}}` | Plucked and validated compilation flags supplied in the request payload. |

---

## Flag Denylist

`flag_allowlist` wildcards are sometimes necessarily broad — a compiler groups dozens of
unrelated options under one prefix (Rust's `-C*` covers everything from `-C opt-level=3` to
`-C linker=...`). Where a handful of flags within such a wildcard are genuinely dangerous
rather than just unwanted, `flag_denylist` carves them back out. The denylist always wins,
even over an allowlist wildcard match — validation checks the denylist first.

`config/lang.yaml`'s Rust entry is the reference example, denylisting the `-C` sub-flags that
would let a submission execute an arbitrary binary or write outside the tracked build output:

```yaml
flag_denylist:
  - "-C linker=*"           # arbitrary binary invoked as the linker (e.g. /bin/sh)
  - "-C linker-flavor=*"    # changes how the linker is invoked, compounds with the above
  - "-C link-arg=*"         # arbitrary args passed straight to the linker
  - "-C link-args=*"
  - "-C extra-filename=*"   # output filename/path manipulation
  - "-C save-temps=*"       # writes extra files beyond the tracked artifact
  - "-C passes=*"           # raw LLVM pass injection
  - "-C llvm-args=*"        # raw args passed straight to LLVM
  - "-C profile-generate=*" # arbitrary output path
  - "-C profile-use=*"      # arbitrary input path
```

A language whose `flag_allowlist` has no risky wildcards (C, C++, Java, Verilog in the
default config all use only fixed flags or narrow wildcards like `-std=*`) has no need for a
`flag_denylist` at all — an empty/absent denylist is a no-op, not a security gap.

---

## How to Add a New Language

To add a new compiler/runtime to your sandbox engine:

### Step 1: Install the compiler on your host system
Ensure the binary executable (e.g. `/usr/bin/gfortran`) exists on the host machine or in the Docker environment.

### Step 2: Register in `lang.yaml`
Append a new block under `languages:` in `config/lang.yaml`:

```yaml
  - id: fortran
    name: Fortran 95
    source_filename: solution.f90
    source_filename_strategy: fixed
    artifact_filename: solution
    artifact_filename_strategy: fixed
    check: --version
    build:
      cmd: /usr/bin/gfortran
      args: ["{{flags}}", "-o", "{{artifact}}", "{{source}}"]
      limits:
        wall_time_s: 10
        memory_kb: 524288
        max_processes: 64
      flag_allowlist:
        - "-O0"
        - "-O2"
        - "-Wall"
    run:
      cmd: "{{artifact}}"
      args: []
      limits:
        wall_time_s: 5
        memory_kb: 131072
        max_processes: 32
```

### Step 3: Restart the Service
Rerun or reload the service to trigger the automatic compilation sanity checks and validation.
