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

### 2.1 CLONE_NEWPID — PRESENT

Current `Cloneflags` at `rootfs.go:316`:
```go
syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET
```

`CLONE_NEWPID` is set. This means:
- The sandboxed process runs as PID 1 in a new PID namespace.
- `ps` inside the sandbox shows only namespace-local processes.
- `pgrep`, `kill`, `pkill` work correctly within the namespace (if /proc is mounted).

### 2.2 /proc IS Mounted Inside the Sandbox

`rootfs.go:269`: `syscall.Mount("proc", "/proc", "proc", 0, "")` — called inside the child after `Chroot()`.

So:
- **`ps` shows namespace-local processes** — reads from `/proc`.
- `pgrep`, `kill`, `pkill` work correctly if `/proc` is mounted.
- Note: `kill` and `pkill` are now symlinked to `busybox` at runtime by `ExtractRootfs()` if missing.

### 2.3 leaky_daemon / exec -a Scenario

- `exec -a` is a **bash feature** (not available in BusyBox ash). Since the interactive shell is bash, `exec -a leaky_daemon sleep 3600 &` works syntactically inside bash.
- `sleep` is now symlinked to `busybox` at runtime if missing.
- With `CLONE_NEWPID`, the backgrounded process is isolated in the namespace. `pkill` (now symlinked) can find it via `/proc`.

### 2.4 pkill/kill from Inside

- `pkill` and `kill` are now symlinked to `busybox` at runtime by `ExtractRootfs()` if missing.
- BusyBox `kill` and `pkill` use `/proc` to find processes. With `/proc` mounted and `CLONE_NEWPID`, they enumerate and signal namespace-local processes correctly.

---

## 3. User/Permission Mechanics

### 3.1 User Namespace — NOT PRESENT

There is no `CLONE_NEWUSER` in `Cloneflags`. The sandbox runs as **real root** (UID 0) in the new UTS, PID, mount, and network namespaces. No UID/GID mapping is performed.

### 3.2 Per-Session User Creation — NOT IMPLEMENTED

`createSandboxUser` does not exist. The sandboxed bash runs as root. The `/etc/passwd` in rootfs contains `ahmed` but it is unused. `whoami` reports `root`.

### 3.3 /dev/null — FIXED

`/dev/null` is created from the embedded rootfs tarball via `syscall.Mknod` in `ExtractRootfs()`. The `&>/dev/null` redirection issue was caused by the missing controlling terminal (no `setsid` + `TIOCSCTTY`). This is now fixed.

### 3.4 chown/chmod/chroot

- `chown` and `chmod` are BusyBox symlinks and work.
- `useradd` (standalone shadow binary) will attempt to lock /etc/passwd, /etc/shadow, /etc/group. Inside the sandbox, these files are owned by root and writable.
- `groupadd` does NOT exist — neither as a binary nor a BusyBox applet symlink.

---

## 4. Shell Environment

### 4.1 Interactive Shell: bash (Standalone)

Confirmed: `bash` at `/bin/bash` (~1.2 MB dynamic ELF). Command: `exec.Command("/bin/bash", "-i")` at `rootfs.go:277`.

### 4.2 Start-up Files

| File | Present? | Sourced by `bash -i`? |
|------|----------|-----------------------|
| `/etc/profile` | **NO** — does not exist | Would be sourced first if present |
| `~/.bashrc` | Only for `/home/ahmed/` (pre-existing user) | Not sourced for non-login interactive shells |
| `/etc/bashrc` | NO | N/A |
| `~/.profile` | YES — for `/home/ahmed/` | Sourced for login shells |

No `cmd.Env` is set in `StartSandBox` — bash inherits the parent's environment. No PATH overrides are applied.

### 4.3 BusyBox vs Bash Syntax Risks

Since `sh -> busybox` (ash):

| Feature | Works in bash | Works in ash | Risk |
|---------|--------------|--------------|------|
| `[[ ]]` | Yes | No | check.sh may fail silently |
| Arrays `arr=(x y)` | Yes | No | Syntax error in ash |
| `exec -a name` | Yes | No | Fails silently |
| `&>/dev/null` | Yes | Yes | Works |
| `$(cmd)` | Yes | Yes | Works |
| `source file` | Yes | No (use `. file`) | Fails if `#!/bin/sh` |

