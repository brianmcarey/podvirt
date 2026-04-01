#!/usr/bin/env bash
# e2e smoke test for podvirt
# Uses a container disk (no local image needed) so it's self-contained.
# Run from the repo root: bash test/e2e-smoke.sh

set -uo pipefail

BINARY="${BINARY:-./bin/podvirt}"
VM_NAME="e2e-smoke-$$"
PASS=0
FAIL=0

green() { printf '\033[32m✔ %s\033[0m\n' "$*"; }
red()   { printf '\033[31m✗ %s\033[0m\n' "$*"; }

pass() { green "$1"; (( PASS++ )) || true; }
fail() { red "$1"; (( FAIL++ )) || true; }

assert_contains() {
    local label="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -q "$needle"; then
        pass "$label"
    else
        fail "$label (expected to find: '$needle')"
        echo "  output was: $haystack"
    fi
}

assert_not_contains() {
    local label="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -q "$needle"; then
        fail "$label (expected NOT to find: '$needle')"
        echo "  output was: $haystack"
    else
        pass "$label"
    fi
}

cleanup() {
    echo ""
    echo "--- Cleanup ---"
    "$BINARY" delete "$VM_NAME" --force 2>/dev/null && echo "Deleted $VM_NAME" || echo "Nothing to clean up"
}
trap cleanup EXIT

echo "================================================"
echo " podvirt e2e smoke test"
echo " VM name : $VM_NAME"
echo " Binary  : $BINARY"
echo "================================================"
echo ""

# ── 1. Create ─────────────────────────────────────────────────────────────────
echo "--- Create ---"
out=$("$BINARY" create \
    --name "$VM_NAME" \
    --cpus 1 \
    --memory 512Mi \
    --image quay.io/containerdisks/fedora:43 2>&1) || true
echo "$out"
assert_contains "create: succeeds"        "created" "$out"
assert_contains "create: shows name"      "$VM_NAME" "$out"
assert_contains "create: start hint"      "podvirt start" "$out"

# ── 2. Duplicate create should fail ───────────────────────────────────────────
echo ""
echo "--- Duplicate create (should fail) ---"
out=$("$BINARY" create --name "$VM_NAME" --image quay.io/containerdisks/fedora:43 2>&1 || true)
echo "$out"
assert_contains "create: duplicate rejected" "already exists" "$out"

# ── 3. List (created, not yet started) ────────────────────────────────────────
echo ""
echo "--- List ---"
out=$("$BINARY" list 2>&1) || true
echo "$out"
assert_contains "list: VM appears" "$VM_NAME" "$out"

# ── 4. Start ──────────────────────────────────────────────────────────────────
echo ""
echo "--- Start ---"
out=$("$BINARY" start "$VM_NAME" 2>&1) || true
echo "$out"
assert_contains "start: succeeds" "$VM_NAME" "$out"

# ── 5. List shows running ─────────────────────────────────────────────────────
echo ""
echo "--- List (after start) ---"
out=$("$BINARY" list 2>&1) || true
echo "$out"
assert_contains "list: VM still appears" "$VM_NAME" "$out"

# ── 6. Status ─────────────────────────────────────────────────────────────────
echo ""
echo "--- Status ---"
out=$("$BINARY" status "$VM_NAME" 2>&1) || true
echo "$out"
assert_contains "status: shows VM name" "$VM_NAME" "$out"

# ── 7. List JSON output ───────────────────────────────────────────────────────
echo ""
echo "--- List --output json ---"
out=$("$BINARY" list --output json 2>&1) || true
echo "$out"
assert_contains "list json: contains VM name" "$VM_NAME" "$out"

# ── 8. List YAML output ───────────────────────────────────────────────────────
echo ""
echo "--- List --output yaml ---"
out=$("$BINARY" list --output yaml 2>&1) || true
echo "$out"
assert_contains "list yaml: contains VM name" "$VM_NAME" "$out"

# ── 9. Stop ───────────────────────────────────────────────────────────────────
echo ""
echo "--- Stop ---"
out=$("$BINARY" stop "$VM_NAME" 2>&1) || true
echo "$out"
assert_contains "stop: succeeds" "$VM_NAME" "$out"

# ── 10. Delete ────────────────────────────────────────────────────────────────
echo ""
echo "--- Delete ---"
out=$("$BINARY" delete "$VM_NAME" --force 2>&1) || true
echo "$out"
assert_contains "delete: succeeds" "$VM_NAME" "$out"

# ── 11. List after delete ─────────────────────────────────────────────────────
echo ""
echo "--- List (after delete) ---"
out=$("$BINARY" list 2>&1) || true
echo "$out"
assert_not_contains "list: VM gone after delete" "$VM_NAME" "$out"

# ── 12. Start non-existent VM ────────────────────────────────────────────────
echo ""
echo "--- Start non-existent VM (should fail) ---"
out=$("$BINARY" start "no-such-vm-$$" 2>&1 || true)
echo "$out"
assert_contains "start: non-existent VM rejected" "not found" "$out"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "================================================"
printf ' Results: \033[32m%d passed\033[0m, \033[31m%d failed\033[0m\n' "$PASS" "$FAIL"
echo "================================================"

[[ $FAIL -eq 0 ]]
