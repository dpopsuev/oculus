#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/../lib/entity.sh"

main() {
    local entity
    entity=$(entity_new "1" "test")
    echo "Entity: ${entity}"
}

main "$@"
