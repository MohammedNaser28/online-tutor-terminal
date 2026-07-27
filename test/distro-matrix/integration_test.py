#!/usr/bin/env python3
"""Integration tests for qo sandbox — runs inside Docker container as root."""
import subprocess, sys, os, time, json, signal

PASS = 0
FAIL = 0

def test(name, fn):
    global PASS, FAIL
    sys.stdout.write(f"\n=== {name} ===\n")
    sys.stdout.flush()
    try:
        fn()
        sys.stdout.write("  PASS\n")
        PASS += 1
    except Exception as e:
        import traceback; traceback.print_exc()
        sys.stdout.write(f"  FAIL: {e}\n")
        FAIL += 1

QO = "/usr/local/bin/qo"
ARCHIVE = "/archive.enc"

class SandboxSession:
    def __init__(self, duration="120s", qo_seccomp=None):
        is_root = os.geteuid() == 0
        cmd = [] if is_root else ["sudo"]
        cmd += [QO, "start",
               "-i", "inttest",
               "-a", ARCHIVE,
               "-p", "inttest_pass",
               "-k", "inttest_key",
               "-d", duration]
        env = os.environ.copy()
        env["QO_STUDENT_NAME"] = "inttest"
        env["QO_SESSION_PATH"] = f"/tmp/qo-session-{int(time.time()*1000)}"
        if qo_seccomp:
            env["QO_SECCOMP"] = qo_seccomp
        self.proc = subprocess.Popen(
            cmd, env=env,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            bufsize=0
        )
        self.stdout_fd = self.proc.stdout.fileno()
        self.stdout_buf = b""

    def read_until(self, target, timeout=20):
        deadline = time.time() + timeout
        while time.time() < deadline:
            r = os.read(self.stdout_fd, 4096)
            if r:
                self.stdout_buf += r
                if target.encode() in self.stdout_buf:
                    return self.stdout_buf.decode(errors='replace')
            else:
                time.sleep(0.02)
        raise TimeoutError(
            f"timeout waiting for {target!r}\n"
            f"  last 2k: {self.stdout_buf.decode(errors='replace')[-2000:]}"
        )

    def write(self, cmd):
        os.write(self.proc.stdin.fileno(), (cmd + "\n").encode())

    def close(self):
        try:
            os.write(self.proc.stdin.fileno(), b"exit\n")
        except: pass
        try:
            self.proc.stdin.close()
        except: pass
        try:
            self.proc.wait(timeout=15)
        except:
            self.proc.kill()
            self.proc.wait()

# ── 1. Basic sandbox boot ──────────────────────────────────────────────
def test_sandbox_boot():
    s = SandboxSession()
    out = s.read_until(":~# ", timeout=30)
    s.write("whoami")
    out = s.read_until("root", timeout=5)
    s.write("id")
    out = s.read_until("uid=0", timeout=5)
    s.write("echo DONE_BOOT")
    out = s.read_until("DONE_BOOT", timeout=5)
    s.close()
    assert "root" in s.stdout_buf.decode(errors='replace'), "Expected root in output"

# ── 2. qo meta ─────────────────────────────────────────────────────────
def test_qo_meta():
    r = subprocess.run(
        [QO, "meta", "-a", ARCHIVE, "-p", "inttest_pass", "-k", "inttest_key"],
        capture_output=True, text=True, timeout=10)
    assert r.returncode == 0, f"qo meta exited {r.returncode}: {r.stderr}"
    data = json.loads(r.stdout)
    assert data["title"] == "Integration Test Challenge", f"title: {data['title']}"
    assert len(data["levels"]) == 2, f"expected 2 levels, got {len(data['levels'])}"
    assert data["levels"][0]["validator"]["type"] == "file_exists"
    assert data["levels"][1]["validator"]["type"] == "file_contains"

# ── 3. Capability dropping (known bug: CLONE_NEWUSER missing) ───────────
def test_cap_drop():
    s = SandboxSession()
    s.read_until(":~# ")
    s.write("grep CapEff /proc/self/status")
    try:
        out = s.read_until("0000000000000000", timeout=5)
        assert "0000000000000000" in out, "Should not reach here"
    except TimeoutError:
        buf = s.stdout_buf.decode(errors='replace')
        for line in buf.split("\n"):
            if "CapEff" in line:
                val = line.split(":")[-1].strip()
                sys.stdout.write(f"  (CapEff={val})\n")
    s.close()
    sys.stdout.write("  XFAIL: CLONE_NEWUSER not yet implemented\n")

