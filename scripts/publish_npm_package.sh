#!/usr/bin/env bash
# Publish the npm wrapper package for an existing tcount GitHub release.

set -euo pipefail

PACKAGE_DIR="${NPM_PACKAGE_DIR:-npm}"
PACKAGE_NAME="${NPM_PACKAGE_NAME:-@obedience-corp/tcount}"
RELEASE_MODE="${RELEASE_MODE:-stable}"

if [ -n "${NPM_PACKAGE_VERSION:-}" ]; then
    VERSION="$NPM_PACKAGE_VERSION"
elif [ -n "${GITHUB_REF_NAME:-}" ]; then
    VERSION="${GITHUB_REF_NAME#v}"
else
    echo "::error::NPM_PACKAGE_VERSION or GITHUB_REF_NAME is required" >&2
    exit 1
fi

case "$RELEASE_MODE" in
    stable) NPM_DIST_TAG="${NPM_DIST_TAG:-latest}" ;;
    rc) NPM_DIST_TAG="${NPM_DIST_TAG:-rc}" ;;
    dev) NPM_DIST_TAG="${NPM_DIST_TAG:-dev}" ;;
    *)
        echo "::error::Unsupported release mode ${RELEASE_MODE}" >&2
        exit 1
        ;;
esac

if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    echo "::error::Invalid npm package version: ${VERSION}" >&2
    exit 1
fi

if [ ! -d "$PACKAGE_DIR" ]; then
    echo "::error::npm package directory not found: ${PACKAGE_DIR}" >&2
    exit 1
fi

echo "Node: $(node --version)"
echo "npm: $(npm --version)"
echo "Package: ${PACKAGE_NAME}@${VERSION}"
echo "Dist tag: ${NPM_DIST_TAG}"
echo "Package directory: ${PACKAGE_DIR}"

NPM_MIN_VERSION="11.5.1"
NODE_MIN_VERSION="22.14.0"

version_at_least() {
    # version_at_least <minimum> <actual>
    [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" = "$1" ]
}

# CI publishes through npm Trusted Publishing (OIDC). actions/setup-node with
# registry-url exports NODE_AUTH_TOKEN=XXXXX and writes _authToken into an
# .npmrc it points NPM_CONFIG_USERCONFIG at; npm authenticates with that dummy
# token and never attempts the OIDC exchange (E404 on scoped packages). Drop
# both so OIDC runs. Only in CI: a local run may carry a real token on purpose,
# and its NPM_CONFIG_USERCONFIG is the developer's own file.
if [ "${CI:-}" = "true" ]; then
    if [ -n "${NODE_AUTH_TOKEN:-}" ]; then
        echo "Unsetting NODE_AUTH_TOKEN so npm can use GitHub OIDC trusted publishing"
        unset NODE_AUTH_TOKEN
    fi
    if [ -n "${NPM_CONFIG_USERCONFIG:-}" ]; then
        echo "Unsetting NPM_CONFIG_USERCONFIG (${NPM_CONFIG_USERCONFIG}) so npm does not read a setup-node .npmrc"
        unset NPM_CONFIG_USERCONFIG
    fi

    node_version="$(node --version)"
    node_version="${node_version#v}"
    npm_version="$(npm --version)"
    if ! version_at_least "$NODE_MIN_VERSION" "$node_version"; then
        echo "::error::Node ${node_version} is below ${NODE_MIN_VERSION}, the minimum for npm trusted publishing. Raise node-version in release.yml." >&2
        exit 1
    fi
    if ! version_at_least "$NPM_MIN_VERSION" "$npm_version"; then
        echo "::error::npm ${npm_version} is below ${NPM_MIN_VERSION}, the minimum for npm trusted publishing. Pin npm in release.yml (npm install -g npm@^${NPM_MIN_VERSION})." >&2
        exit 1
    fi
fi

can_publish_interactively() {
    [ "${NPM_PUBLISH_INTERACTIVE:-auto}" != "never" ] &&
        [ "${CI:-}" != "true" ] &&
        [ -r /dev/tty ] &&
        [ -w /dev/tty ]
}

if [ "${NPM_SKIP_EXISTING_CHECK:-false}" != "true" ] && npm view "${PACKAGE_NAME}@${VERSION}" version >/dev/null 2>&1; then
    echo "${PACKAGE_NAME}@${VERSION} is already published; leaving registry unchanged."
    exit 0
fi

pushd "$PACKAGE_DIR" >/dev/null
npm version "$VERSION" --no-git-tag-version --allow-same-version

set +e
publish_args=(publish --access public --tag "$NPM_DIST_TAG")
if [ "${NPM_PUBLISH_DRY_RUN:-false}" = "true" ]; then
    publish_args+=(--dry-run)
fi

if can_publish_interactively; then
    npm "${publish_args[@]}"
    popd >/dev/null
    exit 0
fi

publish_output="$(npm "${publish_args[@]}" 2>&1)"
publish_status=$?
set -e
printf '%s\n' "$publish_output"

if [ "$publish_status" -ne 0 ]; then
    if printf '%s\n' "$publish_output" | grep -q 'EOTP'; then
        echo "::error::npm publish requires OTP. Re-run from an interactive terminal so npm can prompt for your configured verification method, or use GitHub Actions npm Trusted Publishing (OIDC) instead of a long-lived token." >&2
    elif printf '%s\n' "$publish_output" | grep -q 'E404'; then
        echo "::error::npm publish returned 404 (token cannot publish this package, or OIDC did not run). In CI, grant id-token: write, leave NODE_AUTH_TOKEN unset, and register workflow filename release.yml as a trusted publisher for ${PACKAGE_NAME}." >&2
    fi
    exit "$publish_status"
fi

popd >/dev/null
