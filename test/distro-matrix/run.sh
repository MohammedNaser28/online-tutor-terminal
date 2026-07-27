#!/usr/bin/env bash
set -euo pipefail

# qo Distro Test Matrix — Docker-based compatibility survey
# Tests qo sandbox behavior across Linux distros students are likely to use.
# Usage: ./run.sh [distro_key]    # run all, or one of: ubuntu-22.04 ubuntu-24.04 debian-12 fedora-latest arch-latest pop

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESULTS="$SCRIPT_DIR/results"
mkdir -p "$RESULTS"

# ── Distro definitions ─────────────────────────────────────────────────
declare -A CFG

distro() {
    local k="$1"
    CFG[${k}_name]="$2"
    CFG[${k}_image]="$3"
    CFG[${k}_install]="$4"
}

distro ubuntu-22.04 "Ubuntu 22.04" "ubuntu:22.04" \
    "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gcc libseccomp-dev python3 2>&1 | tail -1"

distro ubuntu-24.04 "Ubuntu 24.04" "ubuntu:24.04" \
    "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gcc libseccomp-dev python3 2>&1 | tail -1"

distro debian-12 "Debian 12" "debian:12" \
    "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gcc libseccomp-dev python3 2>&1 | tail -1"

distro fedora-latest "Fedora (latest)" "fedora:latest" \
    "dnf install -y -q gcc libseccomp-devel python3 2>&1 | tail -1"

distro arch-latest "Arch Linux (baseline)" "archlinux:latest" \
    "pacman -Sy --noconfirm gcc libseccomp python 2>&1 | tail -1"

ALL_KEYS=(ubuntu-22.04 ubuntu-24.04 debian-12 fedora-latest arch-latest)

# ── Per-distro run ─────────────────────────────────────────────────────

run_distro() {
    local key="$1"
    local name="${CFG[${key}_name]}"
    local image="${CFG[${key}_image]}"
    local install="${CFG[${key}_install]}"

    echo ""
    echo "══════════════════════════════════════════════════════════════"
    echo "  $name  ($key)  —  $image"
    echo "══════════════════════════════════════════════════════════════"

    local out="$RESULTS/$key.out"
    local json="$RESULTS/$key.json"

    # Build the inline runner script that runs inside the container
    # We write it to a temp file and mount it to avoid quoting hell
    cat > /tmp/qo-dm-runner-${key}.sh << RUNNER
#!/usr/bin/env bash
set -euo pipefail

NAME='$name'
KEY='$key'
INSTALL_CMD='$install'

echo "=== Installing dependencies ==="
eval "\$INSTALL_CMD"

echo "=== Pre-checks ==="

CGROUP_V=\$(stat -fc %T /sys/fs/cgroup/ 2>/dev/null || echo "NOT_FOUND")
echo "RESULT:CGROUP_V:\$CGROUP_V"

KERNEL=\$(uname -r)
echo "RESULT:KERNEL:\$KERNEL"

USERNS=\$(sysctl kernel.unprivileged_userns_clone 2>/dev/null || echo "not_present")
echo "RESULT:USERNS:\$USERNS"

APPARMOR=""
if command -v aa-status &>/dev/null; then
    AA_USRNS=\$(aa-status 2>/dev/null | grep -i -E "(userns|unprivileged)" || true)
    if [ -n "\$AA_USRNS" ]; then
        APPARMOR="present: \$AA_USRNS"
    else
        APPARMOR="no_userns_rules"
    fi
else
    APPARMOR="not_installed"
fi
echo "RESULT:APPARMOR:\$APPARMOR"

LIBS=""
for lib in libseccomp.so.2 libcap.so.2; do
    if ldconfig -p 2>/dev/null | grep -q "\$lib"; then
        LIBS="\${LIBS}\${lib}=found "
    elif find /usr/lib /lib /usr/lib64 -name "\${lib}*" 2>/dev/null | grep -q .; then
        LIBS="\${LIBS}\${lib}=found "
    else
        LIBS="\${LIBS}\${lib}=MISSING "
    fi
done
echo "RESULT:LIBS:\$LIBS"

echo "=== Building qo-init from source ==="
gcc -o /usr/local/bin/qo-init /src/qo-init.c -lseccomp 2>&1
file /usr/local/bin/qo-init 2>/dev/null | cut -d: -f2- || true
echo "qo-init build complete"

echo "=== Running integration tests ==="
set +e
python3 -u /test.py 2>&1 | tee /tmp/test_out
set -e

# shellcheck disable=SC2126
PASS_COUNT=\$(grep '^  PASS' /tmp/test_out 2>/dev/null | wc -l || true)
FAIL_COUNT=\$(grep '^  FAIL' /tmp/test_out 2>/dev/null | wc -l || true)
echo "RESULT:PASS:\$PASS_COUNT"
echo "RESULT:FAIL:\$FAIL_COUNT"

CUR_TEST=""
while IFS= read -r line; do
    if echo "\$line" | grep -qP '^=== \d+:'; then
        CUR_TEST=\$(echo "\$line" | sed 's/^=== \([0-9]\+:.*\) ===\$/\1/')
    fi
    if echo "\$line" | grep -q '^  FAIL'; then
        echo "RESULT:FAIL_ITEM:\$CUR_TEST"
    fi
done < /tmp/test_out

echo "=== Done ==="
RUNNER
    chmod +x "/tmp/qo-dm-runner-${key}.sh"

    docker run --rm --privileged --cgroupns=host \
        -v "$PROJECT/deploy/qo:/usr/local/bin/qo:ro" \
        -v "$PROJECT/deploy/qo-init.c:/src/qo-init.c:ro" \
        -v "$SCRIPT_DIR/integration_test.py:/test.py:ro" \
        -v "$SCRIPT_DIR/archive.enc:/archive.enc:ro" \
        -v "/tmp/qo-dm-runner-${key}.sh:/runner.sh:ro" \
        "$image" \
        /runner.sh 2>&1 | tee "$out"

    echo "  → Raw output: $out"

    # Parse RESULT: lines into JSON
    local kernel="" cgroup_v="" userns="" apparmor="" libs=""
    local pass=0 fail=0 fail_list="[]"

    while IFS= read -r line; do
        line="${line#RESULT:}"
        local tag="${line%%:*}"
        local val="${line#*:}"
        case "$tag" in
            "KERNEL")    kernel="$val" ;;
            "CGROUP_V")  cgroup_v="$val" ;;
            "USERNS")    userns="$val" ;;
            "APPARMOR")  apparmor="$val" ;;
            "LIBS")      libs="$val" ;;
            "PASS")      pass="$val" ;;
            "FAIL")      fail="$val" ;;
            "FAIL_ITEM") fail_list=$(python3 -c "import json; l=json.loads('$fail_list'.replace(chr(92),'')); l.append('$val'); print(json.dumps(l))") ;;
        esac
    done < <(grep '^RESULT:' "$out")

    # Generate JSON
    python3 -c "
