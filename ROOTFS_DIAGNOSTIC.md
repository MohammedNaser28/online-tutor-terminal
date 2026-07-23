# ROOTFS_DIAGNOSTIC — qo Sandbox Runtime & Rootfs Inventory

**Date:** 2026-07-23
**Rootfs:** `qo/pkg/sandbox/rootfs.tar.gz` (9.5 MB embedded, BusyBox v1.36.1)
**Sandbox code:** `qo/pkg/sandbox/rootfs.go` (466 lines)
**Submodule HEAD:** `d0772ea` (output-dir logging)

---

## 1. Rootfs Contents Inventory

### 1.1 Real Binaries vs BusyBox Applets

| Type | Count | Files |
|------|-------|-------|
| BusyBox symlinks | 32 | awk, cat, chgrp, chmod, chown, clear, cp, cut, echo, egrep, fgrep, find, grep, id, ls, mkdir, mount, mv, pgrep, ps, pwd, rev, rm, sed, sh, sort, su, touch, uname, uniq, whoami |
| Standalone binaries | 8 | bash, busybox (the static multi-call), less, man, mandoc, nano, useradd, usermod, vim |
| Shared libraries | 16 | libc.so.6, ld-linux-x86-64.so.2, libreadline.so.8, libncursesw.so.6, libpcre2-8.so.0, libz.so.1, libzstd.so.1, libmagic.so.1, libacl.so.1, libaudit.so.1, libcap-ng.so.0, libgpm.so.2, libbz2.so.1.0, liblzma.so.5, libm.so.6 (+ duplicates in lib/ and lib64/) |
| Device nodes | 6 | null, zero, random, urandom, tty, ptmx (in tar; bind-mounted at runtime) |
| Terminfo | ~2800 | `usr/share/terminfo/` (full ncurses db) |
| Man pages | none | `man`/`mandoc` binaries present but no man page content — they error at runtime |
| Config | 3 | `etc/passwd`, `etc/group`, `etc/hostname` |

### 1.2 BusyBox Applets WITHOUT a Symlink in /bin — WILL FAIL AT RUNTIME

These are all listed by `busybox --list` but have **no symlink in `/bin`**. Calling them by name inside the sandbox produces `command not found`:

```
sleep       kill        pkill       killall     stat
passwd      chpasswd    adduser     addgroup    deluser
delgroup
```

These are `useradd`'s BusyBox equivalent (`adduser`) but `useradd` is a standalone shadow-utils binary instead. Neither `groupadd` nor `addgroup` exist as standalone binaries.

### 1.3 Standalone Binaries — Details

| Binary | Size | Type | Notes |
|--------|------|------|-------|
| `bin/bash` | 1.2 MB | Dynamic ELF | Full GNU bash, linked to libreadline + libncursesw |
| `bin/busybox` | 1.3 MB | Static ELF | Multicall binary — all applets |
| `bin/useradd` | 108 KB | Dynamic ELF | From shadow-utils — modifies /etc/passwd, /etc/shadow |
| `bin/usermod` | 100 KB | Dynamic ELF | From shadow-utils |
| `bin/vim` | ~2 MB | Dynamic ELF | Full vim — works but warns "Output is not to a terminal" through WebSocket PTY |
| `bin/nano` | ~300 KB | Dynamic ELF | Works in PTY |
| `bin/less` | ~150 KB | Dynamic ELF | Works |
| `bin/man` | ~200 KB | Dynamic ELF | No mandb — `man ls` fails with "No manual entry" |
| `bin/mandoc` | ~200 KB | Dynamic ELF | Same — no man pages installed |

### 1.4 `sh` — Actually BusyBox ash, NOT bash

```
/bin/sh -> busybox              # BusyBox ash
/bin/bash                        # Real GNU bash (standalone)
```

The interactive shell is `bash` (from `StartSandBox` at line 399), but `sh` resolves to BusyBox ash. Any check.sh or setup.sh using `#!/bin/sh` will run under ash, which lacks:
- `[[ ]]` test syntax
- Arrays (`arr=(...)`)
- `exec -a` argv0 renaming
- `source` (uses `.` instead)

