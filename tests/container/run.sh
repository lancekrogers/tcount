#!/usr/bin/env bash

set -euo pipefail

readonly TCOUNT=/workspace/bin/tcount
readonly VALIDATION_BENCH=/workspace/bin/cache-validation-bench
readonly MANIFEST_BENCH=/workspace/bin/cache-manifest-bench
readonly DEFAULT_SAMPLES=5
readonly SMALL_FILES=2000
readonly SMALL_BYTES=$((8 * 1024 * 1024))
readonly MEDIUM_FILES=20000
readonly MEDIUM_BYTES=$((100 * 1024 * 1024))
readonly LARGE_FILES=100000
readonly LARGE_BYTES=$((1024 * 1024 * 1024))

usage() {
    cat <<'EOF'
Usage:
  run.sh test [go-test-regexp]
  run.sh bench [--tiers small,medium] [--samples 5] [--repo /mounted/path]
  run.sh validation [--tiers small,medium] [--samples 5]
  run.sh manifest [--tiers small,medium,large] [--samples 3]

The large tier is opt-in: use --tiers all or include large explicitly.
EOF
}

generate_fixture() {
    local name="$1"
    local files="$2"
    local target_bytes="$3"
    local root="$4"
    local per_file=$((target_bytes / files))
    local template="$root/.template"

    mkdir -p "$root"
    head -c "$per_file" /dev/zero | tr '\0' 'a' > "$template"
    for ((index = 1; index <= files; index++)); do
        cp "$template" "$root/file-$(printf '%06d' "$index").txt"
    done
    rm -f "$template"
    printf 'fixture=%s files=%d bytes=%d per_file=%d\n' "$name" "$files" "$((per_file * files))" "$per_file"
}

fixture_parameters() {
    case "$1" in
        small) printf '%s\n' "$SMALL_FILES $SMALL_BYTES" ;;
        medium) printf '%s\n' "$MEDIUM_FILES $MEDIUM_BYTES" ;;
        large) printf '%s\n' "$LARGE_FILES $LARGE_BYTES" ;;
        *) printf 'unknown benchmark tier: %s\n' "$1" >&2; return 1 ;;
    esac
}

median_value() {
    local values="$1"
    local samples="$2"
    sort -n "$values" | awk -v samples="$samples" 'NR == int((samples + 1) / 2) { print; exit }'
}

p95_value() {
    local values="$1"
    local samples="$2"
    local rank=$(((95 * samples + 99) / 100))
    sort -n "$values" | awk -v rank="$rank" 'NR == rank { print; exit }'
}

run_samples() {
    local tier="$1"
    local root="$2"
    local mode="$3"
    local samples="$4"
    local values
    local sample
    local output
    local elapsed
    local instrumentation

    values=$(mktemp)

    printf 'benchmark tier=%s mode=%s filesystem=container-generated page_cache=warm-process-cold application_cache=none samples=%d\n' "$tier" "$mode" "$samples"
    for sample in $(seq 1 "$samples"); do
        output=$(mktemp)
        if [[ "$mode" == "model" ]]; then
            /usr/bin/time -f 'elapsed_seconds=%e user_seconds=%U sys_seconds=%S peak_rss_kb=%M' \
                "$TCOUNT" --no-color --verbose --directory --model gpt-5 "$root" \
                > /dev/null 2> "$output"
        else
            /usr/bin/time -f 'elapsed_seconds=%e user_seconds=%U sys_seconds=%S peak_rss_kb=%M' \
                "$TCOUNT" --no-color --verbose --directory --all "$root" \
                > /dev/null 2> "$output"
        fi
        elapsed=$(sed -n 's/^elapsed_seconds=\([^ ]*\).*/\1/p' "$output")
        instrumentation=$(sed -n 's/^Instrumentation: //p' "$output")
        printf '%s\n' "$elapsed" >> "$values"
        printf 'sample=%d elapsed_seconds=%s %s %s\n' "$sample" "$elapsed" \
            "$(sed -n 's/^elapsed_seconds=[^ ]* //p' "$output")" "$instrumentation"
        rm -f "$output"
    done
    printf 'summary tier=%s mode=%s median_elapsed_seconds=%s p95_elapsed_seconds=%s\n' \
        "$tier" "$mode" "$(median_value "$values" "$samples")" "$(p95_value "$values" "$samples")"
    rm -f "$values"
}

