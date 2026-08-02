# sandboxd

`sandboxd` is a highly secure, high-performance, multi-language compilation and execution daemon. It allows you to run untrusted code snippets concurrently inside a sandboxed environment isolated via **NsJail** (namespaces, cgroups, chroot, and resource limits) and exposes a RESTful JSON API.

---

## Table of Contents
* [Architecture, Design & Security Docs](#architecture-design--security-docs)
* [Supported Languages & Toolchains](#supported-languages--toolchains)
* [Prerequisites](#prerequisites)
* [Running the Service](#running-the-service)
  * [Option A: Running with Docker Compose (Recommended)](#option-a-running-with-docker-compose-recommended)
  * [Option B: Running Locally (Bare Metal)](#option-b-running-locally-bare-metal)
* [API Reference](#api-reference)
  * [`GET /healthz` - Health Check](#get-healthz---health-check)
  * [`GET /readyz` - Readiness Probe](#get-readyz---readiness-probe)
  * [`GET /info` - Server & Language Info](#get-info---server--language-info)
  * [`POST /run` - Execute Code Sandbox](#post-run---execute-code-sandbox)
* [Code Submission Examples (CURL)](#code-submission-examples-curl)
* [Verification & Testing](#verification--testing)

---

## Architecture, Design & Security Docs

Detailed production-grade guides are available in the `docs` directory:
1. **[REST API Reference](docs/api.md):** Complete specification of the `/healthz`, `/readyz`, `/info`, and `/run` HTTP routes and payloads.
2. **[System Architecture & Design](docs/architecture.md):** Breakdown of components, Go concurrency semaphore scheduling, and workflow sequence diagrams.
3. **[Language Registry Configuration](docs/languages.md):** The dynamic plug-and-play YAML compiler schema and placeholders.
4. **[Security Enforcement & Threat Model](docs/security.md):** Threat mitigations, kernel namespaces, chroot bind mounts, and memory/PID limits.

---

## Supported Languages & Toolchains

The service defines compile and runtime rules in `config/lang.yaml`. The pre-configured toolchains are:

| Language ID | Language Name | Default Strategy | Toolchain Commands |
| :--- | :--- | :--- | :--- |
| `py3` | Python 3 | Fixed filename | `/usr/bin/python3` |
| `bash` | Bash | Fixed filename | `/bin/bash` |
| `node` | JavaScript (Node) | Fixed filename | `/usr/bin/node` |
| `c` | C (GCC) | Fixed filename | `/usr/bin/gcc` (compiler) & Compiled binary (run) |
| `cpp` | C++ (G++) | Fixed filename | `/usr/bin/g++` (compiler) & Compiled binary (run) |
| `java` | Java (OpenJDK) | From request | `/usr/bin/javac` (compiler) & `/usr/bin/java` (run) |
| `verilog` | Verilog (Icarus) | Fixed filename | `/usr/bin/iverilog` (compiler) & `/usr/bin/vvp` (run) |
| `rust` | Rust (Rustc) | Fixed filename | `/usr/bin/rustc` (compiler) & Compiled binary (run) |

---

## Prerequisites

*   **Docker & Docker Compose** (highly recommended, includes NsJail and all compilers pre-bundled).
*   **Go 1.25+** (if compiling locally).
*   **NsJail** (if running locally, must be installed at `/usr/sbin/nsjail` or path set via `NSJAIL_PATH`).

> **Linux only.** `internal/runner` reads Linux-specific fields off `syscall.Rusage`
> for memory reporting, so `go build`/`go test` only work on Linux (or inside the
> Docker image). Building on macOS/Windows will fail — use Docker or WSL for local dev.

---

## Running the Service

### Option A: Running with Docker Compose (Recommended)

Running with Docker Compose is the easiest and most reliable method as it compiles NsJail, installs all necessary compilers/interpreters, and maps permissions automatically.

1.  **Clone and run the application:**
    ```bash
    docker compose up --build
    ```
    *Note: `docker-compose.yml` runs the container with `cap_drop: ALL` plus an explicit
    `cap_add` list (`SYS_ADMIN`, `SYS_CHROOT`, `SYS_RESOURCE`, `SYS_PTRACE`, `NET_ADMIN`,
    `SETUID`, `SETGID`, `SETPCAP`, `DAC_OVERRIDE`, `CHOWN`, `KILL`) and
    `seccomp:unconfined`/`apparmor:unconfined` — the minimum nsjail needs to create
    namespaces and chroot, rather than the full `privileged: true`.*

2.  **Set an API key (strongly recommended):**
    `POST /run` is unauthenticated unless the `API_KEY` environment variable is set. If it's
    unset, the server logs a warning at startup and accepts unauthenticated requests — fine
    for local development, not for anything reachable outside your own machine. Set it via
    the `environment:` block in `docker-compose.yml` or a `.env` file, then send it as
    `Authorization: Bearer <API_KEY>` on every `/run` request.

3.  **Verify the service is up:**
    ```bash
    curl http://localhost:8089/healthz
    # Expected response: {"status":"ok"}
    ```

### Option B: Running Locally (Bare Metal)

If you have Go, NsJail, and the compilers installed on your Linux machine (or inside WSL2):

1.  **Configure Environment Variables:**
    Create a `.env` file in the root directory (see `example.env` for the full list of
    variables and their defaults):
    ```env
    PORT=:8089
    LOG_LEVEL=info
    LOG_JSON=false
    NSJAIL_PATH=/usr/sbin/nsjail
    NSJAIL_BASE_DIR=/tmp/sandboxd-jails
    LANG_CONFIG=config/lang.yaml
    MAX_CONCURRENT=100
    API_KEY=
    ```

2.  **Run the Go application:**
    ```bash
    go run cmd/sandbox/main.go
    ```
    *During startup, the application runs startup probes. If `nsjail` or any configured language toolchain is missing, the server will output a detailed error log and exit.*

---

## API Reference

### `GET /healthz` - Health Check
Verifies the REST server is alive.
*   **Status Code:** `200 OK`
*   **Response:**
    ```json
    { "status": "ok" }
    ```

### `GET /readyz` - Readiness Probe
Runs checks on NsJail and each configured language compiler. Returns `200 OK` if all are healthy, or `503 Service Unavailable` with details of failed compilers.
*   **Status Code:** `200 OK` (Healthy) or `503 Service Unavailable` (Degraded)
*   **Response:**
    ```json
    {
      "status": "ok",
      "nsjail": { "ok": true, "version": "nsjail v3.4" },
      "languages": {
        "py3": { "ok": true, "version": "Python 3.11.2" },
        "c": { "ok": true, "version": "gcc (Debian 12.2.0-14) 12.2.0" }
      }
    }
    ```

### `GET /info` - Server & Language Info
Returns system statistics, global limit boundaries, build metadata, and runtime usage statistics.
*   **Status Code:** `200 OK`
*   **Response (abbreviated):**
    ```json
    {
      "build_info": { "version": "dev", "commit": "unknown", "go_version": "go1.25.0" },
      "nsjail": { "path": "/usr/sbin/nsjail", "version": "nsjail v3.4" },
      "limits": { "max_source_bytes": 262144, "max_tests": 50, "max_concurrent_jobs": 100 },
      "stats": { "in_flight_jobs": 0, "jobs_total": 42, "jobs_failed_internal": 0 }
    }
    ```

### `POST /run` - Execute Code Sandbox
Accepts source code, test inputs, and optional limit overrides to execute inside the sandbox jail.
*   **Authorization:** Required only if `API_KEY` is set on the server — send `Authorization: Bearer <API_KEY>`.
    A missing/wrong key returns `401 Unauthorized`.
*   **Status Code:** `200 OK`
*   **Payload Schema (`models.RunRequest`):**
    ```json
    {
      "language": "py3",
      "source": "import sys\nprint('Hello ' + sys.stdin.read())",
      "tests": [
        { "stdin": "World", "expected_stdout": "Hello World" }
      ],
      "build": {
        "flags": []
      },
      "run": {
        "flags": []
      }
    }
    ```
*   **Response Schema (`models.RunResponse`):**
    ```json
    {
      "status": "accepted",
      "build": null,
      "tests": [
        {
          "status": "accepted",
          "stdout": "Hello World",
          "stderr": "",
          "Exitcode": 0,
          "duration_ms": 35,
          "memory_peak_kb": 3120
        }
      ]
    }
    ```
    *Note: the exit-code field is serialized as `Exitcode` (capitalized, no underscore) —
    an inconsistency with the rest of the snake_case response fields, kept as-is here
    because it reflects the current API's actual JSON output.*
*   **Error Response (4xx/5xx):**
    ```json
    { "error": { "code": "disallowed_flag", "message": "flag \"-fsanitize=address\" is not on the allowlist for this language" } }
    ```
    Common `error.code` values: `missing_field`, `invalid_json`, `unknown_language`,
    `invalid_filename`, `source_too_large`, `too_many_tests`, `disallowed_flag`,
    `denied_flag` (explicitly blocked even though it matches an allowlist wildcard —
    see `flag_denylist` in [docs/languages.md](docs/languages.md)), `unauthorized`,
    `internal_error`.

---

## Code Submission Examples (CURL)

### Python 3 (`py3`) Example
```bash
curl -X POST http://localhost:8089/run \
  -H "Content-Type: application/json" \
  -d '{
    "language": "py3",
    "source": "import sys\nn = int(sys.stdin.read())\nprint(n * 2)",
    "tests": [
      { "stdin": "21", "expected_stdout": "42\n" }
    ]
  }'
```

### Rust (`rust`) Example
```bash
curl -X POST http://localhost:8089/run \
  -H "Content-Type: application/json" \
  -d '{
    "language": "rust",
    "source": "use std::io::{self, Read};\nfn main() {\n    let mut input = String::new();\n    io::stdin().read_to_string(&mut input).unwrap();\n    let num: i32 = input.trim().parse().unwrap();\n    println!(\"{}\", num + 1);\n}",
    "tests": [
      { "stdin": "9", "expected_stdout": "10\n" }
    ]
  }'
```

### Java (`java`) Example
*Note: Java requires class and filename matching. Submit custom filename strategy parameters:*
```bash
curl -X POST http://localhost:8089/run \
  -H "Content-Type: application/json" \
  -d '{
    "language": "java",
    "source_filename": "Solution.java",
    "artifact_filename": "Solution",
    "source": "import java.util.Scanner;\npublic class Solution {\n    public static void main(String[] args) {\n        Scanner sc = new Scanner(System.in);\n        int a = sc.nextInt();\n        System.out.println(a * a);\n    }\n}",
    "tests": [
      { "stdin": "5", "expected_stdout": "25\n" }
    ]
  }'
```

---

## Verification & Testing

To run the automated suite of unit and integration sandbox execution tests:

```bash
# Run all unit tests
go test -v ./internal/...

# Run sandbox specific tests (requires nsjail and toolchains installed locally)
go test -v ./internal/runner
```
