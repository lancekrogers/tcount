# Directory Cache Validation Decision

Status: experimental; no cache validation mode is enabled by default.

Metadata validation compares normalized relative path, size, and nanosecond modification time. It avoids content reads, but it is not content-exact: the container adversarial test rewrites same-size bytes and restores the original timestamp, producing a metadata false hit.

Verified validation reads and SHA-256 hashes the current file bytes. It rejects that timestamp-preserving rewrite and is the required mode for any production cache behavior that promises unconditional exact results. A future metadata mode may be offered only as an explicit, documented performance opt-in under a filesystem-metadata assumption.

Evidence is recorded in the Sequence 01 validation comparison result. The prototype is not connected to normal `tcount` output or persistent storage yet.
