#!/bin/bash
# Pre-tool-use hook — enforce Go coding standards for dune-admin

set -e

INPUT="$1"
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool // empty')
FILE_PATH=$(echo "$INPUT" | jq -r '.parameters.file_path // empty')
COMMAND=$(echo "$INPUT" | jq -r '.parameters.command // empty')

is_go_impl_file() {
    [[ "$1" == *.go ]] && [[ "$1" != *_test.go ]]
}

is_go_test_file() {
    [[ "$1" == *_test.go ]]
}

test_file_exists() {
    local test_file="${1%.go}_test.go"
    [[ -f "$test_file" ]]
}

CONTEXT_PARTS=()

# ==============================================================================
# Write/Edit: TDD compliance + flat-package guard
# ==============================================================================
if [[ "$TOOL_NAME" == "Write" ]] || [[ "$TOOL_NAME" == "Edit" ]]; then
    if is_go_impl_file "$FILE_PATH"; then
        # Guard against creating sub-packages
        if [[ "$FILE_PATH" == */internal/* ]] || [[ "$FILE_PATH" =~ cmd/dune-admin/[^/]+/[^/]+\.go ]]; then
            CONTEXT_PARTS+=("ARCHITECTURE VIOLATION: dune-admin is a flat package main — do NOT create sub-packages. All Go files belong directly in cmd/dune-admin/. See .claude/rules/architecture.md.")
        fi

        if [[ ! -f "$FILE_PATH" ]]; then
            CONTEXT_PARTS+=("TDD REMINDER: Creating a NEW implementation file. Have you written the test FIRST? Expected test file: ${FILE_PATH%.go}_test.go. If it doesn't exist yet, stop and write it first.")
        else
            if ! test_file_exists "$FILE_PATH"; then
                CONTEXT_PARTS+=("MISSING TESTS: Modifying implementation without a test file. Expected: ${FILE_PATH%.go}_test.go. Write tests FIRST.")
            fi
        fi

        # Remind about SQL placement
        if [[ "$FILE_PATH" == *handlers_*.go ]]; then
            CONTEXT_PARTS+=("HANDLER REMINDER: Handlers call cmd functions from db.go — no raw SQL in handlers. Use jsonOK/jsonErr/decode from server.go. Guard globalDB == nil before querying. See .claude/rules/api-design.md.")
        fi
    fi

    if is_go_test_file "$FILE_PATH"; then
        CONTEXT_PARTS+=("Writing tests — excellent! Use table-driven tests. Mock all external dependencies (DB, executor, control plane). Test all error paths. See .claude/rules/testing.md.")
    fi
fi

# ==============================================================================
# Bash: test command reminders
# ==============================================================================
if [[ "$TOOL_NAME" == "Bash" ]]; then
    if [[ "$COMMAND" == *"go test"* ]] && [[ "$COMMAND" != *"-race"* ]]; then
        CONTEXT_PARTS+=("TESTING REMINDER: Use the race detector — prefer: make test-race (or go test -race ./...).")
    fi

    if [[ "$COMMAND" == *"go build"* ]] || [[ "$COMMAND" == *"go run"* ]]; then
        CONTEXT_PARTS+=("TIP: Run tests before building — make test-race.")
    fi

    if [[ "$COMMAND" == *"npm "* ]] && [[ "$FILE_PATH" == *web* || "$COMMAND" == *web* ]]; then
        CONTEXT_PARTS+=("FRONTEND REMINDER: web/ uses pnpm (pinned). Use pnpm instead of npm.")
    fi
fi

# ==============================================================================
# Output JSON if any context to add
# ==============================================================================
if [[ ${#CONTEXT_PARTS[@]} -gt 0 ]]; then
    CONTEXT=""
    for part in "${CONTEXT_PARTS[@]}"; do
        if [[ -n "$CONTEXT" ]]; then
            CONTEXT="$CONTEXT | $part"
        else
            CONTEXT="$part"
        fi
    done

    jq -n \
        --arg context "$CONTEXT" \
        '{
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "additionalContext": $context
            }
        }'
fi

exit 0
