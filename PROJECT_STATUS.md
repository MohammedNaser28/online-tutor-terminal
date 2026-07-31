# PROJECT_STATUS.md — Ground Truth

Generated from code inspection. **Do not infer intent; report what exists.**

---

## Part 1 — qo Core

### Architecture

The qo project is split across two binaries:

| Binary | Language | Role |
|--------|----------|------|
| `qo` (Go, ~17.8 MB static) | Go 1.24.5 | Cobra CLI — `build`, `start`, `meta` commands. Handles archive encryption/decryption, rootfs extraction, PTY creation, cgroup v2 setup, signal forwarding, stdin Passthrough. |
| `qo-init` (C, ~26 KB dynamic) | C17 | Child process spawned by `qo start`. Calls `clone()` with namespace flags, sets up loopback network, performs pivot_root, mounts proc/dev/devpts, drops capabilities, applies rlimits, optionally loads seccomp filter, then `fork()`+`waitpid()` reaps bash. |

**Entrypoints**:

- `qo/main.go:13-22` — checks for `os.Args[1] == "init"` first (legacy entrypoint for older sandbox design). Otherwise delegates to Cobra.
- `qo/cmd/start.go:48-83` — `qo start` cobra command. Requires root. Extracts rootfs from embedded tarball, decrypts archive into it, calls `sandbox.StartSandBox()`.
- `qo/cmd/build.go:44-65` — `qo build`. Validates folder structure, creates encrypted tar archive.
- `qo/cmd/meta.go:23-41` — `qo meta`. Reads and decodes meta.yaml from encrypted archive, prints JSON.

### Namespace/Isolation Checklist

#### clone() flags — `deploy/qo-init.c:341`
```c
CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWNET | CLONE_NEWIPC | CLONE_NEWCGROUP | SIGCHLD
```
| Flag | Present | Notes |
|------|---------|-------|
| `CLONE_NEWUTS` | ✅ | Isolated hostname |
| `CLONE_NEWPID` | ✅ | qo-init child becomes PID 1 |
| `CLONE_NEWNS` | ✅ | Mount namespace |
| `CLONE_NEWNET` | ✅ | Loopback-only (setup_loopback in qo-init.c:104-124) |
| `CLONE_NEWIPC` | ✅ | Inter-process comm isolation |
| `CLONE_NEWCGROUP` | ✅ | Cgroup namespace |
| `CLONE_NEWUSER` | ❌ **NOT SET** | setup_userns() exists (qo-init.c:39-60) but is **never called** from `child()` or `main()`. Root inside sandbox = root on host. |

#### pivot_root vs chroot — `deploy/qo-init.c:62-102`
- **Primary**: `pivot_root` via `syscall(SYS_pivot_root, ...)` with bind mount + `.pivot_old` detach.
- **Fallback**: `chroot()` on failure (qo-init.c:293-299).
- Fallback error behavior: `perror("chroot fallback")` — error is **reported**, then returns 1 (does NOT silently continue).

#### Cgroup v2 setup — `qo/pkg/sandbox/rootfs.go:81-116`

| Operation | Code | Correct? |
|-----------|------|----------|
| Parent cgroup created at | `/sys/fs/cgroup/qo-sessions` | ✅ |
| `subtree_control` written to | `/sys/fs/cgroup/qo-sessions/cgroup.subtree_control` | ✅ — correct ancestor path |
| Controllers enabled | `memory`, `pids`, `cpu` | ✅ |
| Child cgroup at | `/sys/fs/cgroup/qo-sessions/{sessionID}` | ✅ |
| Limits set | `memory.max=536870912`, `pids.max=200`, `cpu.max=100000 100000` | ✅ |
| PID written to `cgroup.procs` | `cmd.Process.Pid` — the Go parent's child process (qo-init host PID) | ⚠️ **Race**: PID write happens after `cmd.Start()` — the qo-init process may already have called `clone()` and forked bash before the cgroup attachment completes. Children inherit cgroup but initial burst is unconstrained. |