All three example check.sh files use `#!/bin/bash`, so this is fine for those. But any script using `#!/bin/sh` will get ash.

---

## 2. Process Visibility / PID Namespace

### 2.1 No CLONE_NEWPID

Current `Cloneflags` at `rootfs.go:427`:
```go
syscall.CLONE_NEWNS | syscall.CLONE_NEWNET | syscall.CLONE_NEWUSER
```

`CLONE_NEWPID` is **absent**. This means:
- The sandboxed process shares the **host PID namespace**.
- `ps` inside the sandbox (when /proc is available) shows host processes.
- PIDs inside are the same as outside.
- There is no PID 1 inside the namespace — bash runs as some high-numbered host PID.

### 2.2 /proc is NEVER Mounted Inside the Sandbox

Despite `cleanupSession` (line 213) attempting to unmount `/proc/` and `ExtractRootfs` (line 240) force-unmounting it — **the code never calls `syscall.Mount("proc", ...)`**. There is no proc mount anywhere in `StartSandBox`.

The rootfs has `/proc/` as an empty directory. With `CLONE_NEWNS`, the host `/proc` is not inherited. So:

- **`ps` will show nothing useful** — it reads from an empty `/proc` directory.
- `pgrep`, `kill`, `pkill` will mostly not work correctly since they rely on `/proc`.
- The stale session logs show session PIDs *were* visible (from the host side), but inside the sandbox `/proc` was unimplemented.

### 2.3 leaky_daemon / exec -a Scenario

- `exec -a` is a **bash feature** (not available in BusyBox ash). Since the interactive shell is bash, `exec -a leaky_daemon sleep 3600 &` would work syntactically inside bash.
- However, `sleep` is a BusyBox applet with **no symlink in /bin** — `sleep: command not found` would occur first.
- Even if `sleep` were symlinked, without `CLONE_NEWPID`, the backgrounded process would be visible as any other host process. `pkill` (also missing from /bin) wouldn't work.

### 2.4 pkill/kill from Inside

- `pkill` is in busybox applet list but has **no symlink** — will fail with `command not found`.
- `kill` is also in busybox but has **no symlink**.
- Even if symlinked, BusyBox `kill` and `pkill` use `/proc` to find processes. With an empty `/proc`, they cannot enumerate or signal processes correctly.

---

## 3. User/Permission Mechanics

### 3.1 User Namespace — PRESENT and MAPPING

The user namespace fix (`CLONE_NEWUSER` + `setupUserNamespaceMapping`) **IS present** in the current submodule state (commit `6ccc4af`). It was not rolled back.

Current mapping at `rootfs.go:121-147`:
```
setgroups -> "deny"
uid_map:   "0 <hostUID> 1"
gid_map:   "0 <hostGID> 1"
```

Inside the sandbox: UID 0 (root).
Outside: `hostUID` (mohammed-niri, usually 1000).

The code runs as root (`os.Geteuid() != 0` check in `start.go:29`), so host UID is 0 at runtime when the server launches qo. If the server runs as a non-root user, the namespace mapping writes would fail with permission errors.

### 3.2 Per-Session User Creation

