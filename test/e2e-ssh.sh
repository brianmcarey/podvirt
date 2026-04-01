#!/usr/bin/env bash
# e2e SSH test for podvirt — boots a real VM and verifies SSH connectivity.
#
# Uses Fedora Cloud 43 (~600 MB qcow2, cached at ~/.cache/podvirt/).
# Fedora Cloud uses standard cloud-init which scans for LABEL=cidata on any
# attached block device — no special kernel cmdline flags required.
#
# Flow:
#   1. Download (and cache) the Fedora Cloud qcow2, convert to raw
#   2. Create a VM with --disk, --port, --password, --ssh-key
#   3. Start the VM (boots real QEMU via virt-launcher + KVM)
#   4. Verify cloud-init payload in STANDALONE_VMI env var
#   5. Verify podvirt ssh port resolution
#   6. Wait for SSH daemon to come up inside the guest (up to 3 min)
#   7. Verify SSH key login and remote command execution
#   8. Verify `podvirt ssh` CLI command
#   9. Stop and delete
#
# Prerequisites:
#   - /dev/kvm available
#   - Podman socket running
#   - make build already done (./bin/podvirt exists)
#
# Run from repo root:
#   bash test/e2e-ssh.sh

set -uo pipefail

BINARY="${BINARY:-./bin/podvirt}"
VM_NAME="e2e-ssh-$$"
HOST_PORT="$((2200 + ($$ % 1000)))"
SSH_USER="${SSH_USER:-fedora}"
TEST_PASSWORD="testpass-$(( RANDOM % 9000 + 1000 ))"
PASS=0
FAIL=0
DEFAULT_CLOUD_INIT_USER="podvirt"

# Fedora Cloud 43 — standard cloud-init, scans for LABEL=cidata automatically.
FEDORA_URL="https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2"
CACHE_DIR="${HOME}/.cache/podvirt"
# Raw image used by the main test (virt-launcher HostDisk is always raw).
DISK_IMAGE="${CACHE_DIR}/fedora-cloud-43.raw"
# qcow2 image used by the qcow2 wrapper injection test.
QCOW2_IMAGE="${CACHE_DIR}/fedora-cloud-43.qcow2"

# qcow2 test VM (separate name/port to run after main test)
QCOW2_VM_NAME="e2e-qcow2-$$"
QCOW2_PORT="$((2200 + ($$ % 1000) + 1))"
QCOW2_PASSWORD="qcow2pass-$(( RANDOM % 9000 + 1000 ))"

# ── helpers ────────────────────────────────────────────────────────────────
green() { printf '\033[32m✔ %s\033[0m\n' "$*"; }
red()   { printf '\033[31m✗ %s\033[0m\n' "$*"; }

pass() { green "$1"; (( PASS++ )) || true; }
fail() { red "$1";   (( FAIL++ )) || true; }

assert_contains() {
    local label="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        pass "$label"
    else
        fail "$label (expected to find: '$needle')"
        printf '  output was: %s\n' "$haystack"
    fi
}

assert_not_contains() {
    local label="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        fail "$label (unexpected: '$needle' found)"
        printf '  output was: %s\n' "$haystack"
    else
        pass "$label"
    fi
}

cleanup() {
    echo ""
    echo "--- Cleanup ---"
    "$BINARY" stop   "$VM_NAME" --force 2>&1 | grep -v "^$" || true
    "$BINARY" delete "$VM_NAME" --force 2>&1 | grep -v "^$" || true
    [[ -n "${RUN_DISK:-}" ]] && rm -f "$RUN_DISK"
    [[ -n "${KEY_DIR:-}" ]]  && rm -rf "$KEY_DIR"
    "$BINARY" stop   "$QCOW2_VM_NAME" --force 2>&1 | grep -v "^$" || true
    "$BINARY" delete "$QCOW2_VM_NAME" --force 2>&1 | grep -v "^$" || true
    [[ -n "${QCOW2_RUN_DISK:-}" ]] && rm -f "$QCOW2_RUN_DISK"
    echo "Cleaned up $VM_NAME and $QCOW2_VM_NAME"
}
trap cleanup EXIT

# ── pre-flight ─────────────────────────────────────────────────────────────
[[ -e /dev/kvm ]] || { echo "SKIP: /dev/kvm not available"; exit 0; }
[[ -x "$BINARY" ]] || { echo "FATAL: $BINARY not found (run 'make build')"; exit 1; }