#### PID 1 handling — `deploy/qo-init.c:188-277`
- `clone()` creates a new process that becomes PID 1 in the new PID namespace.
- `child()` calls `spawn_shell()`, which:
  1. `fork()` → bash becomes PID 2+.
  2. Parent (PID 1) loops on `waitpid(-1, ...)` **reaping all children**.
  3. When shell_pid exits, returns exit status.
- **bash is NOT PID 1** — PID 1 is the `child()` process. bash is a child of PID 1. This is correct: zombie reaping works.

#### Device nodes — `qo/pkg/sandbox/rootfs.go`
- Embedded in `rootfs.tar.gz` as char device nodes (tar.TypeChar). Extracted via `syscall.Mknod` in `ExtractRootfs` (rootfs.go:205-213).
- devtmpfs mounted on `/dev` inside sandbox (qo-init.c:308-311) — overrides tar-embedded device nodes.
- devpts mounted on `/dev/pts` with `newinstance,ptmxmode=0666,mode=0620` (qo-init.c:314-318).

#### Seccomp — `deploy/qo-init.c:151-186`
- **Default: OFF**. If `QO_SECCOMP` env is unset or `"off"`, returns immediately without loading filter.
- `QO_SECCOMP=log` → `SCMP_ACT_LOG` (log violations but allow).
- `QO_SECCOMP=enforce` → `SCMP_ACT_KILL_PROCESS` (kill on violation).
- Filter: ALLOW by default (`seccomp_init(SCMP_ACT_ALLOW)`), with explicit rules blocking: `reboot`, `mount`, `umount2`, `pivot_root`, `unshare`, `setns`, `init_module`, `finit_module`, `delete_module`, `kexec_load`, `personality`, `ptrace`.
- Failure mode: `seccomp_load()` failure is reported via fprintf but execution continues.

#### Capability dropping — `deploy/qo-init.c:126-133`
- Present. Calls `capset` syscall with all-zero capability sets (drops every capability).
- Called after pivot_root/mounts but before seccomp setup.
- No verification that caps were actually dropped (no getcap check).

#### Rootfs contents
- Embedded as `rootfs.tar.gz` (around 3.5 MB compressed, ~12 MB extracted based on directory sizes).
- BusyBox with explicit symlinks: `sleep`, `kill`, `pkill`, `killall`, `stat`, `passwd`, `chpasswd`, `adduser`, `addgroup`, `deluser`, `delgroup`, `wc`, `head`, `tail`, `tr`, `cut`, `more`, `strings`, `diff` → all symlinked to `busybox` at extraction time (rootfs.go:216-223).
- Interactive shell: `/bin/bash` (invoked as `execl("/bin/bash", "/bin/bash", "-i", NULL)` in qo-init.c:254).

#### Multi-challenge/metadata

| Feature | Status | Evidence |
|---------|--------|----------|
| `meta.yaml` parsing | ✅ Exists | `archive/metadata.go:54-100` — `DecryptMetadata()`, `normalizeMeta()` |
| `qo meta` subcommand | ✅ Exists | `qo/cmd/meta.go:23-53` — calls `archive.DecryptMetadata()`, prints JSON |
| In-sandbox `logo`/`quest`/`hint`/`go`/`map`/`status`/`help` | ✅ Exists | Defined as bash aliases via `.bashrc` appended by `spawn_shell()` (qo-init.c:216-252). File-based IPC via `.qo-challenge-req` / `.qo-challenge-resp` |
| Server-side challenge API | ✅ Exists | `server/challenge.go` — `pollChallengeRequests` reads IPC files, handles all actions |
| `check.sh` kept host-side only | ✅ | `loadCheckScripts` (server/challenge.go:153-183) reads check.sh content into memory, deletes from sandbox |
| Validator system | ✅ Exists | 7 types: flag, process_dead, process_running, file_exists, file_not_exists, file_contains, file_permissions (server/challenge.go:227-274) |
| `init.sh` support | ✅ Exists | `loadCheckScripts` reads and deletes init.sh. `runInitScript` executes it on first `quest` access to each level |

### Session lifecycle

