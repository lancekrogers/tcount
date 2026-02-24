set dotenv-load := false

mod release '.justfiles/build.just'
mod test '.justfiles/test.just'

binary := "tcount"
bin_dir := "bin"
BUILDTOOL := "go run ./internal/buildutil"

# Show available recipes
[private]
@default:
    just --list

# Build the binary (with dashboard)
build:
    @{{BUILDTOOL}} build

# Build only (no vet, with dashboard)
build-only:
    @{{BUILDTOOL}} build-only

# Format code
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Run linter
lint:
    golangci-lint run ./...

# Clean build artifacts (with dashboard)
clean:
    @{{BUILDTOOL}} clean

# Download dependencies
deps:
    go mod tidy
    go mod download

# Re-download embedded BPE vocab files from OpenAI
download-vocab:
    #!/usr/bin/env bash
    set -euo pipefail
    dir="tokenizer/bpe/vocabdata"
    base="https://openaipublic.blob.core.windows.net/encodings"
    for name in o200k_base cl100k_base p50k_base r50k_base; do
        echo "Downloading ${name}..."
        curl -sS -o "${dir}/${name}.tiktoken" "${base}/${name}.tiktoken"
    done
    echo "Done. $(ls -lh ${dir}/*.tiktoken | wc -l) vocab files updated."

# Install binary to GOPATH/bin
install: build
    go install ./cmd/tcount

# Uninstall binary
uninstall:
    rm -f $(go env GOPATH)/bin/{{binary}}

# Create a new release (bumps patch by default). Usage: just tag [version]
tag version="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -n "{{version}}" ]; then
        next="{{version}}"
    else
        latest=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
        # Bump patch: v0.1.2 -> v0.1.3
        IFS='.' read -r major minor patch <<< "${latest#v}"
        next="v${major}.${minor}.$((patch + 1))"
    fi
    echo ""
    echo "  Current: $(git describe --tags --always 2>/dev/null || echo 'no tags')"
    echo "  Next:    ${next}"
    echo ""
    echo "This will:"
    echo "  1. Tag commit $(git rev-parse --short HEAD) as ${next}"
    echo "  2. Push the tag to origin"
    echo "  3. Trigger the release workflow (build + homebrew update)"
    echo ""
    read -p "Proceed? [y/N] " confirm
    if [[ "${confirm}" =~ ^[Yy]$ ]]; then
        git tag "${next}"
        git push origin "${next}"
        echo ""
        echo "Tagged and pushed ${next}!"
        echo "Watch the release: https://github.com/lancekrogers/tcount/actions"
    else
        echo "Aborted."
    fi

# Push HOMEBREW_TAP_TOKEN from .env to GitHub Actions secrets
setup-secrets:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f .env ]; then
        echo "No .env file found. Copy .env.example and fill in your token."
        exit 1
    fi
    source .env
    if [ "${HOMEBREW_TAP_TOKEN}" = "your-token-here" ] || [ -z "${HOMEBREW_TAP_TOKEN}" ]; then
        echo "Set HOMEBREW_TAP_TOKEN in .env first (see comments for instructions)."
        exit 1
    fi
    echo "${HOMEBREW_TAP_TOKEN}" | gh secret set HOMEBREW_TAP_TOKEN --repo lancekrogers/tcount
    echo "Secret HOMEBREW_TAP_TOKEN set successfully."
