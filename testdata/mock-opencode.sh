#!/bin/bash
set -euo pipefail

RESULT="${MOCK_OPENDCODE_RESULT:-COMPLETE}"
EXIT_CODE="${MOCK_OPENDCODE_EXIT_CODE:-0}"

# Echo stdin back so tests can verify composed context
cat

echo "__LOOP_RESULT__"
echo "$RESULT"
echo "__LOOP_RESULT_END__"

exit "$EXIT_CODE"
