package cache

import "testing"

func TestValidationModeString(t *testing.T) {
	if Metadata.String() != "metadata" {
		t.Fatalf("Metadata.String() = %q, want metadata", Metadata.String())
	}
	if Verified.String() != "verified" {
		t.Fatalf("Verified.String() = %q, want verified", Verified.String())
	}
	if ValidationMode(99).String() != "unknown" {
		t.Fatalf("unknown mode string = %q, want unknown", ValidationMode(99).String())
	}
}

func TestNewValidatorRejectsUnknownMode(t *testing.T) {
	if _, err := NewValidator(ValidationMode(99)); err == nil {
		t.Fatal("NewValidator(99) returned nil error")
	}
}

func TestValidationStatsSnapshot(t *testing.T) {
	var stats ValidationStats
	stats.filesChecked.Add(2)
	stats.hits.Add(1)
	stats.misses.Add(1)
	stats.fullReads.Add(1)
	stats.bytesRead.Add(4)

	snapshot := stats.Snapshot()
	if snapshot.FilesChecked != 2 || snapshot.Hits != 1 || snapshot.Misses != 1 || snapshot.FullReads != 1 || snapshot.BytesRead != 4 {
		t.Fatalf("unexpected stats snapshot: %+v", snapshot)
	}
}