# ── download Fedora Cloud image (cached) ─────────────────────────────────
mkdir -p "$CACHE_DIR"
if [[ ! -f "$DISK_IMAGE" ]]; then
    QCOW2_TMP="${CACHE_DIR}/fedora-cloud-43.qcow2"
    if [[ ! -f "$QCOW2_TMP" ]]; then
        echo "Downloading Fedora Cloud 43 image (~600 MB)..."
        curl -kL --progress-bar -o "${QCOW2_TMP}.tmp" "$FEDORA_URL"
        mv "${QCOW2_TMP}.tmp" "$QCOW2_TMP"
    fi
    fmt=$(qemu-img info "$QCOW2_TMP" 2>/dev/null | awk '/^file format/{print $3}')
    if [[ "$fmt" == "raw" ]]; then
        echo "Image is raw, moving directly to $DISK_IMAGE..."
        mv "$QCOW2_TMP" "$DISK_IMAGE"
    else
        echo "Converting ${fmt:-qcow2} → raw (cached at $DISK_IMAGE)..."
        qemu-img convert -f "${fmt:-qcow2}" -O raw "$QCOW2_TMP" "$DISK_IMAGE"
        # Keep QCOW2_TMP as the cached qcow2 (used by the qcow2 e2e test below).
    fi
    echo "Done."
else
    echo "Using cached image: $DISK_IMAGE"
fi

# Ensure the qcow2 is also cached for the qcow2 wrapper test.
if [[ ! -f "$QCOW2_IMAGE" ]]; then
    echo "Creating cached qcow2 from raw for qcow2 e2e test..."
    qemu-img convert -f raw -O qcow2 "$DISK_IMAGE" "$QCOW2_IMAGE"
    echo "qcow2 cached at $QCOW2_IMAGE"
fi

# Make a reflink or full copy for this run so the original stays unmodified.
RUN_DISK="${CACHE_DIR}/run-${VM_NAME}.img"
if cp --reflink=always "$DISK_IMAGE" "$RUN_DISK" 2>/dev/null; then
    echo "Run disk: reflink copy → $RUN_DISK"
else
    cp "$DISK_IMAGE" "$RUN_DISK"
    echo "Run disk: full copy → $RUN_DISK"
fi

# Per-run qcow2 copy for the qcow2 wrapper test.
QCOW2_RUN_DISK="${CACHE_DIR}/run-${QCOW2_VM_NAME}.qcow2"
if cp --reflink=always "$QCOW2_IMAGE" "$QCOW2_RUN_DISK" 2>/dev/null; then
    echo "qcow2 run disk: reflink copy → $QCOW2_RUN_DISK"
else
    cp "$QCOW2_IMAGE" "$QCOW2_RUN_DISK"
    echo "qcow2 run disk: full copy → $QCOW2_RUN_DISK"
fi

# ── per-run SSH keypair ────────────────────────────────────────────────────
KEY_DIR=$(mktemp -d)
KEY_FILE="$KEY_DIR/id_test"
ssh-keygen -t ed25519 -N "" -f "$KEY_FILE" -C "podvirt-e2e" -q
KEY_TYPE=$(awk '{print $1}' "${KEY_FILE}.pub")
KEY_BODY=$(awk '{print $2}' "${KEY_FILE}.pub")

echo ""
echo "================================================"
echo " podvirt SSH e2e test"
echo " VM name   : $VM_NAME"
echo " Disk      : $RUN_DISK"
echo " Host port : $HOST_PORT → VM:22"
echo " SSH user  : $SSH_USER"
echo " Binary    : $BINARY"
echo "================================================"
echo ""

# ── 1. Create ──────────────────────────────────────────────────────────────
echo "--- Create VM ---"
out=$("$BINARY" create \
    --name     "$VM_NAME" \
    --disk     "$RUN_DISK" \
    --port     "${HOST_PORT}:22" \
    --password "$TEST_PASSWORD" \
    --ssh-key  "${KEY_FILE}.pub" \
    --user     "$SSH_USER" \
    --memory   "1Gi" \
    --cpus     1 2>&1) || true
echo "$out"
assert_contains "create: succeeds"           "created"       "$out"
assert_contains "create: shows port forward" "$HOST_PORT"    "$out"
assert_contains "create: shows name"         "$VM_NAME"      "$out"
assert_contains "create: shows start hint"   "podvirt start" "$out"

# ── 2. Start ───────────────────────────────────────────────────────────────
echo ""
echo "--- Start ---"
out=$("$BINARY" start "$VM_NAME" 2>&1) || true
echo "$out"
assert_contains "start: succeeds" "started" "$out"

