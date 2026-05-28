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
- **`run`**: Specifies execution rules (takes identical properties as `build` except for `flag_allowlist`).

---

## Template Placeholders

When invoking compilers and sandboxed processes, the arguments slices support string replacement placeholders:

| Placeholder | Substituted With |
| :--- | :--- |
| `{{source}}` | The sandbox-relative path of the source code file (e.g. `/solution.c` or `/Solution.java`). |
| `{{artifact}}` | The sandbox-relative path of the compiled executable/binary (e.g. `/solution` or `/Solution`). |
| `{{workdir}}` | The sandbox-relative working directory (always `/`). |
| `{{flags}}` | Plucked and validated compilation flags supplied in the request payload. |

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
