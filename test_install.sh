#!/usr/bin/env bash
set -euo pipefail

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT

echo "=== Test: install.sh --help shows usage ==="
output=$(./install.sh --help 2>&1) || true
echo "$output" | grep -qi "usage" || { echo "FAIL: --help missing usage"; exit 1; }
output=$(./install.sh -h 2>&1) || true
echo "$output" | grep -qi "usage" || { echo "FAIL: -h missing usage"; exit 1; }
echo "PASS"

echo "=== Test: install.sh to temp directory ==="
./install.sh "$TEST_DIR" > /dev/null 2>&1 || { echo "FAIL: install.sh failed"; exit 1; }
echo "PASS"

echo "=== Test: installed binary exists ==="
[ -f "$TEST_DIR/bin/loop" ] || { echo "FAIL: binary not found at $TEST_DIR/bin/loop"; exit 1; }
echo "PASS"

echo "=== Test: installed binary is executable ==="
[ -x "$TEST_DIR/bin/loop" ] || { echo "FAIL: binary not executable"; exit 1; }
echo "PASS"

echo "=== Test: installed binary --version shows dev ==="
"$TEST_DIR/bin/loop" --version 2>&1 | grep -q "loop vdev" || { echo "FAIL: version output mismatch"; exit 1; }
echo "PASS"

echo "=== Test: install.sh with custom VERSION env ==="
VERSION=1.0.0 ./install.sh "$TEST_DIR" > /dev/null 2>&1 || { echo "FAIL: install.sh with VERSION failed"; exit 1; }
"$TEST_DIR/bin/loop" --version 2>&1 | grep -q "loop v1.0.0" || { echo "FAIL: custom version output mismatch"; exit 1; }
echo "PASS"

echo "=== Test: install.sh --version flag (without v) ==="
./install.sh "$TEST_DIR" --version 2.0.0 > /dev/null 2>&1 || { echo "FAIL: install.sh --version failed"; exit 1; }
"$TEST_DIR/bin/loop" --version 2>&1 | grep -q "loop v2.0.0" || { echo "FAIL: --version output mismatch"; exit 1; }
echo "PASS"

echo "=== Test: install.sh --version flag (with v prefix) ==="
./install.sh --version v0.1.0 "$TEST_DIR" > /dev/null 2>&1 || { echo "FAIL: install.sh --version v0.1.0 failed"; exit 1; }
"$TEST_DIR/bin/loop" --version 2>&1 | grep -q "loop v0.1.0" || { echo "FAIL: --version v0.1.0 output mismatch"; exit 1; }
echo "PASS"

echo "=== Test: install.sh --dir flag ==="
SUB_DIR="$TEST_DIR/subdir"
./install.sh --dir "$SUB_DIR" > /dev/null 2>&1 || { echo "FAIL: install.sh --dir failed"; exit 1; }
[ -f "$SUB_DIR/loop" ] || { echo "FAIL: binary not found at $SUB_DIR/loop"; exit 1; }
echo "PASS"

echo "=== Test: install.sh fallback to \$HOME/bin ==="
# Make .local/bin unwritable by pointing to a non-existent dir via --dir
# that has a non-writable parent
NON_WRITABLE=$(mktemp -d)
chmod 000 "$NON_WRITABLE"
# Use a PREFIX inside the non-writable dir so the writability check fails
# and triggers fallback
FALLBACK_TEST_DIR=$(mktemp -d)
mkdir -p "$FALLBACK_TEST_DIR/bin"
# Temporarily override HOME to test fallback
HOME_SAVE="$HOME"
HOME="$FALLBACK_TEST_DIR"
# When HOME/.local/bin doesn't exist and HOME/.local is not writable,
# it should fallback to HOME/bin
mkdir -p "$HOME/.local"
chmod 000 "$HOME/.local"
output=$(./install.sh 2>&1 || true)
chmod 755 "$HOME/.local" 2>/dev/null || true
HOME="$HOME_SAVE"
echo "$output" | grep -q "$FALLBACK_TEST_DIR/bin" || { echo "FAIL: fallback to \$HOME/bin not detected"; echo "Output: $output"; exit 1; }
echo "PASS"

echo ""
echo "All tests passed!"
