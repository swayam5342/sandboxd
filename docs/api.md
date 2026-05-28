# REST API Reference

`sandboxd` exposes a lightweight, concurrent, and highly secure REST API over HTTP to compile, execute, and validate code submissions.

---

## Service Endpoints

### 1. Execute Code
- **Route**: `POST /run`
- **Content-Type**: `application/json`
- **Description**: Submits source code for compilation and execution against one or more test cases.

#### Request Headers
| Header | Value |
| :--- | :--- |
| `Content-Type` | `application/json` |

#### Request Payload Properties
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `language` | `string` | **Yes** | The target language ID (e.g. `py3`, `bash`, `node`, `c`, `cpp`, `java`, `verilog`, `rust`). |
| `source` | `string` | **Yes** | The source code of the program to run. Max size: `256 KB` (`MAX_SOURCE_SIZE`). |
| `source_filename` | `string` | No | Optional filename to use in sandbox. Required for Java if using `from_request` strategy. |
| `artifact_filename`| `string` | No | Optional compiled binary filename to use. Required for Java if using `from_request` strategy. |
| `build` | `object` | No | Optional configuration for the compiler phase. |
| `build.flags` | `array[string]`| No | Optional arguments passed to the compiler (subject to language allowlist validation). |
| `build.limits` | `object` | No | Optional resource limit overrides for the build phase. |
| `build.limits.wall_time_s` | `int` | No | Override compiler CPU wall-time limit in seconds. |
| `build.limits.memory_kb` | `int` | No | Override compiler maximum resident memory limit in Kilobytes. |
| `build.limits.max_processes`| `int` | No | Override compiler maximum process/thread limit. |
| `run` | `object` | No | Optional configuration for the execution/runtime phase. |
| `run.flags` | `array[string]`| No | Optional arguments passed to the executed binary/interpreter (subject to allowlist). |
| `run.limits` | `object` | No | Optional resource limit overrides for the execution phase. Supports identical options as `build.limits`. |
| `tests` | `array` | **Yes** | List of test case definitions. Max tests: `50` (`MAX_TEST_SIZE`). |
| `tests[].stdin` | `string` | **Yes** | The input fed into `stdin` for this test case. Max size: `64 KB` (`MAX_STDIN`). |
| `tests[].expected_stdout`| `string` | **Yes** | The expected standard output to validate against. |

#### Example Request Payload (C Program)
```json
{
  "language": "c",
  "source": "#include <stdio.h>\nint main() {\n    printf(\"Hello, World!\\n\");\n    return 0;\n}",
  "tests": [
    {
      "stdin": "",
      "expected_stdout": "Hello, World!\n"
    }
  ]
}
```

#### Example Response Payload (Successful Run)
```json
{
  "status": "accepted",
  "build": {
    "status": "ok",
    "stdout": "",
    "stderr": "",
    "duration_ms": 142
  },
  "tests": [
    {
      "status": "accepted",
      "stdout": "Hello, World!\n",
      "stderr": "",
      "exit_code": 0,
      "duration_ms": 4,
      "memory_peak_kb": 1284
    }
  ]
}
```

---

### 2. Service Telemetry
- **Route**: `GET /info`
- **Description**: Returns live system build statistics, global limit boundaries, and dynamic concurrency telemetry.

#### Example Response
```json
{
  "build_info": {
    "version": "dev",
    "commit": "unknown",
    "go_version": "go1.25.0"
  },
  "nsjail": {
    "path": "/usr/sbin/nsjail",
    "version": "nsjail v3.4"
  },
  "languages": [
    {
      "id": "py3",
      "name": "Python 3",
      "version": "Python 3.11.2",
      "default_run_limits": {
        "wall_time_s": 9,
        "memory_kb": 102400,
        "max_processes": 100
      }
    }
  ],
  "limits": {
    "max_source_bytes": 262144,
    "max_tests": 50,
    "max_concurrent_jobs": 100
  },
  "stats": {
    "in_flight_jobs": 0,
    "jobs_total": 42,
    "jobs_failed_internal": 0,
    "last_internal_error_at": null
  }
}
```

---

### 3. Health Diagnostics & Readiness Probes
- **Routes**: `GET /healthz` (Liveness) & `GET /readyz` (Readiness)
- **Description**: Verification probes returning system-wide or detailed status. Used by orchestration controllers to monitor readiness.

#### Example Response `GET /healthz`
- **Status**: `200 OK`
- **Body**:
```json
{
  "status": "ok"
}
```

#### Example Response `GET /readyz`
Checks NsJail availability and dynamically pings all configured compiler toolchains.
- **Status**: `200 OK` (All toolchains healthy) or `503 Service Unavailable` (If NsJail or any compiler fails probe)
- **Body**:
```json
{
  "status": "ok",
  "nsjail": {
    "ok": true,
    "version": "nsjail v3.4"
  },
  "languages": {
    "py3": {
      "ok": true,
      "version": "Python 3.11.2"
    },
    "c": {
      "ok": true,
      "version": "gcc (Debian 12.2.0-14) 12.2.0"
    }
  }
}
```

---

## Status Definitions

The response schema reports granular execution and compiler statuses matching our internal state machines:

### Top-Level Execution Statuses (`RunResponse.Status`)
| Status | Description |
| :--- | :--- |
| `accepted` | The code compiled successfully and all test outputs matched expected criteria. |
| `build_failed` | The compilation phase failed. Test cases were skipped. |
| `wrong_output` | The execution completed, but at least one test case's output did not match expected output. |
| `output_whitespace_mismatch` | The program output matches only after stripping trailing or leading whitespaces. |
| `time_exceeded` | The execution process exceeded the defined runtime CPU timeout window. |
| `memory_exceeded` | The process exceeded its sandbox cgroup or rlimit memory allocation. |
| `runtime_error` | The execution process crashed (SIGSEGV, non-zero exit code, etc.). |
| `internal_error` | A backend supervisor error occurred (e.g. filesystem failures, disk limits). |

### Phase Statuses (`BuildResult.Status` & `TestResult.Status`)
*   **Build Statuses**: `ok`, `failed`, `internal_error`
*   **Test Statuses**: `accepted`, `wrong_output`, `output_whitespace_mismatch`, `time_exceeded`, `memory_exceeded`, `runtime_error`, `not_executed`, `internal_error`
