# Directory Cache Rollout Decision

Decision date: 2026-07-13

## Decision

The directory cache remains an experimental, opt-in feature. It is not
default-on and the existing CLI surface remains unchanged:

- `--cache` enables the fast metadata-validation mode explicitly.
- `--cache-verify` is the recommended mode whenever the count must remain
  exact under timestamp-preserving rewrites.
- `--no-cache` remains the explicit cold oracle and bypasses cache state.
- No migration or deprecation is required because the manifest is an internal
  versioned cache and incompatible state safely rebuilds from cold.

Metadata validation is acceptable only as a documented filesystem-metadata
assumption. It is not an unconditional exactness guarantee. The product must
not silently change `--cache` to default-on until ownership explicitly accepts
that trade-off or the verified path becomes the default policy.

## Backend decision

Retain the versioned standard-library binary manifest backend for v1. It passed
the persistence threshold and operational tests:

- Medium decode + merge + atomic-write median: 50.214 ms.
- Medium manifest size: 6,360,042 bytes for 20,000 entries.
- Large manifest benchmark: 100,000 entries, 31,800,042 bytes, with bounded
  decode allocation and atomic write measurements.
- Containerized corruption, incompatible-schema, interruption, concurrency,
  permission, and clear/write race tests pass.
- Review remediation bounds encoded and loaded manifests at 256 MiB and keeps
  verified validation from retaining the full corpus in memory.

The backend remains internal. A future schema change must use an explicit
version or cold rebuild; users should use `cache status` and `cache clear`
rather than depending on manifest bytes.

## Evidence and limits

All acceptance gates passed with documented evidence in the Festival results:

- Cached and forced-cold mutation results match in the container oracle.
- Complete hits make zero tokenizer calls; one-file edits recount only the
  changed file.
- Medium metadata population stayed within the 10% overhead threshold and
  metadata warm runs were approximately 245x faster than the uncached median.
- Verified performance, CPU/RSS, bytes read, and manifest costs are recorded.
- Full 2k/20k/100k validation and manifest tiers passed.

The following are intentionally not release blockers but remain outside the
current capacity evidence: full 100k cache tokenization, medium all-method
cache population, Windows writer-lock persistence, and parent-directory fsync
power-loss durability. They are tracked as campaign intents:

- `.campaign/intents/inbox/evaluate-verified-by-default-cache-20260713-172606.md`
- `.campaign/intents/inbox/run-full-100k-file-and-20260713-172613.md`
- `.campaign/intents/inbox/harden-cross-platform-cache-durability-20260713-172617.md`

## Reconsideration criteria

Revisit default-on behavior only after a product owner approves the metadata
exactness trade-off or verified validation is made the default, and after the
tracked capacity/platform/durability evidence is complete. Any such change
must rerun the CLI compatibility and full quality gates and update this
decision before changing defaults.