# ── 3. Port mapping in list JSON ───────────────────────────────────────────
echo ""
echo "--- List JSON ---"
list_json=$("$BINARY" list --output json 2>&1) || true
echo "$list_json"
assert_contains "list json: VM present"     "$VM_NAME"             "$list_json"
assert_contains "list json: host port"      "$HOST_PORT"           "$list_json"
assert_contains "list json: container port" '"containerPort":22'   "$list_json"
assert_contains "list json: protocol"       '"protocol":"tcp"'     "$list_json"

# ── 4. cloud-init payload ──────────────────────────────────────────────────
echo ""
echo "--- cloud-init: inspect STANDALONE_VMI env var ---"
vmi_json=$(podman inspect "podvirt-${VM_NAME}" \
    --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | grep '^STANDALONE_VMI=' \
    | sed 's/^STANDALONE_VMI=//') || true

if [[ -z "$vmi_json" ]]; then
    fail "cloud-init: STANDALONE_VMI env var not found in container"
else
    pass "cloud-init: STANDALONE_VMI env var present"
    assert_contains "cloud-init: cloudInitNoCloud volume present" "cloudInitNoCloud" "$vmi_json"
    assert_contains "cloud-init: password injected into userData" "$TEST_PASSWORD"   "$vmi_json"
    assert_contains "cloud-init: SSH key type in userData"        "$KEY_TYPE"        "$vmi_json"
    assert_contains "cloud-init: SSH key body in userData"        "$KEY_BODY"        "$vmi_json"
    assert_contains "cloud-init: ssh_pwauth enabled"              "ssh_pwauth"       "$vmi_json"
    assert_contains "cloud-init: passt port 22 in VMI"           '"port":22'        "$vmi_json"
fi

# ── 5. podvirt ssh: port resolution ────────────────────────────────────────
echo ""
echo "--- podvirt ssh: port resolution ---"
ssh_err=$("$BINARY" ssh "$VM_NAME" --user "$SSH_USER" --identity "$KEY_FILE" -- "exit 0" 2>&1) || true
assert_not_contains "podvirt ssh: not 'no port 22 mapping' error" "no port 22 mapping"  "$ssh_err"
assert_not_contains "podvirt ssh: not 'VM not found' error"       "not found"            "$ssh_err"
assert_contains     "podvirt ssh: shows connection attempt"       "localhost:${HOST_PORT}" "$ssh_err"

# ── 6. Wait for SSH daemon ─────────────────────────────────────────────────
echo ""
echo "--- Waiting for VM to boot and SSH to become available (up to 3 min) ---"
SSH_READY=0
for i in $(seq 1 36); do
    if ssh -4 \
        -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=4 \
        -o BatchMode=yes \
        -i "$KEY_FILE" \
        -p "$HOST_PORT" \
        "${SSH_USER}@localhost" \
        "echo podvirt-ssh-ok" 2>/dev/null | grep -q "podvirt-ssh-ok"; then
        SSH_READY=1
        echo "  SSH is ready (attempt $i)."
        break
    fi
    printf '  attempt %2d/36: not ready yet, waiting 5s...\n' "$i"
    sleep 5
done

if [[ "$SSH_READY" -eq 0 ]]; then
    fail "SSH: daemon never became available on port ${HOST_PORT} after 3 minutes"
    echo "  --- container logs (last 30 lines) ---"
    podman logs "podvirt-${VM_NAME}" 2>&1 | tail -30 || true
else
    pass "SSH: daemon up and accepting key auth"

    # ── 7. SSH key login: run a remote command ─────────────────────────────
    echo ""
    echo "--- SSH key login: run remote command ---"
    ssh_out=$(ssh -4 \
        -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=10 \
        -i "$KEY_FILE" \
        -p "$HOST_PORT" \
        "${SSH_USER}@localhost" \
        "hostname && uname -s && id" 2>&1) || true
    echo "$ssh_out"
    assert_contains "SSH: hostname returned"  "."      "$ssh_out"
    assert_contains "SSH: kernel reported"    "Linux"  "$ssh_out"

    # ── 8. podvirt ssh: run command via CLI ───────────────────────────────
    echo ""
    echo "--- podvirt ssh: run remote command via CLI ---"
    podvirt_out=$("$BINARY" ssh "$VM_NAME" \
        --user     "$SSH_USER" \
        --identity "$KEY_FILE" \
        -- "echo podvirt-cmd-ok && hostname" 2>&1) || true
    echo "$podvirt_out"
    assert_contains     "podvirt ssh: command executed" "podvirt-cmd-ok" "$podvirt_out"
    assert_not_contains "podvirt ssh: no error"         "Error:"         "$podvirt_out"