`createSandboxUser` adds `s<studentID>` with UID 2000+ to the chroot's passwd/group/shadow. This works because:
- The parent process is root (can write to chroot's /etc files).
- The sandbox starts as UID 2000+ (from `bash --login` starting as the sandbox user, then su -, or... wait, actually looking at the code more carefully...)

Actually wait — looking at `StartSandBox` again:
```go
username := "s" + studentID
createSandboxUser(chrootPath, username, studentID, uid)
cmd := exec.Command("/bin/bash", "--login", "-i")
cmd.Dir = "/home/" + username
```

The bash process runs as root (UID 0) inside the user namespace because the parent process never calls `DropToUser()` or similar. The user is added to passwd/group/shadow but **bash starts as root** — the sandbox user entry in passwd is just for reference, not activated by `su` or `login`.

So `whoami` inside the sandbox reports `root`, not the student username. `id` shows UID 0.

### 3.3 /dev/null Bug — CONFIRMED BROKEN

From actual session logs:
```
/level1/check.sh: line 2: can't create /dev/null: Permission denied
```

The bind-mount of `/dev/null` in `createDevices` (line 366) is attempted but fails inside the user namespace. This is because the namespace UID 0 (root) is mapped to an unprivileged host UID — and the bind mount was already done by the parent process, but the device node permissions inside the namespace don't allow writing.

The actual issue: `createDevices` runs in the **parent process** (before `cmd.Start()`), so the bind mounts happen with real-root privileges. But inside the user namespace, /dev/null may still have permissions that don't allow the sandbox user to access it. The `check.sh` scripts use `&>/dev/null` which needs write access to /dev/null.

### 3.4 chown/chmod/chroot

- `chown` and `chmod` are BusyBox symlinks and exit. `chown` inside a user namespace will succeed for UID 0 (since UID 0 inside owns all namespace resources) but will fail if trying to change to an unmapped UID.
- `useradd` (standalone shadow binary) will attempt to lock /etc/passwd, /etc/shadow, /etc/group. Inside the sandbox, these files are owned by the mapped host UID and writable. However, `useradd` also tries to:
  - Create home directories (may fail if parent doesn't exist)
  - Write to /etc/shadow (works if file exists and is writable)
  - Preserve SELinux contexts (not applicable)
- `groupadd` does NOT exist — neither as a binary nor a BusyBox applet symlink.

---

## 4. Shell Environment

### 4.1 Interactive Shell: bash (Standalone)

Confirmed: `bash` at `/bin/bash` (~1.2 MB dynamic ELF). Command: `exec.Command("/bin/bash", "--login", "-i")`.

### 4.2 Start-up Files

| File | Present? | Sourced by `bash --login -i`? |
|------|----------|-------------------------------|
| `/etc/profile` | **NO** — does not exist | Would be sourced first if present |
| `~/.profile` | YES — created by `createSandboxUser` | Sourced for login shells |
| `~/.bashrc` | Only for `/home/ahmed/` (pre-existing user) | Not sourced for login-only shells |
| `/etc/bashrc` | NO | N/A |

The `createSandboxUser` function writes only `PS1=...` to `~/.profile`. No PATH, no aliases, no other environment setup. The environment is set via `cmd.Env` in `StartSandBox`:
```
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
HOME=/home/s<studentID>
USER=s<studentID>
LOGNAME=s<studentID>
TERM=xterm-256color
```

### 4.3 BusyBox vs Bash Syntax Risks

Since `sh -> busybox` (ash):

| Feature | Works in bash | Works in ash | Risk |
|---------|--------------|--------------|------|
| `[[ ]]` | Yes | No | check.sh may fail silently |
| Arrays `arr=(x y)` | Yes | No | Syntax error in ash |
| `exec -a name` | Yes | No | Fails silently |
| `&>/dev/null` | Yes | Yes | Works (but /dev/null is broken anyway) |
| `$(cmd)` | Yes | Yes | Works |
| `source file` | Yes | No (use `. file`) | Fails if `#!/bin/sh` |

The three example check.sh files all use `#!/bin/bash`, so they will invoke bash directly (not through the sh symlink). This is correct.

---

## 5. What Broke — Evidence from Logs & State

### 5.1 /dev/null: Permission denied (CONFIRMED, REPRODUCIBLE)

```
/level1/check.sh: line 2: can't create /dev/null: Permission denied
```

This appears in **3 of 8** session logs. The bind-mount of host `/dev/null` into the chroot succeeds from the parent process, but inside the user namespace the device node has wrong permissions or is unmounted. This causes any `&>/dev/null` redirect to fail.

**Root cause:** Likely the user namespace mapping causes device node access checks to fail. The chown bind-mount is done by the parent (real root) but /dev/null inside the namespace may not be accessible to the sandboxed user.

### 5.2 "bash: cannot set terminal process group: Inappropriate ioctl for device"

This appears in **7 of 8** logs. Cosmetic — happens when bash is started without a controlling terminal (which is the case when running through a PTY via WebSocket). The session still works.

### 5.3 "bash: type:shutdown: command not found"

The server sends JSON shutdown messages like `{"type":"shutdown",...}` directly to the PTY, which bash interprets as a command. This is a **server-side protocol issue in `ws.go`** — shutdown messages are written to stdin instead of being intercepted at the WebSocket level.

### 5.4 "mkdir: can't create directory 'file': Permission denied"

A user tried `mkdir file` and got permission denied (appears in one log). This suggests the home directory or working directory may not have been writable for that session.

### 5.5 "Vim: Warning: Output is not to a terminal" / "E1187: Failed to source defaults.vim"

vim works but prints warnings in a PTY environment. Cosmetic.

### 5.6 Stale Cgroups

8 stale cgroup directories exist at `/sys/fs/cgroup/tmp/qo-sessions/-*`. These are **orphaned** — no processes are attached (all `cgroup.procs` are empty), but the directories were never cleaned up. The cleanup code runs but may fail when the unmount of `/proc` fails first.

### 5.7 Stale Session Directories

8 stale session dirs at `/tmp/qo-sessions/-*/` remain. Some still have the decrypted challenge files inside (`rootfs/tmp/`). This means `ExtractRootfs` failed to fully clean up, likely because:
1. The proc unmount in `cleanupSession` fails (since /proc was never mounted), causing the early return to skip the `os.RemoveAll`.

Actually, looking more carefully at `cleanupSession` (lines 211-229): it does NOT return early on failure — it logs a warning and continues. So `os.RemoveAll` should still run. The fact that dirs remain suggests the sessions were killed externally (SIGKILL) before cleanup could run, or the `ExtractRootfs` force-unmount failed.

---

## 6. Current meta.yaml Handling — Exact State

### 6.1 Build-time (qo build)

`meta.yaml` is **parsed during `qo build`** via two paths:

| Path | Function | Location | Behavior |
|------|----------|----------|----------|
| Basic validation | `IsValidFolderStructure` | `archive/utils.go:66-69` | meta.yaml is **optional** — warns on stderr if missing, does NOT fail |
| Manifest validation | `ValidateManifest` | `archive/manifest.go:60-63` | meta.yaml is **required** — fails with error if missing |

The `build` command only calls `IsValidFolderStructure`, not `ValidateManifest`. So `meta.yaml` is effectively **optional at build time** (just gets a stderr warning).

`ChallengeMetadata` struct (`archive/metadata.go:10-14`):
```go
type ChallengeMetadata struct {
    Title      string `json:"title" yaml:"title"`
    Difficulty string `json:"difficulty" yaml:"difficulty"`
    Question   string `json:"question" yaml:"question"`
}
```

### 6.2 Encrypted Archive

`meta.yaml` IS included in the encrypted archive (it's a regular file in the challenge folder that gets added to the tar during `CreateEncryptedTarArchive`). The `encrypt.go` code walks the source directory and includes all files recursively — `meta.yaml` gets tar'd and encrypted like any other file.

### 6.3 Post-build: Reading from Encrypted Archive (qo meta)

`DecryptMetadata` in `archive/decrypt.go:197-247` reads `meta.yaml` **from inside the encrypted archive** without decrypting the full payload:

```go
if strings.HasSuffix(header.Name, "/meta.yaml") || header.Name == "meta.yaml" || header.Name == "./meta.yaml" {
    data, err := io.ReadAll(tr)
    // parse YAML
}
```

The `qo meta` CLI command (`cmd/meta.go`) wraps this and outputs JSON to stdout.

### 6.4 Server Loading

The server (`handlers.go:286-313`) runs `qo meta` as a subprocess at startup:
```go
out, err := exec.Command(s.config.QoBinaryPath, "meta", "-a", archive, "-p", password, "-k", key).Output()
```

The returned title/difficulty is stored on each `Session` and sent to the frontend login/terminal pages.

### 6.5 Inside the Sandbox — NOT Read at All

**There is no code path that reads `meta.yaml` from inside a running sandbox session.** The challenge files (including `meta.yaml`) are decrypted into `rootfs/tmp/<level-dir>/meta.yaml` by `DecryptTarArchive`, but no code inside the sandbox ever reads it. It's available as a file in the filesystem if the student knows to look for it, but:

- qo does not surface the question/title/difficulty inside the terminal.
- The frontend gets the title/difficulty from the join response (loaded at server startup via `qo meta`).
- Inside the sandbox, the student only sees `question.txt` (per the example challenge structure) if they navigate to the level directory.

### 6.6 Summary Table

| Context | meta.yaml used? | Code path | Status |
|---------|----------------|-----------|--------|
| Build validation | Optional | `IsValidFolderStructure` | Working |
| Build encryption | Included in archive | `CreateEncryptedTarArchive` | Working |
| Pre-decrypt reading | Reads from encrypted archive | `DecryptMetadata` | Working |
| CLI query | `qo meta` command | `cmd/meta.go` | Working |
| Server startup | Via `qo meta` subprocess | `handlers.go:loadChallengeMeta` | Working |
| Inside sandbox | Available as file, never read | None | **Gap** — no code consumes it inside |
| Inside sandbox (question text) | From `question.txt`, not meta.yaml | Student navigates manually | **Gap** — no automatic display |

---

## Appendix: Shell Compatibility Matrix

| Command | Present? | Type | Notes for check.sh authors |
|---------|----------|------|---------------------------|
| `ls` | YES | BusyBox | Standard flags work |
| `cat` | YES | BusyBox | Standard |
| `touch` | YES | BusyBox | Standard |
| `mkdir` | YES | BusyBox | Standard |
| `rm` | YES | BusyBox | Standard |
| `cp` | YES | BusyBox | Standard |
| `mv` | YES | BusyBox | Standard |
| `chmod` | YES | BusyBox | Standard octal works |
| `chown` | YES | BusyBox | Works inside user namespace for own UID |
| `chgrp` | YES | BusyBox | Standard |
| `find` | YES | BusyBox | Standard flags (`-type`, `-name`, `-exec`) work |
| `grep` | YES | BusyBox | **NO `--color=auto`** by default; `-E`, `-F` work |
| `egrep` | YES | BusyBox | Works (grep -E) |
| `fgrep` | YES | BusyBox | Works (grep -F) |
| `sed` | YES | BusyBox | **NO `-i` (in-place)** without backup; `-E` works |
| `awk` | YES | BusyBox | **Limited** — no `strftime`, no multi-dim arrays, no `PROCINFO` |
| `cut` | YES | BusyBox | Standard flags work |
| `id` | YES | BusyBox | Standard — reports UID 0 inside namespace |
| `whoami` | YES | BusyBox | Reports "root" |
| `ps` | YES | BusyBox | **Empty output** — /proc is unmounted |
| `pgrep` | YES | BusyBox | **Will fail** — no symlink in /bin |
| `pkill` | BusyBox | NO SYMLINK | `command not found` at runtime |
| `kill` | BusyBox | NO SYMLINK | `command not found` at runtime |
| `killall` | BusyBox | NO SYMLINK | `command not found` at runtime |
| `sleep` | BusyBox | NO SYMLINK | `command not found` at runtime |
| `stat` | BusyBox | NO SYMLINK | `command not found` at runtime |
| `useradd` | Standalone | YES | From shadow — works if /etc/shadow is writable |
| `usermod` | Standalone | YES | From shadow |
| `useradd` (shadow) | YES | Standalone | 108K — requires /etc/{passwd,shadow,group} writable |
| `groupadd` | **MISSING** | N/A | Neither standalone nor busybox applet |
| `adduser` | BusyBox | NO SYMLINK | `command not found` — but exists in busybox applet list |
| `addgroup` | BusyBox | NO SYMLINK | `command not found` |
| `passwd` | BusyBox | NO SYMLINK | `command not found` |
| `su` | YES | BusyBox | **Errors: `su: must be suid to work properly`** — no setuid in chroot |
| `mount` | YES | BusyBox | Works if /etc/fstab exists and permissions allow |
| `clear` | YES | BusyBox | Works |
| `nano` | Standalone | YES | Works in PTY |
| `vim` | Standalone | YES | Works in PTY with warnings |
| `less` | Standalone | YES | Works |
