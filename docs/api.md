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
| `Authorization` | `Bearer <API_KEY>` — required only if the server was started with an `API_KEY` set. A missing or wrong key returns `401 Unauthorized`. If `API_KEY` is unset on the server, this route is unauthenticated. |

The request body is capped at 512 KiB (`http.MaxBytesReader`); a larger body is rejected with `413 Request Entity Too Large` before any parsing happens.

#### Request Payload Properties
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `language` | `string` | **Yes** | The target language ID (e.g. `py3`, `bash`, `node`, `c`, `cpp`, `java`, `verilog`, `rust`). |
| `source` | `string` | **Yes** | The source code of the program to run. Max size: `256 KB` (`MAX_SOURCE_SIZE`). |
| `source_filename` | `string` | No | Optional filename to use in sandbox. Required for Java if using `from_request` strategy. |
| `artifact_filename`| `string` | No | Optional compiled binary filename to use. Required for Java if using `from_request` strategy. |
| `build` | `object` | No | Optional configuration for the compiler phase. |
| `build.flags` | `array[string]`| No | Optional arguments passed to the compiler. Validated against that language's build-phase `flag_allowlist`, then against its `flag_denylist` (deny always wins, even over an allowlist wildcard match — see [docs/languages.md](languages.md)). |
| `build.limits` | `object` | No | Optional resource limit overrides for the build phase. |
| `build.limits.wall_time_s` | `int` | No | Override compiler CPU wall-time limit in seconds. |
| `build.limits.memory_kb` | `int` | No | Override compiler maximum resident memory limit in Kilobytes. |
| `build.limits.max_processes`| `int` | No | Override compiler maximum process/thread limit. |
| `run` | `object` | No | Optional configuration for the execution/runtime phase. |
| `run.flags` | `array[string]`| No | Optional arguments passed to the executed binary/interpreter. Validated against that language's run-phase allowlist/denylist — a separate list from the build phase's. |
| `run.limits` | `object` | No | Optional resource limit overrides for the execution phase. Supports identical options as `build.limits`. |
| `tests` | `array` | **Yes** | List of test case definitions. Max tests: `50` (`MAX_TEST_SIZE`). |
| `tests[].stdin` | `string` | **Yes** | The input fed into `stdin` for this test case. Max size: `64 KB` (`MAX_STDIN`). |
| `tests[].expected_stdout`| `string` | **Yes** | The expected standard output to validate against. |

**Limit overrides can only tighten, never loosen, a language's configured default.** Any
`*.limits.*` value you send is clamped into `[1, <language default>]` — sending `0`, a
negative number, or a value above the default all clamp down to `1` or the default
respectively rather than erroring or disabling the limit. There is no way to request a
*larger* limit than the language config allows.

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
      "Exitcode": 0,
      "duration_ms": 4,
      "memory_peak_kb": 1284
    }
  ]
}
```
*Note: the exit-code field is serialized as `Exitcode` (capitalized, no underscore) — an
inconsistency with the rest of the snake_case fields, documented here as-is because it
reflects the current API's actual output rather than the intended naming.*

#### Error Responses

Any validation failure or internal error returns a JSON body of the same shape, regardless
of which endpoint produced it:
```json
{ "error": { "code": "disallowed_flag", "message": "flag \"-fsanitize=address\" is not on the allowlist for this language" } }
```

| HTTP Status | `error.code` | Cause |
| :--- | :--- | :--- |
| `400` | `missing_field` | Required field absent (`language`, `source`, `tests`, or a Java `from_request` filename). |
| `400` | `invalid_json` | Request body is not valid JSON. |
| `400` | `unknown_language` | `language` doesn't match any entry in `config/lang.yaml`. |
| `400` | `invalid_filename` | `source_filename`/`artifact_filename` fails the path-safety check (separators, leading dot, disallowed characters, too long). |
| `400` | `source_too_large` | `source` exceeds `MAX_SOURCE_SIZE`, or a test's `stdin` exceeds `MAX_STDIN`. |
| `400` | `too_many_tests` | `tests` exceeds `MAX_TEST_SIZE`. |
| `400` | `disallowed_flag` | A `build.flags`/`run.flags` entry isn't on that phase's `flag_allowlist`. |
| `400` | `denied_flag` | A flag is explicitly blocked by that phase's `flag_denylist`, even though it also matches an allowlist wildcard. |
| `401` | `unauthorized` | Missing/invalid `Authorization: Bearer <API_KEY>` on `/run` (only when `API_KEY` is configured on the server). |
| `413` | `source_too_large` | Request body exceeds the 512 KiB `MaxBodySize` cap (checked before JSON decoding). |
| `500` | `internal_error` | An unhandled failure in the runner (nsjail invocation failure, filesystem error, etc.) — not caused by the submitted request. |

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
| `time_exceeded` | The process was killed for exceeding its wall-time limit. |
| `memory_exceeded` | The process was killed for exceeding its memory limit. |
| `runtime_error` | The execution process crashed or exited non-zero for a reason other than a resource limit (SIGSEGV, uncaught exception, `exit(1)`, etc.). |
| `internal_error` | A backend supervisor error occurred (e.g. filesystem failures, nsjail invocation failure) — not caused by the submitted code. |

`time_exceeded`/`memory_exceeded` detection: nsjail reuses exit codes 2 and 3 both as its own
"I killed this for a limit violation" signal, and as a plain passthrough of a child process
that happened to exit normally with that same code. To avoid misreporting an ordinary
`exit(2)`/`exit(3)` as a limit violation, the server cross-checks the measured wall-clock
duration (for exit code 2) or peak memory (for exit code 3) against ≥90% of the configured
limit before assigning `time_exceeded`/`memory_exceeded` — otherwise it falls back to
`runtime_error`.

### Phase Statuses (`BuildResult.Status` & `TestResult.Status`)
*   **Build Statuses**: `ok`, `failed`, `internal_error`
*   **Test Statuses**: `accepted`, `wrong_output`, `output_whitespace_mismatch`, `time_exceeded`, `memory_exceeded`, `runtime_error`, `not_executed`, `internal_error`