import json
r = {
    'distro': '$name',
    'key': '$key',
    'kernel': '$(echo "$kernel" | sed "s/'//g")',
    'cgroup_version': '$(echo "$cgroup_v" | sed "s/'//g")',
    'unprivileged_userns_clone': '$(echo "$userns" | sed "s/'//g")',
    'apparmor_userns': '$(echo "$apparmor" | sed "s/'//g")',
    'libraries': '$(echo "$libs" | sed "s/'//g")',
    'pass': $pass,
    'fail': $fail,
    'failures': $fail_list,
}
print(json.dumps(r, indent=2))
" > "$json"

    echo "  → JSON: $json"
    rm -f "/tmp/qo-dm-runner-${key}.sh"
}

# ── Main ───────────────────────────────────────────────────────────────

TARGET="${1:-all}"

if [ "$TARGET" = "all" ]; then
    for k in "${ALL_KEYS[@]}"; do
        run_distro "$k"
    done
elif [ "$TARGET" = "pop" ]; then
    echo "Pop!_OS: proxied via Ubuntu 24.04 (not independently verified)"
    echo "Pop!_OS shares Ubuntu's package base; results assumed to carry over."
    run_distro ubuntu-24.04
else
    # Check if valid key
    valid=0
    for k in "${ALL_KEYS[@]}"; do [ "$k" = "$TARGET" ] && valid=1; done
    if [ "$valid" = 1 ]; then
        run_distro "$TARGET"
    else
        echo "Unknown distro: $TARGET"
        echo "Valid: ${ALL_KEYS[*]} pop"
        exit 1
    fi
fi

# ── Summary table ──────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════════════════════════"
echo "  SUMMARY"
echo "══════════════════════════════════════════════════════════════"
echo ""

python3 << EOF
import json, os, glob

results_dir = "$RESULTS"
results = []
for key in ['ubuntu-22.04', 'ubuntu-24.04', 'debian-12', 'fedora-latest', 'arch-latest']:
    path = os.path.join(results_dir, f'{key}.json')
    if os.path.exists(path):
        with open(path) as f:
            results.append(json.load(f))

# Markdown table
print('| Distro | Kernel | Cgroup | Unpriv Userns | AppArmor | libseccomp | libcap | Tests |')
print('|--------|--------|--------|---------------|----------|------------|--------|-------|')
for r in results:
    libs = r.get('libraries', '')
    lsec = 'found' if 'libseccomp.so.2=found' in libs else 'MISSING'
    lcap = 'found' if 'libcap.so.2=found' in libs else 'MISSING'
    tests = f"{r['pass']}/{r['pass']+r['fail']}"
    fails = r.get('failures', [])
    note = '' if not fails else ' ⚠️ ' + ', '.join(fails)
    print(f"| {r['distro']} | {r['kernel'][:25]} | {r['cgroup_version']} | {r['unprivileged_userns_clone']} | {r['apparmor_userns']} | {lsec} | {lcap} | {tests}{note} |")

# Combined JSON
combined_path = os.path.join(results_dir, 'combined.json')
with open(combined_path, 'w') as f:
    json.dump({
        'distros': results,
        'pop_os_note': 'Proxied via Ubuntu 24.04 — not independently verified',
        'known_bug_cap_drop': 'CLONE_NEWUSER not yet implemented; CapEff != 0 on all distros'
    }, f, indent=2)

print(f"\nCombined JSON: {combined_path}")
EOF
