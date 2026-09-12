set working-directory := '..'

BUILDTOOL := "go run ./internal/buildutil"

# tcount's integration tests are host `go test` runs selected by name. The
# build-util `integration` command instead cross-compiles a Linux binary and
# runs Docker suites under tests/integration, so this lane calls `go test`
# directly and `all` is composed here rather than delegated to `buildutil all`.
INTEGRATION_ARGS := "-run Integration -timeout 60s ./..."

# Show available test recipes
[private]
@default:
    just --list --justfile {{ source_file() }}

# Run all tests (clean, build, unit, integration)
all:
    #!/usr/bin/env bash
    set -uo pipefail
    failures=0
    printf '\n🧹 Cleaning...\n'
    {{ BUILDTOOL }} clean || failures=$((failures + 1))
    printf '\n🔨 Building...\n'
    if ! {{ BUILDTOOL }} build; then
        printf '\nstopping due to build failure\n' >&2
        exit 1
    fi
    printf '\n🧪 Testing...\n'
    {{ BUILDTOOL }} test || failures=$((failures + 1))
    printf '\n🔗 Integration Testing...\n'
    go test {{ INTEGRATION_ARGS }} || failures=$((failures + 1))
    if [ "$failures" -gt 0 ]; then
        printf '\n✗ %d tasks failed\n' "$failures" >&2
        exit 1
    fi
    printf '\n✓ All tasks complete\n'

# Run unit tests (with dashboard)
unit:
    @{{ BUILDTOOL }} test

# Run integration tests on the host (no Docker; see INTEGRATION_ARGS note above)
integration:
    go test {{ INTEGRATION_ARGS }}

# Run tests with coverage
coverage:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

# Run tests with coverage HTML report
coverage-html:
    go test ./... -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
bench:
    go test ./... -bench=. -benchmem

container-image:
    docker build --file tests/container/Dockerfile --tag tcount-test:local .

# Run all tests in the filesystem-isolated container harness.
container filter="": container-image
    docker run --rm tcount-test:local test "{{filter}}"

# Run deterministic directory benchmarks. Use `just test cache-bench all 5` to include the 1 GiB tier.
cache-bench tiers="small,medium" samples="5": container-image
    docker run --rm tcount-test:local bench --tiers "{{tiers}}" --samples "{{samples}}"

# Benchmark integrated cache population, warm reuse, and mutation work.
cache-path tiers="small,medium" samples="3": container-image
    docker run --rm tcount-test:local cache --tiers "{{tiers}}" --samples "{{samples}}"

# Benchmark only the one-model cache path for resource-heavy tiers.
cache-path-model tiers="large" samples="1": container-image
    docker run --rm tcount-test:local cache --tiers "{{tiers}}" --samples "{{samples}}" --model-only

# Compare metadata and verified validation without rerunning tokenization.
cache-validate tiers="small,medium" samples="5": container-image
    docker run --rm tcount-test:local validation --tiers "{{tiers}}" --samples "{{samples}}"

# Benchmark bounded manifest encode/decode/merge/atomic-write at all tiers.
manifest-bench tiers="small,medium,large" samples="3": container-image
    docker run --rm tcount-test:local manifest --tiers "{{tiers}}" --samples "{{samples}}"
