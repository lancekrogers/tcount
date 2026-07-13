package cache

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestManifestRoundTripIsDeterministic(t *testing.T) {
	first := benchmarkManifest(2)
	second := benchmarkManifest(2)
	firstBytes, err := EncodeManifest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := EncodeManifest(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("equivalent manifests encoded differently")
	}
	decoded, err := DecodeManifest(firstBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Entries) != 2 || decoded.Generation != first.Generation || decoded.Root != first.Root {
		t.Fatalf("decoded manifest = %+v", decoded)
	}
}

func TestManifestDecodeRejectsCorruptionAndUnknownVersion(t *testing.T) {
	encoded, err := EncodeManifest(benchmarkManifest(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeManifest(encoded[:len(encoded)-1]); err == nil {
		t.Fatal("truncated manifest decoded successfully")
	}
	unknown := append([]byte(nil), encoded...)
	unknown[4] = 2
	if _, err := DecodeManifest(unknown); err == nil {
		t.Fatal("unknown schema version decoded successfully")
	}
	if _, err := DecodeManifest(append(encoded, 1)); err == nil {
		t.Fatal("manifest with trailing bytes decoded successfully")
	}
}

func TestMergeEntriesAdvancesGeneration(t *testing.T) {
	base := benchmarkManifest(2)
	update := benchmarkManifest(1).Entries["pkg/file-000000.txt"]
	merged, err := MergeEntries(base, UpdateSet{"pkg/file-000000.txt": update})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Generation != base.Generation+1 || len(merged.Entries) != len(base.Entries) {
		t.Fatalf("merged manifest generation/entries = %d/%d", merged.Generation, len(merged.Entries))
	}
}

func TestManifestRejectsNonRelativePaths(t *testing.T) {
	for _, path := range []string{"/absolute.txt", "../outside.txt", "pkg/../outside.txt", "pkg\\file.txt", "pkg//file.txt"} {
		manifest := benchmarkManifest(1)
		entry := manifest.Entries["pkg/file-000000.txt"]
		manifest.Entries = map[string]FileEntry{path: entry}
		if _, err := EncodeManifest(manifest); err == nil {
			t.Fatalf("manifest path %q was accepted", path)
		}
	}
}

func TestManifestWriterBoundsOutput(t *testing.T) {
	writer := manifestWriter{size: MaxManifestBytes}
	writer.u8(1)
	if writer.err == nil {
		t.Fatal("manifest writer accepted bytes beyond the configured limit")
	}
	if len(writer.data) != 0 {
		t.Fatalf("manifest writer appended data after limit: %d bytes", len(writer.data))
	}
}

func TestMergeEntriesDoesNotAliasMethods(t *testing.T) {
	base := benchmarkManifest(1)
	merged, err := MergeEntries(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := "pkg/file-000000.txt"
	key := ContractKey{Method: "new"}
	merged.Entries[path].Methods[key] = 1
	if _, exists := base.Entries[path].Methods[key]; exists {
		t.Fatal("merged entry aliases base method map")
	}
}

func TestMergeEntriesPreservesPartialMethodHits(t *testing.T) {
	base := benchmarkManifest(1)
	path := "pkg/file-000000.txt"
	update := base.Entries[path]
	update.Methods = map[ContractKey]int{{Method: "new-contract"}: 17}

	merged, err := MergeEntries(base, UpdateSet{path: update})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(merged.Entries[path].Methods); got != len(base.Entries[path].Methods)+1 {
		t.Fatalf("merged method count = %d, want %d", got, len(base.Entries[path].Methods)+1)
	}
	if got := merged.Entries[path].Methods[ContractKey{Method: "new-contract"}]; got != 17 {
		t.Fatalf("new method count = %d, want 17", got)
	}
}

func benchmarkManifest(count int) Manifest {
	methods := map[ContractKey]int{
		{Method: "bpe_gpt_5", Encoding: "o200k_base", Implementation: "bpe-v1", NormalizationPolicy: "default", SpecialTokenPolicy: "default"}:     10,
		{Method: "bpe_gpt_4o", Encoding: "o200k_base", Implementation: "bpe-v1", NormalizationPolicy: "default", SpecialTokenPolicy: "default"}:    11,
		{Method: "whitespace_split", Encoding: "approx", Implementation: "builtin-v1", NormalizationPolicy: "default", SpecialTokenPolicy: "none"}: 3,
	}
	entries := make(map[string]FileEntry, count)
	for i := 0; i < count; i++ {
		path := "pkg/file-" + zeroPad(i) + ".txt"
		digest := sha256.Sum256([]byte(path))
		entries[path] = FileEntry{Size: 128, ModTimeNS: int64(i), ContentDigest: digest, Classification: ClassificationText, Characters: 128, Words: 20, Lines: 4, Methods: methods}
	}
	return Manifest{SchemaVersion: CurrentSchemaVersion, Root: "/workspace/fixture", Generation: 7, Entries: entries}
}

func zeroPad(value int) string {
	return fmt.Sprintf("%06d", value)
}
