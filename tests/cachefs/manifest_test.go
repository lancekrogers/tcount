//go:build container

package cachefs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
)

func TestAtomicManifestRoundTripInContainer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest")
	manifest := cache.Manifest{
		SchemaVersion: cache.CurrentSchemaVersion,
		Root:          root,
		Generation:    3,
		Entries: map[string]cache.FileEntry{
			"main.go": {
				Size:           12,
				ModTimeNS:      42,
				Classification: cache.ClassificationText,
				Methods: map[cache.ContractKey]int{
					{Method: "bpe_gpt_5", Encoding: "o200k_base", Implementation: "bpe-v1"}: 4,
				},
			},
		},
	}
	if err := cache.WriteManifestAtomic(context.Background(), path, manifest); err != nil {
		t.Fatal(err)
	}
	decoded, err := cache.LoadManifest(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Generation != manifest.Generation || len(decoded.Entries) != 1 {
		t.Fatalf("decoded manifest = %+v", decoded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest permissions = %o, want 600", info.Mode().Perm())
	}
}
