#!/bin/bash
# Extract total coverage percentage from go coverage output
# Usage: ./get-coverage.sh coverage.out

COVERAGE_FILE="${1:-coverage.out}"

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "Error: Coverage file not found: $COVERAGE_FILE" >&2
    exit 1
fi

# Extract total coverage percentage (removes % sign)
COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep total | awk '{print $3}' | sed 's/%//')

if [ -z "$COVERAGE" ]; then
    echo "Error: Failed to extract coverage from $COVERAGE_FILE" >&2
    exit 1
fi

echo "$COVERAGE"
