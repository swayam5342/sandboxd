// Content drawn from the sandboxd source: config/lang.yaml, internal/runner, internal/validator.
// Response figures (duration_ms, memory_peak_kb) are typical values; a real run will differ.

export const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
export const asset = (p: string) => `${base}/assets/${p}`;
export const REPO = "https://github.com/swayam5342/sandboxd";

export type Attempt = {
  id: string;
  label: string;
  lang: string;
  source: string;
  extra?: string;
  status: string;
  kind: "limit" | "caught" | "error";
  by: string;
  why: string;
  http: number;
  response: unknown;
};

const test = (o: Record<string, unknown>) => ({
  status: o.status,
  stdout: o.stdout ?? "",
  stderr: o.stderr ?? "",
  Exitcode: o.Exitcode,
  duration_ms: o.duration_ms,
  memory_peak_kb: o.memory_peak_kb,
});

export const attempts: Attempt[] = [
  {
    id: "loop", label: "Loop forever", lang: "py3",
    source: "while True:\n    pass",
    status: "time_exceeded", kind: "limit",
    by: "--time_limit 9",
    why: "Killed at the 9 second wall clock limit set for py3.",
    http: 200,
    response: { status: "time_exceeded", build: null, tests: [test({ status: "time_exceeded", Exitcode: 2, duration_ms: 9011, memory_peak_kb: 8920 })] },
  },
  {
    id: "fork", label: "Fork bomb", lang: "bash",
    source: ":(){ :|:& };:",
    status: "time_exceeded", kind: "limit",
    by: "--rlimit_nproc 50",
    why: "Bash may hold 50 processes. Every fork after that fails until the clock runs out.",
    http: 200,
    response: { status: "time_exceeded", build: null, tests: [test({ status: "time_exceeded", stderr: "solution.sh: fork: retry: Resource temporarily unavailable\n…", Exitcode: 2, duration_ms: 9004, memory_peak_kb: 41216 })] },
  },
  {
    id: "net", label: "Phone home", lang: "py3",
    source: 'import socket\nsocket.create_connection(("1.1.1.1", 443), timeout=3)',
    status: "runtime_error", kind: "caught",
    by: "iface_no_lo",
    why: "The jail has no network interface, so there is no route anywhere.",
    http: 200,
    response: { status: "runtime_error", build: null, tests: [test({ status: "runtime_error", stderr: "Traceback (most recent call last):\n  …\nOSError: [Errno 101] Network is unreachable\n", Exitcode: 1, duration_ms: 61, memory_peak_kb: 9340 })] },
  },

  {
    id: "mem", label: "Eat the memory", lang: "py3",
    source: "hog = bytearray(1 << 30)  # 1 GiB",
    status: "memory_exceeded", kind: "limit",
    by: "--cgroup_mem_max 100 MB",
    why: "py3 gets 100 MB. Where cgroup v2 is missing, RLIMIT_AS still stands behind it.",
    http: 200,
    response: { status: "memory_exceeded", build: null, tests: [test({ status: "memory_exceeded", Exitcode: 3, duration_ms: 142, memory_peak_kb: 102400 })] },
  },
  {
    id: "exit3", label: "Fake a kill", lang: "c",
    source: "#include <stdlib.h>\nint main(void) { exit(3); }",
    status: "runtime_error", kind: "caught",
    by: "the 90% cross-check",
    why: "nsjail also exits 3 for memory kills. A 1 MB peak is nowhere near 90% of the limit, so this stays runtime_error.",
    http: 200,
    response: { status: "runtime_error", build: { status: "ok", stdout: "", stderr: "", duration_ms: 214 }, tests: [test({ status: "runtime_error", Exitcode: 3, duration_ms: 4, memory_peak_kb: 1180 })] },
  },
  {
    id: "linker", label: "Swap the linker", lang: "rust",
    source: "fn main() {}",
    extra: '"build": { "flags": ["-C linker=/bin/sh"] }',
    status: "denied_flag", kind: "error",
    by: "flag_denylist",
    why: "rustc may take -C*, but -C linker=* is carved back out. Nothing is compiled.",
    http: 400,
    response: { error: { code: "denied_flag", message: 'flag "-C linker=/bin/sh" is explicitly denied for this language' } },
  },
];

export type Lang = { id: string; name: string; yaml: string; fact: string; tone: "ink" | "accent" | "deep" | "own"; bar?: string };