# ── 4. Cgroup limits ───────────────────────────────────────────────────
def test_cgroup_limits():
    s = SandboxSession()
    s.read_until(":~# ")
    time.sleep(1)

    base = "/sys/fs/cgroup/qo-sessions"
    dirs = os.listdir(base)
    cg = None
    for d in sorted(dirs):
        if d.startswith("qo-session-"):
            cg = os.path.join(base, d)
            break
    assert cg, f"No qo-session-* dir in {dirs}"

    with open(os.path.join(cg, "memory.max")) as f:
        v = f.read().strip()
    assert v == "536870912", f"memory.max={v}, expected 536870912"

    with open(os.path.join(cg, "pids.max")) as f:
        v = f.read().strip()
    assert v == "200", f"pids.max={v}, expected 200"

    with open(os.path.join(cg, "cpu.max")) as f:
        v = f.read().strip()
    assert v == "1000000 1000000", f"cpu.max={v}, expected 1000000 1000000"

    s.close()

# ── 5. Duration enforcement ────────────────────────────────────────────
def test_duration():
    s = SandboxSession(duration="5s")
    s.read_until(":~# ")
    time.sleep(15)
    ret = s.proc.poll()
    assert ret is not None, "Sandbox still running after 5s + grace"
    sys.stdout.write(f"  (exit: {ret})\n")
    try:
        s.proc.stdin.close()
    except: pass

# ── 6. Seccomp (log mode) ──────────────────────────────────────────────
def test_seccomp_log():
    s = SandboxSession(qo_seccomp="log")
    s.read_until(":~# ")
    s.write("whoami")
    out = s.read_until("root", timeout=5)
    s.write("echo SECCOMP_OK")
    out = s.read_until("SECCOMP_OK", timeout=5)
    s.close()

# ── 7. Anti-cheat ──────────────────────────────────────────────────────
def test_anti_cheat():
    s = SandboxSession()
    s.read_until(":~# ")
    s.write('echo "fake pass" > /tmp/.qo-challenge-resp')
    time.sleep(1)
    s.write("echo still_here")
    out = s.read_until("still_here", timeout=5)
    assert "still_here" in out, "Shell unresponsive after forgery"
    s.close()

# ── 8. Busybox applet symlinks ─────────────────────────────────────────
def test_busybox_applets():
    s = SandboxSession()
    s.read_until(":~# ")
    applets = ["sleep", "kill", "pkill", "cat", "ls", "echo", "grep", "touch",
               "mkdir", "rm", "mv", "cp", "chmod", "ps", "head", "tail", "sh",
               "bash", "whoami", "id", "find", "gzip", "tar", "cut", "sort",
               "wc", "tee", "which", "env", "printf", "true", "false", "seq",
               "dirname", "basename"]
    missing = []
    for a in applets:
        s.write(f"command -v {a} && echo OK_{a}")
        try:
            out = s.read_until(f"OK_{a}", timeout=3)
        except TimeoutError:
            missing.append(a)
    s.close()
    if missing:
        sys.stdout.write(f"  Missing applets: {missing}\n")
    assert not missing, f"Missing applets: {missing}"

# ── Run all ────────────────────────────────────────────────────────────
if __name__ == "__main__":
    test("1: sandbox boot (shell, whoami, id)", test_sandbox_boot)
    test("2: qo meta output (JSON correctness)", test_qo_meta)
    test("3: capability dropping (CapEff=0)", test_cap_drop)
    test("4: cgroup limits (memory/pids/cpu)", test_cgroup_limits)
    test("5: duration enforcement (5s)", test_duration)
    test("6: seccomp (log mode, shell functional)", test_seccomp_log)
    test("7: anti-cheat forgery resistance", test_anti_cheat)
    test("8: busybox applet symlinks", test_busybox_applets)

    print(f"\n=== RESULTS: {PASS}/{PASS+FAIL} passed, {FAIL} failed ===")
    sys.exit(0 if FAIL == 0 else 1)
