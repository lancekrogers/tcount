# Directory Cache Storage Decision

Status: approved conditionally for the experimental v1 store boundary.

Use one deterministic, versioned binary manifest per canonical root, published with same-directory temporary-file write, file `Sync`, close, atomic rename, and parent-directory `Sync`. Concurrent writers take an exclusive inter-process lock (`flock` on Unix, `LockFileEx` on Windows). Keep serialization and filesystem ownership inside the cache store; count results must depend only on the store boundary, not this format.

Parent-directory fsync is required after rename so the new directory entry is durable on crash-safe local filesystems. Filesystems that report directory sync as unsupported (`EINVAL` / `ENOTSUP` / `ENOSYS` on Unix, `ERROR_INVALID_FUNCTION` / `ERROR_NOT_SUPPORTED` on Windows) skip that extra metadata flush after the file sync and rename have already completed. Writer locks remain unavailable only on platforms without a Unix or Windows lock implementation.

The container prototype used three representative tokenizer contracts per entry. It measured 636,042 bytes / 6,360,042 bytes / 31,800,042 bytes for 2,000 / 20,000 / 100,000 entries. Medium atomic-write median was 22.5 ms; decode plus merge plus write was approximately 38 ms, below 10% of the measured 46.46 s one-model and 98.42 s all-method medium count baselines.

The 100,000-entry decode allocated approximately 126 MB, so the implementation must retain bounded decode limits and monitor memory during acceptance. Reopen the backend if representative repositories show material memory pressure or persistence exceeds the 10% warm-latency threshold.

Rejected initial alternatives:

- JSON: easy to inspect but unnecessarily verbose and slower for large manifests; a diagnostic summary is preferred over exposing the internal format.
- Embedded SQLite or another transactional database: useful if the single-manifest threshold fails, but adds dependency, binary/package, cross-platform, locking, and migration surface before those costs are justified.
- One file per source file: rejected because inode and directory-entry amplification scales poorly and complicates atomic generation semantics.
