# Directory Cache Operations

The directory cache is an experimental, opt-in optimization for recursive
counts. It is not used by default, by single-file counts, or by `--no-cache`.

## Location and stored data

The default parent is the platform user-cache directory. tcount stores state
under:

```text
<user-cache>/tcount/v1/roots/
```

Set `TCOUNT_CACHE_DIR` to replace `<user-cache>`; tcount still appends the
versioned `tcount/v1` namespace. Each canonical counted root gets its own
versioned manifest and lock files.

The manifest stores the canonical root, relative file paths, file size and
nanosecond modification time, text classification, aggregate character/word/
line totals, tokenizer contract identifiers, token counts, and SHA-256 content
digests for populated entries. Verified mode recomputes and compares those
digests before reuse; metadata mode stores the digest but does not recompute it
on a hit. It does not store source file contents. The canonical root, relative
paths, metadata, and token counts may still reveal sensitive project
information, so use an appropriate cache parent and clear it when required.

Cached writes and `cache clear --all` share a user-wide lifecycle lock in this
experimental v1 implementation. That keeps root creation, generation commits,
membership pruning, and clearing coordinated, but serializes cached writes for
different roots. Revisit the lock scope if concurrent multi-repository
workloads become a supported performance target.

## Validation and exactness

```bash
tcount -d --cache ./src                 # metadata validation
tcount -d --cache --cache-verify ./src  # content-digest validation
```

The default metadata mode reuses an entry when path, size, and nanosecond
modification time match. It is fast and avoids content reads, but it is not
content-exact under a timestamp-preserving same-size rewrite. Verified mode
reads and hashes current bytes before reuse and is the required mode when that
exactness guarantee matters. Directory membership always comes from the
current successful walk; the manifest cannot add stale files back to a count.

## Inspect, clear, and bypass

```bash
tcount cache status ./src
tcount cache status --json ./src
tcount cache clear ./src
tcount cache clear --all
tcount -d --no-cache ./src
```

`cache status` is read-only and reports the canonical root, presence, schema,
entry count, manifest bytes, generation, age, and modification time. Its JSON
form is intended for scripts. `cache clear --all` is explicit and
noninteractive; it cannot be combined with a path. Both clear forms use the
same canonicalization and locking boundary as counting and never delete
outside the tcount cache namespace. Clearing an empty namespace is an
idempotent success.

`--no-cache` is a true cold bypass: it does not construct, read, or write a
cache store, even when `TCOUNT_CACHE_DIR` points at an invalid location.

## Fallback and diagnostics

Cache health must not change a correctly computed count. During a cached count,
load, validation, quarantine, or persistence failures fall back to the cold
count path; `--verbose` emits one concise diagnostic summary on stderr with the
validation mode, hits, partial hits, misses, incompatibilities, cache bytes
reused, bytes read, tokenizer calls, warnings, invalidation reasons, and stage
timings. Normal text output and JSON count output remain on stdout and keep
their existing shape.

Explicit management commands have different semantics: `cache status` and
`cache clear` return operation failures as errors rather than silently falling
back. This makes scripts able to distinguish an absent cache from a management
failure.

## Experimental status

The cache remains experimental. Metadata validation is intentionally not
content-exact, the manifest format is versioned and internal, and the feature
is not enabled by default. Use `--cache-verify` when correctness is more
important than avoiding validation reads, and treat `cache status` as the
supported way to inspect state rather than depending on manifest internals.

See [the rollout decision](rollout-decision.md) for the accepted backend,
validation, default-behavior, and follow-up decisions.
