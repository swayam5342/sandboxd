<div align="center">

<img src="site/public/icon.svg" width="88" height="88" alt="sandboxd">

# sandboxd

**Run code you don't trust.**

`sandboxd` is a highly secure, high-performance, multi-language compilation and execution daemon. It allows you to run untrusted code snippets concurrently inside a sandboxed environment isolated via **NsJail** (namespaces, cgroups, chroot, and resource limits) and exposes a RESTful JSON API.

[![Go](https://img.shields.io/badge/Go-1.25%2B-4cb3f0?style=flat-square&logo=go&logoColor=white&labelColor=0d3a55)](go.mod)
[![Platform](https://img.shields.io/badge/platform-Linux-4cb3f0?style=flat-square&logo=linux&logoColor=white&labelColor=0d3a55)](#prerequisites)
[![Isolation](https://img.shields.io/badge/isolation-NsJail-4cb3f0?style=flat-square&labelColor=0d3a55)](docs/security.md)
[![Docker](https://img.shields.io/badge/run%20with-Docker%20Compose-4cb3f0?style=flat-square&logo=docker&logoColor=white&labelColor=0d3a55)](#option-a-running-with-docker-compose-recommended)
[![Docker Hub](https://img.shields.io/docker/v/swayam5342/sandboxd?sort=date&style=flat-square&logo=docker&logoColor=white&label=docker%20hub&labelColor=0d3a55&color=4cb3f0)](https://hub.docker.com/r/swayam5342/sandboxd)
[![Image size](https://img.shields.io/docker/image-size/swayam5342/sandboxd/core?style=flat-square&labelColor=0d3a55&color=4cb3f0)](https://hub.docker.com/r/swayam5342/sandboxd/tags)

[**Website**](https://sandboxd.swayam.li/) &nbsp;&middot;&nbsp; [API](docs/api.md) &nbsp;&middot;&nbsp; [Architecture](docs/architecture.md) &nbsp;&middot;&nbsp; [Languages](docs/languages.md) &nbsp;&middot;&nbsp; [Security](docs/security.md)

</div>

<br>

## Table of Contents

| | |
| :--- | :--- |
| **Overview** | [Architecture, Design & Security Docs](#architecture-design--security-docs) &middot; [Supported Languages & Toolchains](#supported-languages--toolchains) |
| **Setup** | [Prerequisites](#prerequisites) &middot; [Docker Image](#docker-image) &middot; [Running the Service](#running-the-service) &middot; [Option A: Docker Compose (Recommended)](#option-a-running-with-docker-compose-recommended) &middot; [Option B: Locally (Bare Metal)](#option-b-running-locally-bare-metal) |
| **API** | [API Reference](#api-reference) &middot; [`GET /healthz`](#get-healthz---health-check) &middot; [`GET /readyz`](#get-readyz---readiness-probe) &middot; [`GET /info`](#get-info---server--language-info) &middot; [`POST /run`](#post-run---execute-code-sandbox) |
| **Examples** | [Code Submission Examples (CURL)](#code-submission-examples-curl) &middot; [Verification & Testing](#verification--testing) |

<br>

## Architecture, Design & Security Docs

Detailed production-grade guides are available in the `docs` directory:

| | Guide | What's inside |
| :---: | :--- | :--- |
| 1 | **[REST API Reference](docs/api.md)** | Complete specification of the `/healthz`, `/readyz`, `/info`, and `/run` HTTP routes and payloads. |
| 2 | **[System Architecture & Design](docs/architecture.md)** | Breakdown of components, Go concurrency semaphore scheduling, and workflow sequence diagrams. |
| 3 | **[Language Registry Configuration](docs/languages.md)** | The dynamic plug-and-play YAML compiler schema and placeholders. |
| 4 | **[Security Enforcement & Threat Model](docs/security.md)** | Threat mitigations, kernel namespaces, chroot bind mounts, and memory/PID limits. |

<br>

## Supported Languages & Toolchains

The service defines compile and runtime rules in [`config/lang.yaml`](config/lang.yaml). The pre-configured toolchains are:

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

<br>

## Prerequisites

| Requirement | When |
| :--- | :--- |
| **Docker & Docker Compose** | Highly recommended, includes NsJail and all compilers pre-bundled. |
| **Go 1.25+** | If compiling locally. |
| **NsJail** | If running locally, must be installed at `/usr/sbin/nsjail` or path set via `NSJAIL_PATH`. |

> [!WARNING]
> **Linux only.** `internal/runner` reads Linux-specific fields off `syscall.Rusage`
> for memory reporting, so `go build`/`go test` only work on Linux (or inside the
> Docker image). Building on macOS/Windows will fail — use Docker or WSL for local dev.

<br>

## Docker Image

The published image is a small **core**: sandboxd and NsJail, with **bash** as its only
language. You add the toolchains you want on top of it, so an image that only runs Python
doesn't carry a JDK and a Rust compiler.

| Image | What's inside | Size |
| :--- | :--- | :--- |
| [`swayam5342/sandboxd:core`](https://hub.docker.com/r/swayam5342/sandboxd) | sandboxd, NsJail, bash | ~145 MB |
| `swayam5342/sandboxd:core-<commit>` | the same, pinned to one commit. Use this in production. | ~145 MB |

> [!IMPORTANT]
> Toolchains must live **inside** the image. Every jail bind-mounts the container's own `/usr`, `/lib` and
> `/bin` read-only, so the compiler a submission sees is the container's compiler. sandboxd also checks every
> language in its `lang.yaml` at startup and exits if one is missing.

**Run the core image** (bash only), with the same capability set `docker-compose.yml` uses (nsjail needs these to build a jail; `--privileged` is not required):

```bash
docker run -d --name sandboxd -p 8089:8089 \
  --cap-drop ALL \
  --cap-add SYS_ADMIN --cap-add SYS_CHROOT --cap-add SYS_RESOURCE --cap-add SYS_PTRACE \
  --cap-add NET_ADMIN --cap-add SETUID --cap-add SETGID --cap-add SETPCAP \
  --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add KILL \
  --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -e API_KEY=change-me \
  swayam5342/sandboxd:core

curl http://localhost:8089/healthz
# {"status":"ok"}
```

**Add your languages** by building on core. Install the toolchains, then replace the registry with your own [`lang.yaml`](docs/languages.md):

```dockerfile
FROM swayam5342/sandboxd:core
RUN apt-get update && apt-get install -y --no-install-recommends python3 \
    && rm -rf /var/lib/apt/lists/*
COPY lang.yaml /app/config/lang.yaml   # bash + py3, for example
```

**All eight languages:** [`docker/full.Dockerfile`](docker/full.Dockerfile) is the complete example. It pulls core,
installs Python, Node.js, GCC/G++, OpenJDK 17, Icarus Verilog and Rust, and copies in [`config/lang.yaml`](config/lang.yaml).
`docker-compose.yml` builds it, so `docker compose up --build` gives you every language in the table above.

> [!TIP]
> `GET /info` reports the image's version and commit under `build_info`.

<br>

## Running the Service

### Option A: Running with Docker Compose (Recommended)

Running with Docker Compose is the easiest and most reliable method as it pulls the core image (NsJail prebuilt), installs all necessary compilers/interpreters on top ([`docker/full.Dockerfile`](docker/full.Dockerfile)), and maps permissions automatically.

**1. Clone and run the application**

```bash
docker compose up --build
```

> [!NOTE]
> `docker-compose.yml` runs the container with `cap_drop: ALL` plus an explicit
> `cap_add` list (`SYS_ADMIN`, `SYS_CHROOT`, `SYS_RESOURCE`, `SYS_PTRACE`, `NET_ADMIN`,
> `SETUID`, `SETGID`, `SETPCAP`, `DAC_OVERRIDE`, `CHOWN`, `KILL`) and
> `seccomp:unconfined`/`apparmor:unconfined` — the minimum nsjail needs to create
> namespaces and chroot, rather than the full `privileged: true`.

**2. Set an API key (strongly recommended)**

> [!IMPORTANT]
> `POST /run` is unauthenticated unless the `API_KEY` environment variable is set. If it's
> unset, the server logs a warning at startup and accepts unauthenticated requests — fine
> for local development, not for anything reachable outside your own machine. Set it via
> the `environment:` block in `docker-compose.yml` or a `.env` file, then send it as
> `Authorization: Bearer <API_KEY>` on every `/run` request.

**3. Verify the service is up**

```bash
curl http://localhost:8089/healthz
# Expected response: {"status":"ok"}
```

### Option B: Running Locally (Bare Metal)

If you have Go, NsJail, and the compilers installed on your Linux machine (or inside WSL2):

**1. Configure Environment Variables**

Create a `.env` file in the root directory (see [`example.env`](example.env) for the full list of variables and their defaults):

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

**2. Run the Go application**

```bash
go run cmd/sandbox/main.go
```

> [!TIP]
> During startup, the application runs startup probes. If `nsjail` or any configured language toolchain is missing, the server will output a detailed error log and exit.

<br>

## API Reference

| Method | Route | Purpose | Auth |
| :--- | :--- | :--- | :--- |
| `GET` | [`/healthz`](#get-healthz---health-check) | Health Check | — |
| `GET` | [`/readyz`](#get-readyz---readiness-probe) | Readiness Probe | — |
| `GET` | [`/info`](#get-info---server--language-info) | Server & Language Info | — |
| `POST` | [`/run`](#post-run---execute-code-sandbox) | Execute Code Sandbox | `Bearer <API_KEY>` if set |

### `GET /healthz` - Health Check

Verifies the REST server is alive.

**Status Code:** `200 OK`

```json
{ "status": "ok" }
```

### `GET /readyz` - Readiness Probe

Runs checks on NsJail and each configured language compiler. Returns `200 OK` if all are healthy, or `503 Service Unavailable` with details of failed compilers.

**Status Code:** `200 OK` (Healthy) or `503 Service Unavailable` (Degraded)

<details>
<summary><b>Response</b></summary>

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

</details>

### `GET /info` - Server & Language Info

Returns system statistics, global limit boundaries, build metadata, and runtime usage statistics.

**Status Code:** `200 OK`

<details>
<summary><b>Response (abbreviated)</b></summary>

```json
{
  "build_info": { "version": "dev", "commit": "unknown", "go_version": "go1.25.0" },
  "nsjail": { "path": "/usr/sbin/nsjail", "version": "nsjail v3.4" },
  "limits": { "max_source_bytes": 262144, "max_tests": 50, "max_concurrent_jobs": 100 },
  "stats": { "in_flight_jobs": 0, "jobs_total": 42, "jobs_failed_internal": 0 }
}
```

</details>

### `POST /run` - Execute Code Sandbox

Accepts source code, test inputs, and optional limit overrides to execute inside the sandbox jail.

| | |
| :--- | :--- |
| **Authorization** | Required only if `API_KEY` is set on the server — send `Authorization: Bearer <API_KEY>`. A missing/wrong key returns `401 Unauthorized`. |
| **Status Code** | `200 OK` |

<details open>
<summary><b>Payload Schema</b> (<code>models.RunRequest</code>)</summary>

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

</details>

<details open>
<summary><b>Response Schema</b> (<code>models.RunResponse</code>)</summary>

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

</details>

> [!NOTE]
> The exit-code field is serialized as `Exitcode` (capitalized, no underscore) —
> an inconsistency with the rest of the snake_case response fields, kept as-is here
> because it reflects the current API's actual JSON output.

<details>
<summary><b>Error Response (4xx/5xx)</b></summary>

```json
{ "error": { "code": "disallowed_flag", "message": "flag \"-fsanitize=address\" is not on the allowlist for this language" } }
```

Common `error.code` values: `missing_field`, `invalid_json`, `unknown_language`,
`invalid_filename`, `source_too_large`, `too_many_tests`, `disallowed_flag`,
`denied_flag` (explicitly blocked even though it matches an allowlist wildcard —
see `flag_denylist` in [docs/languages.md](docs/languages.md)), `unauthorized`,
`internal_error`.

</details>

<br>

## Code Submission Examples (CURL)

<details open>
<summary><b>Python 3</b> (<code>py3</code>)</summary>

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

</details>

<details>
<summary><b>Rust</b> (<code>rust</code>)</summary>

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

</details>

<details>
<summary><b>Java</b> (<code>java</code>)</summary>

> [!NOTE]
> Java requires class and filename matching. Submit custom filename strategy parameters:

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

</details>

<br>

## Verification & Testing

To run the automated suite of unit and integration sandbox execution tests:

```bash
# Run all unit tests
go test -v ./internal/...

# Run sandbox specific tests (requires nsjail and toolchains installed locally)
go test -v ./internal/runner
```