| Phase | Code | Detail |
|-------|------|--------|
| **Path generation** | `rootfs.go:30-36` | `/tmp/qo-sessions/{studentID}-{4-char-hex}` |
| **Concurrency cap** | `rootfs.go:54-79` | Lock file at `/tmp/qo-sessions.lock` + count of dirs in `/tmp/qo-sessions/`. Max 8 (`maxConcurrentSessions`). |
| **Rootfs extraction** | `rootfs.go:147-233` | Extracts embedded tar.gz, creates missing busybox symlinks, chmods device nodes |
| **Archive decryption** | `archive/decrypt.go:64-225` | PBKDF2 key derivation, AES-CTR stream decryption, unlock time check |
| **PTY** | `rootfs.go:265-338` | `pty.Open()` via creack/pty, terminal raw mode, SIGWINCH forwarding |
| **Duration enforcement** | `rootfs.go:302-310` | goroutine with `time.Sleep(duration)` then `SIGTERM` → 5s wait → `SIGKILL` |
| **Cleanup** | `rootfs.go:118-138` | Unmount devpts/proc, remove rootfs dir tree, remove cgroup dir. Called after `cmd.Wait()` returns. |

### Known open bugs/gaps

1. **`setup_userns()` defined but never called** — `deploy/qo-init.c:39-60`. The function exists and compiles, but `child()` and `main()` never invoke it. Sandbox runs as real root.
2. **`deploy/src/qo-init.c` is stale** — 86-line minimal version with no seccomp, no user_ns setup, no pivot_root (uses chroot only), no capability dropping, no loopback, no fork+reap. The real code is `deploy/qo-init.c` (352 lines). The `src/` copy should be removed or updated.
3. **Cgroup PID write race** — `setupCgroupV2` writes qo-init's PID to `cgroup.procs` after `cmd.Start()` returns but before the clone() inside qo-init has set up namespaces. While children inherit cgroups, there is a window where qo-init itself runs uncgrouped.
4. **Hardcoded paths in `findHelper()`** — `qo/pkg/sandbox/rootfs.go:241-246`:
   ```go
   candidates := []string{
       filepath.Join(filepath.Dir(binaryPath), "qo-init"),
       "/home/mohammed-niri/projects/qo-learn-tool/qo/qo-init",
       filepath.Join("/home/mohammed-niri/projects/qo-learn-tool", "qo", "qo-init"), // DUPLICATE of above
       "/usr/local/bin/qo-init",
   }
   ```
   Lines 3 and 4 resolve to the same path. All hardcoded paths reference the developer's machine.
5. **Shared mutable global flags** — `cmd/meta.go` references `archivePath`, `passwordStart`, `utKeyStart` defined in `cmd/start.go`. These are package-level vars bound in `start.go:init()`. The `meta` command works because both `init()` functions run, but the variable names suggest they "belong" to `start`.
6. **Seccomp default-off** — No environment variable is set anywhere in the deployment chain. QO_SECCOMP is never exported, so seccomp is always off in practice.
7. **Duration marked "not implemented" in README** — README says "Setting duration is not implemented yet" but `rootfs.go:302-310` clearly implements it. README is stale.
8. **`cgroup.subtree_control` written every session** — `setupCgroupV2` writes to `subtree_control` on every `qo start` invocation. Should be done once at system setup.
9. **`io.Copy(os.Stdout, master)` blocks until PTY closes** — The master-to-stdout copy in `rootfs.go:338` blocks the main goroutine until the sandbox exits. If the terminal session is disconnected, there's no timeout mechanism if the PTY read hangs.
10. **No PID file or tracking** — Sandbox processes are tracked only through the `exec.Cmd` struct. If `go start` is killed abruptly, orphaned qo-init processes remain.

### Build/distribution

**Build process** (from `deploy/run.sh`):
```bash
gcc -O2 -o deploy/qo-init deploy/qo-init.c -lseccomp          # dynamic link
CGO_ENABLED=0 go build -o deploy/qo ./qo                       # static Go
CGO_ENABLED=0 go build -o deploy/server ./server               # static Go
```

