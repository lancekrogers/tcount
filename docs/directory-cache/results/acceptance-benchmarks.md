# Directory Cache Acceptance Benchmarks

Status: PASS

## Environment

- Date: 2026-07-13.
- Project revision: `84b2ffb` (`feat: complete cache CLI and operations`), with the acceptance-only harness changes in `tests/container/run.sh` and `.justfiles/test.just` present during these measurements.
- Toolchain: Go 1.25.6, Docker 28.4.0, `golang:1.25.6-bookworm`, arm64 host.
- Synthetic fixtures are generated and removed inside the non-root `tcounttest` container. No generated fixture is checked in.
- The harness reports warm-process/page-cache conditions; it does not drop host page caches. Each timed count is a new process. Cache population uses a fresh cache root; warm samples reuse one root.

## Commands

```text
just test container
just test cache-validate all 1
just test manifest-bench all 3
docker run --rm tcount-test:local cache --tiers small --samples 3 --model-only
docker run --rm tcount-test:local cache --tiers medium --samples 1 --model-only
docker run --rm -v "$PWD":/repo tcount-test:local bench --tiers medium --samples 3 --repo /repo
docker run --rm -v "$PWD":/repo tcount-test:local cache --tiers medium --samples 3 --repo /repo --model-only
```

The three-sample small all-method portion was run through `just test cache-path` and completed before the intentionally redundant long medium matrix was stopped. The medium one-model run above was repeated in isolation after that stop. The uncached synthetic reference is the approved three-sample Sequence 01 result from `just test cache-bench medium 3`.

## Result

### Count cache

The clean isolated medium one-model results are one sample, so median and p95 are equal. The uncached comparison is the approved three-sample baseline: 46.46s median / 57.04s p95, 173.76s median user CPU, 2.14s median system CPU, and 53.4 MiB median peak RSS.

| Fixture / mode | Phase | Median wall | p95 wall | User / sys CPU | Peak RSS | Tokenizer calls | Bytes read | Manifest |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| small / model / metadata | population | 3.06s | 3.06s | 11.30s / 0.16s | 53.4 MiB | 2,000 | 8,388,000 | 366,064 B |
| small / model / metadata | warm | 0.05s | 0.05s | 0.04s / 0.01s | 39.0 MiB | 0 | 0 | 366,064 B |
| small / model / verified | population | 2.83s | 2.83s | 10.90s / 0.05s | 71.5 MiB | 2,000 | 8,388,000 | 366,064 B |
| small / model / verified | warm | 0.09s | 0.10s | 0.06s / 0.04s | 46.2 MiB | 0 | 8,388,000 | 366,064 B |
| medium / model / metadata | population | 44.63s | 44.63s | 171.65s / 1.32s | 114.1 MiB | 20,000 | 104,840,000 | 3,660,065 B |
| medium / model / metadata | warm | 0.19s | 0.19s | 0.11s / 0.11s | 88.3 MiB | 0 | 0 | 3,660,065 B |
| medium / model / verified | population | 42.63s | 42.63s | 167.53s / 0.45s | 293.4 MiB | 20,000 | 104,840,000 | 3,660,065 B |
| medium / model / verified | warm | 0.64s | 0.64s | 0.35s / 0.40s | 201.5 MiB | 0 | 104,840,000 | 3,660,065 B |
| small / all / metadata | population | 11.37s | 12.55s | 22.26s / 0.18s | 72.3 MiB | 8,000 | 8,388,000 | 866,064 B |
| small / all / metadata | warm | 0.09s | 0.10s | 0.07s / 0.02s | 51.9 MiB | 0 | 0 | 866,064 B |
| small / all / verified | population | 9.57s | 11.00s | 21.50s / 0.05s | 92.3 MiB | 8,000 | 8,388,000 | 866,064 B |
| small / all / verified | warm | 0.20s | 0.21s | 0.08s / 0.04s | 59.3 MiB | 0 | 8,388,000 | 866,064 B |

Medium metadata population is 3.94% faster than the approved 46.46s uncached median, therefore below the 10% overhead gate. Metadata warm reuse is approximately 245x faster than the uncached median; verified warm reuse is approximately 73x faster while reading and hashing the 100 MiB fixture.

Mutation samples on the 2,000-file fixture used metadata/model mode and three repetitions:

| Scenario | Median / p95 wall | Hits | Misses | Tokenizer calls | Bytes read | Work reason |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| edit one file | 0.05s / 0.06s | 1,999 | 1 | 1 | 16 B | `size_changed=1` |
| add one file | 0.06s / 0.07s | 2,000 | 1 | 1 | 4,194 B | `entry_missing=1` |
| delete one file | 0.05s / 0.05s | 1,999 | 0 | 0 | 0 B | deleted entry removed |
| rename one file | 0.06s / 0.06s | 1,999 | 1 | 1 | 4,194 B | `entry_missing=1` |