fi

# ── 9. Status ─────────────────────────────────────────────────────────────
echo ""
echo "--- Status ---"
out=$("$BINARY" status "$VM_NAME" 2>&1) || true
echo "$out"
assert_contains "status: shows VM name"         "$VM_NAME"   "$out"
assert_contains "status: shows container field" "Container"  "$out"

# ── 10. Stop ──────────────────────────────────────────────────────────────
echo ""
echo "--- Stop ---"
out=$("$BINARY" stop "$VM_NAME" 2>&1) || true
echo "$out"
assert_contains "stop: succeeds" "stopped" "$out"

# ── 11. Delete ────────────────────────────────────────────────────────────
echo ""
echo "--- Delete ---"
out=$("$BINARY" delete "$VM_NAME" --force 2>&1) || true
echo "$out"
assert_contains "delete: succeeds" "deleted" "$out"

# ── 12. podvirt ssh on deleted VM fails cleanly ────────────────────────────
echo ""
echo "--- podvirt ssh on deleted VM ---"
out=$("$BINARY" ssh "$VM_NAME" --user "$SSH_USER" 2>&1) || true
assert_contains "podvirt ssh: non-existent VM rejected" "not found" "$out"

# ── 13. qcow2 format-node injection ────────────────────────────────────────
echo ""
echo "--- qcow2 format-node injection test ---"
QCOW2_LIBVIRT_LOG="${HOME}/.cache/podvirt/libvirt-logs/default_${QCOW2_VM_NAME}.log"

out=$(PODVIRT_DEBUG=1 "$BINARY" create \
    --name      "$QCOW2_VM_NAME" \
    --disk      "$QCOW2_RUN_DISK" \
    --port      "${QCOW2_PORT}:22" \
    --password  "$QCOW2_PASSWORD" \
    --ssh-key   "${KEY_FILE}.pub" 2>&1)
assert_contains "qcow2: create succeeds" "created" "$out"
echo "$out"

out=$(PODVIRT_DEBUG=1 "$BINARY" start "$QCOW2_VM_NAME" 2>&1)
assert_contains "qcow2: start succeeds" "started" "$out"
echo "$out"

echo "Waiting for qcow2 VM to become reachable (up to 180s)..."
QCOW2_OK=0
for i in $(seq 1 36); do
    if ssh -4 -o StrictHostKeyChecking=no \
          -o UserKnownHostsFile=/dev/null \
          -o BatchMode=yes \
          -o ConnectTimeout=5 \
          -i "$KEY_FILE" \
          -p "$QCOW2_PORT" \
          "${DEFAULT_CLOUD_INIT_USER}@127.0.0.1" "echo alive" 2>/dev/null | grep -q alive; then
        QCOW2_OK=1
        break
    fi
    sleep 5
done
if [[ $QCOW2_OK -eq 1 ]]; then
    pass "qcow2 VM: SSH reachable after boot"
else
    fail "qcow2 VM: SSH not reachable within 180s"
fi

# Poll for the persisted libvirt launch log after the guest has had time to
# boot. libvirt sanitizes the emulator environment, so qemu-wrapper debug logs
# are not reliable here even when PODVIRT_DEBUG=1 is set on the container.
QCOW2_LOG_OK=0
for _i in $(seq 1 6); do
    if [[ -f "$QCOW2_LIBVIRT_LOG" ]] \
        && grep -qF "/usr/libexec/qemu-kvm" "$QCOW2_LIBVIRT_LOG" \
        && grep -qF "$(basename "$QCOW2_RUN_DISK")" "$QCOW2_LIBVIRT_LOG"; then
        QCOW2_LOG_OK=1
        break
    fi
    sleep 5
done
if [[ $QCOW2_LOG_OK -eq 1 ]]; then
    pass "qcow2 launch log: qemu invoked with qcow2 disk"
else
    fail "qcow2 launch log: expected qemu launch with $(basename "$QCOW2_RUN_DISK")"
    [[ -f "$QCOW2_LIBVIRT_LOG" ]] && tail -n 40 "$QCOW2_LIBVIRT_LOG"
fi

out=$("$BINARY" stop   "$QCOW2_VM_NAME" --timeout 5 2>&1); echo "$out"
out=$("$BINARY" delete "$QCOW2_VM_NAME" --force   2>&1); echo "$out"

echo ""
echo "================================================"
printf ' Results: %d passed, %d failed\n' "$PASS" "$FAIL"
echo "================================================"
echo ""

[[ $FAIL -eq 0 ]]