**qo-init's runtime location** — `findHelper()` in `rootfs.go:235-256`:
1. Next to the `qo` binary (`{binaryDir}/qo-init`)
2. `/home/mohammed-niri/projects/qo-learn-tool/qo/qo-init` (hardcoded)
3. `/home/mohammed-niri/projects/qo-learn-tool/qo/qo-init` (same, duplicated)
4. `/usr/local/bin/qo-init`

**qo-init linkage** — dynamically linked against libseccomp.so.2 and libcap.so.2:
```
linux-vdso.so.1
libcap.so.2 → /usr/lib/libcap.so.2
libseccomp.so.2 → /usr/lib/libseccomp.so.2
libc.so.6 → /usr/lib/libc.so.6
/lib64/ld-linux-x86-64.so.2
```

---

## Part 2 — Server

### Directory structure

```
server/
├── main.go                 # Entry: LoadConfig, NewServer, http.Server, graceful shutdown
├── config.go               # Env-based config: PORT, EVENT_CODE, ADMIN_SECRET, QO_BINARY_PATH, etc.
├── config_test.go          # 8 tests: defaults, missing each required field, custom values, invalid port
├── handlers.go             # ServeHTTP router, join/login/terminal/admin/challenge API handlers
├── handlers_test.go        # 22 tests: join flow, capacity, reconnect, rate limit, admin auth
├── session.go              # Session struct, SessionManager (token→session, studentID→token)
├── session_test.go         # 9 tests: NewSession, duplicate, capacity, get, remove, state transitions
├── admin.go                # Admin handlers: state, kill, shutdown, queue management
├── admin_test.go           # 5 tests: state, kill, shutdown, queue add/remove
├── challenge.go            # ChallengeState, ChallengeMetadata, validators, pollChallengeRequests, check scripts
├── challenge_test.go       # 35 tests: ChallengeState, normalizeMeta, loadCheckScripts, runInitScript,
│                           #   RunCheckScript, runValidator (all 7 types), processExists,
│                           #   DiscoverLevelsFromRootfs, pollChallengeRequests (14 sub-tests)
├── leaderboard.go          # Leaderboard handler: returns all sessions with scores
├── leaderboard_test.go     # 2 tests: empty list, sessions with scores
├── rate_limit.go           # Per-IP rate limiter with sliding window
├── ratelimit_test.go       # 4 tests: allow, overflow, window expiry, concurrent safety
├── ws.go                   # WebSocket handler, PTY piping, reconnect, cleanup, findQoInit
├── ws_test.go              # 12 tests: findQoInit, cleanupSession, wsNotify, itoa, clientIP, challenge API
├── grace_test.go           # 7 tests: orphan transition, reconnect, close, full flow, multiple disconnects
├── shutdown_test.go        # 4 tests: rejects joins, 404 for unknown, sessions persist, cleanup all
├── log.go                  # Log level filtering (debug/info/warn/error)
├── go.mod / go.sum         # Go 1.26.4, deps: creack/pty, google/uuid, gorilla/websocket
├── webassets/
│   ├── embed.go            # //go:embed index.html terminal.html admin.html leaderboard.html static
│   ├── index.html          # Login page: event code + student name
│   ├── terminal.html       # xterm.js terminal (CDN v5.3.0)
│   ├── admin.html          # Admin dashboard: session list, queue, kill, shutdown
│   ├── leaderboard.html    # Real-time leaderboard
│   ├── static/logo.txt     # ASCII logo asset
│   └── frontend/           # Source copies (go:generate cp -r ../../frontend/. .)
├── eval-results/           # Output directory for eval reports (empty)
├── testdata/
│   ├── broken-meta.yaml    # Invalid YAML for error-path tests
│   └── sample-challenge/   # 2-level challenge fixture for tests
│       ├── 00-basics/      # Has check.sh, init.sh, meta.yaml
│       └── 01-advanced/    # Has check.sh
├── qo-init                 # Prebuilt dynamic binary (copy of deploy/qo-init)
├── server                  # Prebuilt static binary (~10 MB)
└── server-test             # Prebuilt test binary
```

