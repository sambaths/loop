#!/usr/bin/env bash
set -euo pipefail

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT

echo "=== Test: make -n install-go (verify target exists and syntax is valid) ==="
make -n install-go > /dev/null 2>&1 || { echo "FAIL: make -n install-go failed"; exit 1; }
echo "PASS"

echo "=== Test: make install-go to temp directory ==="
make install-go GO_INSTALL_DIR="$TEST_DIR" > /dev/null 2>&1 || { echo "FAIL: make install-go failed"; exit 1; }
echo "PASS"

echo "=== Test: installed Go binary works ==="
"$TEST_DIR/go/bin/go" version > /dev/null 2>&1 || { echo "FAIL: go version failed"; exit 1; }
echo "PASS"

echo "=== Test: installed Go meets minimum version ==="
MIN_GO_VERSION="1.22"
INSTALLED_VERSION=$("$TEST_DIR/go/bin/go" version | grep -oP 'go\d+\.\d+')
CLEAN_VERSION=$(echo "$INSTALLED_VERSION" | sed 's/go//')
MAJOR=$(echo "$CLEAN_VERSION" | cut -d. -f1)
MINOR=$(echo "$CLEAN_VERSION" | cut -d. -f2)
if [ "$MAJOR" -lt 1 ] || { [ "$MAJOR" -eq 1 ] && [ "$MINOR" -lt 22 ]; }; then
    echo "FAIL: Go version $CLEAN_VERSION is below minimum $MIN_GO_VERSION"
    exit 1
fi
echo "PASS"

echo "=== Test: installed Go can compile a simple program ==="
echo 'package main; import "fmt"; func main() { fmt.Println("hello") }' > "$TEST_DIR/hello.go"
"$TEST_DIR/go/bin/go" build -o "$TEST_DIR/hello" "$TEST_DIR/hello.go" > /dev/null 2>&1 || { echo "FAIL: go build failed"; exit 1; }
"$TEST_DIR/hello" | grep -q "hello" || { echo "FAIL: hello output mismatch"; exit 1; }
echo "PASS"

echo ""
echo "All tests passed!"
