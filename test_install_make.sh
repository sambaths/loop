#!/usr/bin/env bash
set -euo pipefail

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT

echo "=== Test: make install target exists and syntax is valid ==="
make -n install > /dev/null 2>&1 || { echo "FAIL: make -n install failed"; exit 1; }
echo "PASS"

echo "=== Test: make install to temp directory ==="
make install INSTALL_DIR="$TEST_DIR" > /dev/null 2>&1 || { echo "FAIL: make install failed"; exit 1; }
echo "PASS"

echo "=== Test: installed binary exists ==="
[ -f "$TEST_DIR/loop" ] || { echo "FAIL: binary not found at $TEST_DIR/loop"; exit 1; }
echo "PASS"

echo "=== Test: installed binary is executable ==="
[ -x "$TEST_DIR/loop" ] || { echo "FAIL: binary not executable"; exit 1; }
echo "PASS"

echo "=== Test: installed binary --version runs ==="
"$TEST_DIR/loop" --version > /dev/null 2>&1 || { echo "FAIL: binary --version failed"; exit 1; }
echo "PASS"

echo "=== Test: make install with INSTALL_DIR override ==="
SUB_DIR="$TEST_DIR/sub"
make install INSTALL_DIR="$SUB_DIR" > /dev/null 2>&1 || { echo "FAIL: make install with override failed"; exit 1; }
[ -f "$SUB_DIR/loop" ] || { echo "FAIL: binary not found at override path"; exit 1; }
echo "PASS"

echo ""
echo "All tests passed!"