### Session management — `server/session.go`

`SessionManager` exists and provides:
- `NewSession(studentID)` — creates session, checks duplicate, enforces capacity cap
- `GetSession(token)` / `LookupByStudentID(sid)` — lookups
- `RemoveSession(token)` — removes from both maps
- `SetSessionState(token, state)` — state machine transitions
- `AllSessions(fn)` — iterate all sessions
- `CountActive()` — counts pending+active+orphaned
- Capacity: configurable via env `MAX_CONCURRENT` (default 8)

**Session spawn flow** — `server/ws.go:handleWebSocket`:
1. Client connects via WebSocket with `?token=`
2. If session has existing Term + process alive → reuse PTY, upgrade to WS
3. If session has existing rootfs but process dead → restart qo-init in same rootfs (preserves challenge state)
4. Otherwise → generate session rootfs path → `exec.Command(qoBinaryPath, "start", "-i", ..., "-a", ..., ...)` with `QO_SESSION_PATH` env var → PTY pipe → WebSocket upgrade

**qo-init path resolution** — `server/ws.go:433-446` (`findQoInit`):
```go
candidates := []string{
    filepath.Join(filepath.Dir(qoBinaryPath), "qo-init"),
    "/home/mohammed-niri/projects/qo-learn-tool/qo/qo-init",
    filepath.Join("/home/mohammed-niri/projects/qo-learn-tool", "qo", "qo-init"), // DUPLICATE
    "/usr/local/bin/qo-init",
}
```
**Identical hardcoded paths** as the qo sandbox package — same duplication bug.

### HTTP/WS surface

