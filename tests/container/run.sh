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
  run.sh cache [--tiers small,medium] [--samples 3] [--repo /mounted/path] [--model-only]
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
        instrumentation=$(sed -n 's/^Cache diagnostics: //p' "$output")
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

run_timed_cache_count() {
    local tier="$1"
    local root="$2"
    local mode="$3"
    local validation="$4"
    local cache_root="$5"
    local phase="$6"
    local sample="$7"
    local output
    local elapsed
    local instrumentation
    local -a args

    output=$(mktemp)
    args=(--no-color --verbose --directory --cache)
    if [[ "$mode" == "model" ]]; then
        args+=(--model gpt-5)
    else
        args+=(--all)
    fi
    if [[ "$validation" == "verified" ]]; then
        args+=(--cache-verify)
    fi
    env TCOUNT_CACHE_DIR="$cache_root" /usr/bin/time -f 'elapsed_seconds=%e user_seconds=%U sys_seconds=%S peak_rss_kb=%M' \
        "$TCOUNT" "${args[@]}" "$root" > /dev/null 2> "$output"
    elapsed=$(sed -n 's/^elapsed_seconds=\([^ ]*\).*/\1/p' "$output")
    instrumentation=$(sed -n 's/^Cache diagnostics: //p' "$output")
    printf 'cache tier=%s phase=%s validation=%s mode=%s sample=%s elapsed_seconds=%s %s %s\n' \
        "$tier" "$phase" "$validation" "$mode" "$sample" "$elapsed" \
        "$(sed -n 's/^elapsed_seconds=[^ ]* //p' "$output")" "$instrumentation"
    printf '%s\n' "$elapsed"
    rm -f "$output"
}

cache_manifest_bytes() {
    local cache_root="$1"
    find "$cache_root" -type f -name manifest -printf '%s\n' 2>/dev/null | awk '{total += $1} END {print total + 0}'
}

run_cache_scenario() {
    local tier="$1"
    local root="$2"
    local mode="$3"
    local validation="$4"
    local samples="$5"
    local workspace="$6"
    local population_values
    local warm_values
    local sample
    local cache_root
    local warm_cache
    local elapsed

    population_values=$(mktemp)
    warm_values=$(mktemp)
    for sample in $(seq 1 "$samples"); do
        cache_root="$workspace/cache-$tier-$validation-$mode-population-$sample"
        elapsed=$(run_timed_cache_count "$tier" "$root" "$mode" "$validation" "$cache_root" population "$sample" | tee /dev/stderr | tail -n 1)
        printf '%s\n' "$elapsed" >> "$population_values"
        printf 'cache manifest tier=%s phase=population validation=%s mode=%s sample=%s bytes=%s\n' \
            "$tier" "$validation" "$mode" "$sample" "$(cache_manifest_bytes "$cache_root")"
    done

    warm_cache="$workspace/cache-$tier-$validation-$mode-warm"
    run_timed_cache_count "$tier" "$root" "$mode" "$validation" "$warm_cache" warm-seed 0 >/dev/null
    for sample in $(seq 1 "$samples"); do
        elapsed=$(run_timed_cache_count "$tier" "$root" "$mode" "$validation" "$warm_cache" warm "$sample" | tee /dev/stderr | tail -n 1)
        printf '%s\n' "$elapsed" >> "$warm_values"
    done
    printf 'cache summary tier=%s phase=population validation=%s mode=%s median_elapsed_seconds=%s p95_elapsed_seconds=%s\n' \
        "$tier" "$validation" "$mode" "$(median_value "$population_values" "$samples")" "$(p95_value "$population_values" "$samples")"
    printf 'cache summary tier=%s phase=warm validation=%s mode=%s median_elapsed_seconds=%s p95_elapsed_seconds=%s manifest_bytes=%s\n' \
        "$tier" "$validation" "$mode" "$(median_value "$warm_values" "$samples")" "$(p95_value "$warm_values" "$samples")" "$(cache_manifest_bytes "$warm_cache")"
    rm -f "$population_values" "$warm_values"
}

run_cache_mutations() {
    local workspace="$1"
    local samples="$2"
    local scenario
    local sample
    local root
    local cache_root
    local values
    local elapsed

    for scenario in edit add delete rename; do
        values=$(mktemp)
        for sample in $(seq 1 "$samples"); do
            root="$workspace/mutation-$scenario-$sample"
            cache_root="$workspace/mutation-cache-$scenario-$sample"
            generate_fixture "mutation-$scenario" 2000 "$SMALL_BYTES" "$root" >/dev/null
            env TCOUNT_CACHE_DIR="$cache_root" "$TCOUNT" --no-color --directory --cache --model gpt-5 "$root" >/dev/null
            case "$scenario" in
                edit) printf 'changed content\n' > "$root/file-000001.txt" ;;
                add) cp "$root/file-000001.txt" "$root/file-added.txt" ;;
                delete) rm -f "$root/file-000001.txt" ;;
                rename) mv "$root/file-000001.txt" "$root/file-renamed.txt" ;;
            esac
            elapsed=$(run_timed_cache_count small "$root" model metadata "$cache_root" "mutation-$scenario" "$sample" | tee /dev/stderr | tail -n 1)
            printf '%s\n' "$elapsed" >> "$values"
        done
        printf 'cache mutation summary tier=small scenario=%s median_elapsed_seconds=%s p95_elapsed_seconds=%s\n' \
            "$scenario" "$(median_value "$values" "$samples")" "$(p95_value "$values" "$samples")"
        rm -f "$values"
    done
}

run_cache_benchmarks() {
    local tiers="$1"
    local samples="$2"
    local repo="$3"
    local model_only="$4"
    local workspace
    local tier
    local params
    local files
    local bytes
    local root
    local validation

    workspace=$(mktemp -d /tmp/tcount-cache-benchmark.XXXXXX)
    if [[ "$tiers" == "all" ]]; then
        tiers="small,medium,large"
    fi
    IFS=',' read -r -a requested_tiers <<< "$tiers"
    for tier in "${requested_tiers[@]}"; do
        if [[ -n "$repo" ]]; then
            root="$repo"
            printf 'cache fixture=%s source=mounted-repository path=%s content=not-recorded\n' "$tier" "$root"
        else
            params=$(fixture_parameters "$tier")
            read -r files bytes <<< "$params"
            root="$workspace/$tier"
            generate_fixture "$tier" "$files" "$bytes" "$root"
        fi
        run_cache_scenario "$tier" "$root" model metadata "$samples" "$workspace"
        run_cache_scenario "$tier" "$root" model verified "$samples" "$workspace"
        if [[ "$model_only" != "true" ]]; then
            run_cache_scenario "$tier" "$root" all metadata "$samples" "$workspace"
            run_cache_scenario "$tier" "$root" all verified "$samples" "$workspace"
        fi
        if [[ "$tier" == "small" && -z "$repo" ]]; then
            run_cache_mutations "$workspace" "$samples"
        fi
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
        cache)
            local tiers="small,medium"
            local samples="3"
            local repo=""
            local model_only="false"
            while [[ $# -gt 0 ]]; do
                case "$1" in
                    --tiers) tiers="$2"; shift 2 ;;
                    --samples) samples="$2"; shift 2 ;;
                    --repo) repo="$2"; shift 2 ;;
                    --model-only) model_only="true"; shift ;;
                    -h|--help) usage; return 0 ;;
                    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; return 2 ;;
                esac
            done
            run_cache_benchmarks "$tiers" "$samples" "$repo" "$model_only"
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