The three example check.sh files all use `#!/bin/bash`, so they will invoke bash directly (not through the sh symlink). This is correct.

---

## 5. What Broke — Evidence from Logs & State

### 5.1 /dev/null: Permission denied — FIXED

This was caused by bash running without a controlling terminal (no `setsid` + `TIOCSCTTY`). With the fix in `rootfs.go:287` (`unix.IoctlSetInt(TIOCSCTTY)`) and `Setsid: true` in `SysProcAttr`, bash now has a proper controlling terminal and `/dev/null` redirection works.

### 5.2 "bash: cannot set terminal process group: Inappropriate ioctl for device" — FIXED

This was caused by bash running without a controlling terminal. Fixed by adding `Setsid: true` and `TIOCSCTTY` ioctl in `rootfs.go`.

### 5.3 "bash: sleep: command not found" — FIXED

`sleep` was not symlinked in the rootfs. `ExtractRootfs()` now creates symlinks for missing BusyBox applets (`sleep`, `kill`, `pkill`, `killall`, `stat`, `passwd`, `chpasswd`, `adduser`, `addgroup`, `deluser`, `delgroup`) at runtime.

### 5.4 "mkdir: can't create directory 'file': Permission denied"

If this occurs, it suggests the working directory is not writable. The sandbox `chdir`s to `/tmp` which should be writable by root.

### 5.5 "Vim: Warning: Output is not to a terminal" / "E1187: Failed to source defaults.vim"

vim works but prints warnings in a PTY environment. Cosmetic.

### 5.6 Stale Cgroups

If stale cgroup directories exist, it means the parent process was killed before `cleanupSession()` could run. The cleanup code is now called via explicit call after `cmd.Wait()` in the parent.

### 5.7 Stale Session Directories

Same as stale cgroups — parent was killed before cleanup. The `defer releaseConcurrencyCap()` ensures the lock is released, and `cleanupSession()` removes the directory on normal exit.

---

## 6. Current meta.yaml Handling — Exact State

### 6.1 Build-time (qo build)

`meta.yaml` is **optional** during `qo build`. `IsValidFolderStructure()` in `archive/utils.go` does not require it. If present, it is included in the encrypted archive as a regular file.

### 6.2 Encrypted Archive

`meta.yaml` IS included in the encrypted archive if present in the challenge folder. It is tar'd and encrypted like any other file.

### 6.3 Post-build: Reading from Encrypted Archive (qo meta)

`DecryptMetadata` in `pkg/archive/metadata.go` reads `meta.yaml` from inside the encrypted archive without decrypting the full payload. The `qo meta` CLI command (`cmd/meta.go`) wraps this and outputs JSON to stdout.

### 6.4 Server Loading

The server (`handlers.go:284-313`) runs `qo meta` as a subprocess at startup to load title/difficulty.

### 6.5 Inside the Sandbox — NOT Read at All

There is no code path that reads `meta.yaml` from inside a running sandbox session. The challenge files (including `meta.yaml`) are decrypted into `rootfs/tmp/<level-dir>/meta.yaml`, but no code inside the sandbox ever reads it.

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
| `ps` | YES | BusyBox | Works — /proc is mounted |
| `pgrep` | YES | BusyBox | Works in namespace |
| `pkill` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `kill` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `killall` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `sleep` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `stat` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `useradd` | Standalone | YES | From shadow — works if /etc/shadow is writable |
| `usermod` | Standalone | YES | From shadow |
| `useradd` (shadow) | YES | Standalone | 108K — requires /etc/{passwd,shadow,group} writable |
| `groupadd` | **MISSING** | N/A | Neither standalone nor busybox applet |
| `adduser` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `addgroup` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `passwd` | BusyBox | SYMLINKED AT RUNTIME | Symlinked to busybox by `ExtractRootfs()` if missing |
| `su` | YES | BusyBox | **Errors: `su: must be suid to work properly`** — no setuid in chroot |
| `mount` | YES | BusyBox | Works if /etc/fstab exists and permissions allow |
| `clear` | YES | BusyBox | Works |
| `nano` | Standalone | YES | Works in PTY |
| `vim` | Standalone | YES | Works in PTY with warnings |
| `less` | Standalone | YES | Works |