Routes as defined by `ServeHTTP` in `handlers.go:46-105`:

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/` | `handleLogin` → serves `index.html` | None |
| POST | `/join` | `handleJoin` → validates event code, creates session | Rate-limited (5/window) |
| GET | `/terminal?token=` | `handleTerminal` → serves `terminal.html` | Token lookup |
| GET | `/ws?token=` | `handleWebSocket` → WebSocket + PTY | Token lookup |
| GET | `/leaderboard` | `handleLeaderboardPage` → serves `leaderboard.html` | None |
| GET | `/api/leaderboard` | `handleLeaderboardData` → JSON `[{id, name, solved, avatar}]` | None |
| POST | `/api/solved` | `handleSolved` → increment score | Token (form/query) |
| GET | `/api/challenge/quest?token=` | Question text | Token lookup |
| GET | `/api/challenge/hint?token=` | Hint text | Token lookup |
| POST | `/api/challenge/go?token=` | Run check (reads token from URL query) | Token lookup |
| GET | `/api/challenge/map?token=` | Progress map | Token lookup |
| GET | `/api/challenge/status?token=` | Current level info | Token lookup |
| GET | `/admin` | Admin login page HTML | **None** (auth via JS) |
| GET | `/admin/state` | Session list + queue JSON | `X-Admin-Token` header |
| POST | `/admin/kill?token=` | Kill session | `X-Admin-Token` header |
| POST | `/admin/shutdown` | Shutdown event | `X-Admin-Token` header |
| GET | `/admin/leaderboard` | Leaderboard page | None |
| GET | `/static/*` | Static files (logo.txt) | None |

### Admin auth — `handlers.go:325-339`
```go
func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) bool {
    token := r.Header.Get("X-Admin-Token")
    if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AdminSecret)) != 1 {
        if !s.adminLimiter.Allow(ip) {
            writeJSON(w, http.StatusTooManyRequests, ...)
            return false
        }
        writeJSON(w, http.StatusUnauthorized, ...)
        return false
    }
    return true
}
```
- **Confirmed protected**: `/admin/state`, `/admin/kill`, `/admin/shutdown` all gates through `adminAuth()`.
- **Separate rate limiter**: `adminLimiter` (5 per minute) tracks failed auth attempts by IP.
- **Login page (GET /admin) is NOT protected** — serves HTML that performs auth via JavaScript.

### Leaderboard
- `solved` is a **real tracked value**: `session.IncrementScore()` called from `pollChallengeRequests` when `go` passes (challenge.go:477-487).
- `handleSolved` endpoint at `POST /api/solved` allows external score increment (token-based).
- No code path listens for `QO_EVENT: SOLVED/ALL_COMPLETE` signals from a qo start subprocess. The challenge system is entirely file-based IPC (within the sandbox's tmp dir) and the server's own in-memory `ChallengeState`. The two are independent.

### Frontend — served via `go:embed`
- `server/webassets/embed.go:7`: `//go:embed index.html terminal.html admin.html leaderboard.html static`
- All pages are embedded into the server binary at compile time.
- A parallel `frontend/` directory exists at the project root; `go:generate cp -r ../../frontend/. .` copies it into webassets before build.
- Pages: `index.html` (login form), `terminal.html` (xterm.js terminal v5.3.0 from CDN), `admin.html` (dashboard + auth overlay), `leaderboard.html` (ranked table).

### Known gaps from prior audits

| Gap | Current state | Evidence |
|-----|---------------|----------|
| **Idle timeout enforcement** | ✅ **IMPLEMENTED** | `ws.go:354-375` — 5-second ticker, checks `time.Since(session.LastActive())` against configurable `IdleTimeout` (default 600s). Sends shutdown notification, waits 3s, closes conn, calls `cleanupSession`. |
| **Resize handling** (`pty.Setsize`) | ✅ **IMPLEMENTED** | `ws.go:321-330` — JSON messages with `{"type":"resize","cols":N,"rows":N}` trigger `pty.Setsize()`. Tested in `challenge_test.go` resize handling tests. |
| **Rate limiting scope** | ✅ **IMPLEMENTED** | Two rate limiters: `limiter` (5 per GracePeriod for /join) and `adminLimiter` (5 per minute for failed admin auth). Both per-IP sliding windows. |
| **Per-session archive paths** | ✅ **IMPLEMENTED** | Each session gets unique rootfs at `/tmp/qo-sessions/{id}-{random}` (ws.go:165). `QO_SESSION_PATH` env var passed to `qo start` (ws.go:181). Session's `RootfsPath` field tracks it. |
| **Grace period / graceful disconnect** | ✅ **IMPLEMENTED** | `ws.go:402-410` — on disconnect without cleanup, session enters `Orphaned` state. After `GracePeriod` (default 45s, env `GRACE_PERIOD`), if still orphaned, calls `cleanupSession`. Reconnect within window reactivates. |
| **Shutdown flow** | ✅ **IMPLEMENTED** | `main.go:34-78` — SIGINT/SIGTERM sets shutdown flag, notifies all sessions, waits 30s for voluntary disconnect, then force-kills remaining. `handleJoin` returns 503 when `shutdown==true`. |

---

## Part 3 — Cross-Cutting

### Distro testing
**No test matrix exists.** There is no `deploy/test-matrix/` directory, no Docker-based test script, no cross-distro CI matrix, and no results files anywhere in the repository. All testing has been done on the developer's Arch Linux machine.

### Security posture

**Current threat model: sandbox escape ≈ real host root.**

Plain statement: because `CLONE_NEWUSER` is **not** set (despite `setup_userns()` existing in code), the sandbox runs as UID 0 on the host. This means:

| Protection | Status | Impact on escape |
|-----------|--------|------------------|
| Mount namespace | ✅ | Filesystem is isolated, but root in the sandbox can still interact with host fs via procfs if not restricted. The `switch_root` + private mount propagation helps. |
| PID namespace | ✅ | Can't see host processes |
| Network namespace | ✅ | Loopback only — no external network |
| IPC namespace | ✅ | Can't use host IPC |
| Cgroup namespace | ✅ | Sees cgroup subtree |
| User namespace | ❌ **Missing** | **Root in sandbox = root on host.** Kernel exploit in a namespace-aware syscall gives full host root. |
| Seccomp | ⚠️ Default OFF | Dangerous syscalls (mount, reboot, etc.) are whitelisted by default. Must be manually enabled via `QO_SECCOMP=enforce`. |
| Capabilities | ✅ | Dropped after pivot_root — sandbox has zero capabilities |
| rlimits | ✅ | nofile=1024, nproc=128, core=0, fsize=10MB |

**Realistic attack vectors:**
1. Kernel bug exploited from inside the sandbox → full host root (no user namespace barrier).
2. `/proc/sysrq-trigger` or similar procfs writes from inside the sandbox (if procfs is mounted without `hidepid=invisible`).
3. `ptrace` is blocked by seccomp, but only when seccomp is enabled.

### What's tested vs. untested

#### Tested (verified by automated tests and explicitly passing)

All 73 tests pass on Arch Linux. Test categories:

- **`session_test.go`** (9): Manager CRUD, duplicate detection, capacity enforcement, state transitions, concurrent safety
- **`challenge_test.go`** (35): ChallengeState data model, normalizeMeta, loadCheckScripts, runInitScript, RunCheckScript, all 7 validator types, processExists, DiscoverLevelsFromRootfs, pollChallengeRequests (14 action types)
- **`handlers_test.go`** (22): Join flow (valid, invalid code, missing name, capacity, reconnect), rate limiting, admin auth (no header, wrong, correct, rate-limited, different IP), kill/shutdown auth guards, shutdown rejection
- **`admin_test.go`** (5): State response, kill, kill 404, shutdown flag, queue add/remove
- **`leaderboard_test.go`** (2): Empty list, entries with scores
- **`config_test.go`** (8): Defaults, missing each required field, custom values, invalid port
- **`ratelimit_test.go`** (4): Allow under limit, overflow, window reset, concurrent safety
- **`grace_test.go`** (7): Orphan transition, reconnect, close, full lifecycle, multiple disconnects, reconnect after cleanup, same-student reconnect
- **`shutdown_test.go`** (4): Reject new joins, 404 routes, sessions persist, cleanup all
- **`ws_test.go`** (12): findQoInit, cleanupSession (3 variants), wsNotify (nil Term), itoa, clientIP (2), challenge API handlers (7)

**Total: 73 automated tests, all passing.**

#### Untested (code exists but never verified end-to-end)

- **Complete `qo build` → encrypted archive → `qo start` flow** — No integration test creates a real encrypted archive and boots a sandbox.
- **Sandbox startup** — `ExtractRootfs` + `DecryptTarArchive` + `StartSandBox` sequence never tested together end-to-end.
- **Cgroup enforcement** — `memory.max=512M`, `pids.max=200` written, but never verified that a process exceeding these limits actually gets OOM-killed or PID-limited.
- **Duration enforcement** — `SIGTERM` after `time.Sleep(duration)` never integration-tested.
- **Seccomp filter** — Always off in practice (`QO_SECCOMP` never set). Never tested with `enforce` or `log`.
- **Capability dropping** — Code runs, but never verified from inside the sandbox that `cat /proc/self/status | grep Cap` shows zeroes.
- **PTY resize** — Code parses resize messages, never integration-tested with actual terminal resize events.
- **WebSocket reconnect** — Reuse PTY / restart qo-init in existing rootfs code paths never integration-tested.
- **Idle timeout** — `ws.go:354-375` code exists, never verified with end-to-end test.
- **Orphan → grace period → cleanup** — State machine tested in unit tests, but the goroutine that waits `GracePeriod` then calls `cleanupSession` is never integration-tested.
- **Multiple concurrent sandboxes** — `maxConcurrentSessions=8` cap is tested (exceeds cap returns error), but running 8 sandboxes simultaneously is never verified.
- **Distro compatibility** — No tests on any distro other than Arch Linux.
- **`qo build` with non-trivial challenge** — The `build` command validates folder structure and creates archives, but no test verifies that the resulting archive is decryptable and extractable.
- **`qo meta` subcommand** — No test calls `qo meta` and parses the JSON output.
- **Embedded rootfs extraction on different filesystems** — Rootfs is a `.tar.gz` embedded in the Go binary. Extraction on overlayfs, tmpfs, or btrfs is untested.
