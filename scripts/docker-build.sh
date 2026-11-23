#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

namespace=${DOCKERHUB_NAMESPACE:-nmisha}

dry_run=false
versions=()
services=()
while (($#)); do
    case "$1" in
        --dry-run) dry_run=true; shift; continue;;
        -v|--version) versions+=("${2:?Missing version}");;
        -s|--services) services+=("${2:?Missing service}");;
        *) echo "Unknown argument: $1" >&2; exit 1;;
    esac
    shift 2
done

if [[ -z "${versions[0]:-}" ]]; then
    versions=(latest)
fi
manifest_args=(versions.json --args --)
if [[ -n "${services[0]:-}" ]]; then
    manifest_args+=("${services[@]}")
fi
images=$(jq -er '
    if $ARGS.positional == [] then .images[] else
        INDEX(.images[]; .name) as $images |
        $ARGS.positional[] | $images[.] // error("Unknown service: " + .)
    end | [.source,.compose,.target,.repository] | @tsv
' "${manifest_args[@]}" | tr -d '\r')

while IFS=$'\t' read -r source compose target repository; do
    tags=()
    for version in "${versions[@]}"; do
        tags+=(--set "$target.tags=$namespace/$repository:$version")
    done
    if [[ "$dry_run" == true ]]; then
        printf 'cd %q && ' "$source"
        printf '%q ' docker buildx bake -f "$compose" "${tags[@]}" --push "$target"
        printf '\n'
    else
        (cd "$source" && docker buildx bake -f "$compose" "${tags[@]}" --push "$target")
    fi
done <<< "$images"