run_validation_samples() {
    local tier="$1"
    local root="$2"
    local mode="$3"
    local samples="$4"
    local output
    local timing

    output=$(mktemp)
    timing=$(mktemp)
    /usr/bin/time -f 'user_seconds=%U sys_seconds=%S peak_rss_kb=%M' \
        "$VALIDATION_BENCH" --root "$root" --mode "$mode" --samples "$samples" \
        > "$output" 2> "$timing"
    sed "s/^/validation tier=$tier /" "$output"
    printf 'validation resources tier=%s mode=%s %s\n' "$tier" "$mode" "$(cat "$timing")"
    rm -f "$output" "$timing"
}

run_validation_benchmarks() {
    local tiers="$1"
    local samples="$2"
    local workspace
    local tier
    local params
    local files
    local bytes
    local root

    workspace=$(mktemp -d /tmp/tcount-validation.XXXXXX)
    if [[ "$tiers" == "all" ]]; then
        tiers="small,medium,large"
    fi
    IFS=',' read -r -a requested_tiers <<< "$tiers"
    for tier in "${requested_tiers[@]}"; do
        params=$(fixture_parameters "$tier")
        read -r files bytes <<< "$params"
        root="$workspace/$tier"
        generate_fixture "$tier" "$files" "$bytes" "$root"
        run_validation_samples "$tier" "$root" metadata "$samples"
        run_validation_samples "$tier" "$root" verified "$samples"
    done
    rm -rf "$workspace"
}

run_manifest_benchmarks() {
    local tiers="$1"
    local samples="$2"
    local tier
    local params
    local entries
    local ignored_bytes

    if [[ "$tiers" == "all" ]]; then
        tiers="small,medium,large"
    fi
    IFS=',' read -r -a requested_tiers <<< "$tiers"
    for tier in "${requested_tiers[@]}"; do
        params=$(fixture_parameters "$tier")
        read -r entries ignored_bytes <<< "$params"
        "$MANIFEST_BENCH" --entries "$entries" --samples "$samples" \
            | sed "s/^/manifest tier=$tier /"
    done
}

run_benchmarks() {
    local tiers="$1"
    local samples="$2"
    local repo="$3"
    local workspace
    local tier
    local params
    local files
    local bytes
    local root

    workspace=$(mktemp -d /tmp/tcount-benchmark.XXXXXX)

    if [[ "$tiers" == "all" ]]; then
        tiers="small,medium,large"
    fi
    IFS=',' read -r -a requested_tiers <<< "$tiers"
    for tier in "${requested_tiers[@]}"; do
        if [[ -n "$repo" ]]; then
            root="$repo"
            printf 'fixture=%s source=mounted-repository path=%s content=not-recorded\n' "$tier" "$root"
        else
            params=$(fixture_parameters "$tier")
            read -r files bytes <<< "$params"
            root="$workspace/$tier"
            generate_fixture "$tier" "$files" "$bytes" "$root"
        fi
        run_samples "$tier" "$root" model "$samples"
        run_samples "$tier" "$root" all "$samples"
        run_validation_samples "$tier" "$root" metadata "$samples"
        run_validation_samples "$tier" "$root" verified "$samples"
    done
    rm -rf "$workspace"
}

main() {
    local command="${1:-}"
    shift || true

    case "$command" in
        test)
            filter="${1:-}"
            if [[ -n "$filter" ]]; then
                exec go test -tags container ./... -run "(?i)$filter"
            fi
            exec go test -tags container ./...
            ;;
        bench)
            local tiers="small,medium"
            local samples="$DEFAULT_SAMPLES"
            local repo=""
            while [[ $# -gt 0 ]]; do
                case "$1" in
                    --tiers) tiers="$2"; shift 2 ;;
                    --samples) samples="$2"; shift 2 ;;
                    --repo) repo="$2"; shift 2 ;;
                    -h|--help) usage; return 0 ;;
                    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; return 2 ;;
                esac
            done
            run_benchmarks "$tiers" "$samples" "$repo"
            ;;
        validation)
            local tiers="small,medium"
            local samples="$DEFAULT_SAMPLES"
            while [[ $# -gt 0 ]]; do
                case "$1" in
                    --tiers) tiers="$2"; shift 2 ;;
                    --samples) samples="$2"; shift 2 ;;
                    -h|--help) usage; return 0 ;;
                    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; return 2 ;;
                esac
            done
            run_validation_benchmarks "$tiers" "$samples"
            ;;
        manifest)
            local tiers="small,medium,large"
            local samples="3"
            while [[ $# -gt 0 ]]; do
                case "$1" in
                    --tiers) tiers="$2"; shift 2 ;;
                    --samples) samples="$2"; shift 2 ;;
                    -h|--help) usage; return 0 ;;
                    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; return 2 ;;
                esac
            done
            run_manifest_benchmarks "$tiers" "$samples"
            ;;
        -h|--help) usage ;;
        *) usage >&2; return 2 ;;
    esac
}

main "$@"