export const languages: Lang[] = [
  {
    id: "py3", name: "Python 3", tone: "accent",
    fact: "No build step. Runs for at most 9 seconds in 100 MB, with up to 100 processes.",
    yaml: `- id: py3
  name: Python 3
  source_filename: solution.py
  check: --version
  run:
    cmd: /usr/bin/python3
    args: ["{{source}}"]
    limits:
      wall_time_s: 9
      memory_kb: 102400
      max_processes: 100`,
  },
  {
    id: "bash", name: "Bash", tone: "deep",
    fact: "Runs for at most 9 seconds in 64 MB. Only 50 processes, so a fork bomb starves early.",
    yaml: `- id: bash
  name: Bash
  source_filename: solution.sh
  check: --version
  run:
    cmd: /bin/bash
    args: ["{{source}}"]
    limits:
      wall_time_s: 9
      memory_kb: 65536
      max_processes: 50`,
  },
  {
    id: "node", name: "JavaScript", tone: "ink",
    fact: "Node gets 9 seconds and 256 MB, because V8 reserves a lot before it does anything.",
    yaml: `- id: node
  name: JavaScript (Node)
  source_filename: solution.js
  check: --version
  run:
    cmd: /usr/bin/node
    args: ["{{source}}"]
    limits:
      wall_time_s: 9
      memory_kb: 262144
      max_processes: 100`,
  },
  {
    id: "c", name: "C", tone: "ink",
    fact: "Compiled by gcc in at most 10 seconds and 512 MB, then run for at most 5 seconds in 256 MB. Each phase gets its own jail.",
    yaml: `- id: c
  name: C
  build:
    cmd: /usr/bin/gcc
    limits:
      wall_time_s: 10
      memory_kb: 524288
    flag_allowlist: [-O0, -O1, -O2, -O3,
      -Wall, -Wextra, -Werror, "-std=*",
      -lm, -lpthread]
  run:
    cmd: "{{artifact}}"
    limits:
      wall_time_s: 5
      memory_kb: 262144
      max_processes: 64`,
  },
  {
    id: "cpp", name: "C++", tone: "accent",
    fact: "g++ gets 10 seconds and a full 1 GB to compile templates; the binary runs for 5 seconds in 512 MB.",
    yaml: `- id: cpp
  name: C++
  build:
    cmd: /usr/bin/g++
    limits:
      wall_time_s: 10
      memory_kb: 1048576
    flag_allowlist: [-O0, -O1, -O2, -O3,
      -Wall, -Wextra, -Werror, "-std=*",
      -lm, -lpthread]
  run:
    cmd: "{{artifact}}"
    limits:
      wall_time_s: 5
      memory_kb: 524288
      max_processes: 64`,
  },
  {
    id: "java", name: "Java", tone: "deep",
    fact: "The class name comes from your request. javac gets 15 seconds; the JVM runs capped at -Xmx256m.",
    yaml: `- id: java
  name: Java
  source_filename_strategy: from_request
  build:
    cmd: /usr/bin/javac
    args: ["-J-Xmx384m", "{{flags}}",
           "{{source}}"]
    limits:
      wall_time_s: 15
      memory_kb: 524288
    flag_allowlist: [-g, -nowarn, -verbose,
      "-source=*", "-target=*"]
  run:
    cmd: /usr/bin/java
    args: ["-Xmx256m", "-cp", "{{workdir}}",
           "{{artifact_name}}"]
    limits: { wall_time_s: 10 }`,
  },
  {
    id: "verilog", name: "Verilog", tone: "ink",
    fact: "Icarus compiles to a .vvp file in 10 seconds, then vvp simulates it for at most 5 seconds in 128 MB.",
    yaml: `- id: verilog
  name: Verilog
  source_filename: solution.v
  artifact_filename: solution.vvp
  build:
    cmd: /usr/bin/iverilog
    limits:
      wall_time_s: 10
      memory_kb: 262144
    flag_allowlist: [-g2012, -Wall,
      -Winfloop, -DTEST]
  run:
    cmd: /usr/bin/vvp
    args: ["{{artifact}}"]
    limits:
      wall_time_s: 5
      memory_kb: 131072`,
  },
  {
    id: "rust", name: "Rust", tone: "accent",
    fact: "rustc gets the longest build of all: 30 seconds and 2 GB. Ten -C flags are denied outright.",
    yaml: `- id: rust
  name: Rust
  build:
    cmd: /usr/bin/rustc
    limits:
      wall_time_s: 30
      memory_kb: 2097152
    flag_allowlist: [-O, "-C*", -g,
      "--edition=*", "-A*", "-D*", "-W*"]
    flag_denylist:
      - "-C linker=*"
      - "-C link-arg=*"
      - "-C llvm-args=*"
      - …and seven more
  run:
    cmd: "{{artifact}}"
    limits: { wall_time_s: 5 }`,
  },
  {
    id: "own", name: "Your own", tone: "own", bar: "config/lang.yaml: adding Lua",
    fact: "Lua isn’t included; this is how you would add it. Paste an entry like this into config/lang.yaml, install lua in the Docker image, and restart. On start, sandboxd runs lua -v to make sure it works.",
    yaml: `- id: lua
  name: Lua
  source_filename: solution.lua
  source_filename_strategy: fixed
  check: -v
  run:
    cmd: /usr/bin/lua
    args: ["{{source}}"]
    limits:
      wall_time_s: 5
      memory_kb: 65536
      max_processes: 16`,
  },
];
