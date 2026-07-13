set working-directory := '..'

BUILDTOOL := "go run ./internal/buildutil"

# Show available test recipes
[private]
@default:
    just --list --justfile {{ source_file() }}

# Run all tests (unit + integration, with dashboard)
all:
    @{{ BUILDTOOL }} all

# Run unit tests (with dashboard)
unit:
    @{{ BUILDTOOL }} test

# Run integration tests (with dashboard)
integration:
    @{{ BUILDTOOL }} integration

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
