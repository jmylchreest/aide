#!/bin/bash
# Sample bash script for grammar testing.

set -euo pipefail

# A simple greeting function.
function greet() {
    local name=$1
    echo "Hello, $name!"
}

# Compute factorial iteratively.
function factorial() {
    local n=$1
    local result=1
    for ((i = 2; i <= n; i++)); do
        result=$((result * i))
    done
    echo "$result"
}

# Entry point.
function main() {
    greet "world"
    echo "5! = $(factorial 5)"
}

main "$@"