The containerized Sequence 04 forced-cold oracle remains green for unchanged, edit, add, delete, rename, root ignore, text/binary transitions, model aliases, encoding changes, approximation-ratio changes, and SentencePiece vocabulary changes. It compares normal and JSON results and checks tokenizer deltas. Metadata's timestamp-preserving false-hit is intentionally demonstrated; verified mode is the exact mode for that case.

### Validation and manifest scale

Current validation-only measurements, one sample per mode, are:

| Tier | Files / bytes | Metadata | Verified | Verified bytes read |
| --- | ---: | ---: | ---: | ---: |
| small | 2,000 / 8,388,000 | 0.002s, 0 B | 0.013s | 8,388,000 B |
| medium | 20,000 / 104,840,000 | 0.027s, 0 B | 0.152s | 104,840,000 B |
| large | 100,000 / 1,073,700,000 | 0.140s, 0 B | 0.983s | 1,073,700,000 B |

Three-sample manifest results:

| Entries | Encoded bytes | Decode median / p95 | Decode alloc | Merge median | Atomic write median |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 2,000 | 636,042 B | 1.649 / 1.753 ms | 2.68 MB | 1.291 ms | 2.364 ms |
| 20,000 | 6,360,042 B | 13.313 / 13.655 ms | 24.7 MiB | 14.879 ms | 22.022 ms |
| 100,000 | 31,800,042 B | 58.997 / 78.397 ms | 120.0 MiB | 93.326 ms | 107.114 ms |

At medium, decode + merge + atomic write is 50.214ms, or 0.113% of the isolated 44.63s metadata population baseline. This uses the approved persistence-share interpretation from the storage decision: persistence cost is compared with the uncached directory-count latency because a complete warm hit has no changed manifest to publish. The large cache population/all-method count was not attempted; full 1 GiB validation and manifest tiers passed, and tokenization capacity is explicitly left for later release testing.

### Representative repository

The mounted project repository was read without recording source content. Its uncached model count was 0.12s median / 0.12s p95 with 109 countable files, 429,462 bytes read, 109 tokenizer calls, and 52.5 MiB peak RSS. The cache model run reported metadata population 0.12s median / 0.12s p95, metadata warm 0.06s / 0.07s with zero tokenizer calls and zero bytes read, verified population 0.11s / 0.12s, and verified warm 0.07s / 0.08s with zero tokenizer calls and 429,462 bytes read. The representative run used a mounted checkout and is not treated as a synthetic-tier performance claim.

### Acceptance gates

| Gate | Outcome | Evidence |
| --- | --- | --- |
| 1. Cached equals forced-cold across mutation matrix | PASS | Container oracle and Sequence 04 result; metadata timestamp caveat is explicit and verified mode matches cold. |
| 2. Complete hit makes zero tokenizer calls | PASS | Medium metadata/verified warm diagnostics: 20,000 hits and 0 tokenizer calls; all-method small warm: 8,000 methods avoided and 0 calls. |
| 3. One-file edit recounts only that file | PASS | Edit: 1 miss, 1 tokenizer call, 1,999 hits; container oracle also compares to forced cold. |
| 4. Medium cold population adds no more than 10% | PASS | 44.63s cache population versus 46.46s uncached median: -3.94% overhead. |
| 5. Medium metadata warm is at least 5x faster | PASS | 0.19s versus 46.46s: approximately 245x faster. |
| 6. Verified performance is measured and documented | PASS | Medium verified population 42.63s and warm 0.64s, with full-read bytes and RSS reported. |
| 7. Persistence is below 10% of medium baseline | PASS | 50.214ms isolated persistence versus 44.63s population: 0.113%; backend remains within the prior decision. |
| 8. Concurrent/interrupted writes are safe | PASS | Full container suite covers distinct-process merge, stale updates, kill/interruption, cancellation, and lock-wait behavior. |
| 9. Corrupt/incompatible/permission failures preserve counts | PASS | Container fallback test compares each result to forced cold and records warnings. |
| 10. Race and normal quality gates pass | PASS | `just test container` and the required quality ladder completed successfully; see the sequence acceptance artifact for the final ladder. |

## Decisions / Deviations

- Verified validation remains the exactness-preserving mode. Metadata mode is opt-in/documented and intentionally has a timestamp-preserving false-hit risk.
- All-method cache coverage is complete at the small tier; medium all-method cache population and 100k cache tokenization were omitted as capacity-expensive. Medium one-model and all-method uncached baselines, all validation tiers, and all manifest tiers are present.
- The benchmark harness now parses the user-facing `Cache diagnostics:` line, so benchmark work counters reflect the current CLI contract rather than the old instrumentation label.

## Handoff

- Machine-readable benchmark surfaces: `tests/container/run.sh`, `.justfiles/test.just`, and this file.
- Correctness/safety surfaces: `tests/cachefs/`, `internal/cache/`, and `tokenizer/cache_integration.go`.
- The next task should review this evidence and decide rollout/default status; no acceptance measurement blocker remains.
