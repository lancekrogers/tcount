//go:build container

package cachefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
)

func TestWriteManifestAtomicSyncsParentDirectoryInContainer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest")
	var synced []string
	cache.SetDirectorySyncTestHook(func(directory string) error {
		synced = append(synced, directory)
		return nil
	})
	defer cache.SetDirectorySyncTestHook(nil)

	if err := cache.WriteManifestAtomic(context.Background(), path, durabilityManifest(root)); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != root {
		t.Fatalf("parent directory sync calls = %q, want [%q]", synced, root)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("published manifest missing after directory sync: %v", err)
	}
}

func TestWriteManifestAtomicReportsParentDirectorySyncFailureInContainer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest")
	injected := errors.New("injected directory sync failure")
	cache.SetDirectorySyncTestHook(func(string) error { return injected })
	defer cache.SetDirectorySyncTestHook(nil)

	err := cache.WriteManifestAtomic(context.Background(), path, durabilityManifest(root))
	if !errors.Is(err, injected) {
		t.Fatalf("WriteManifestAtomic() error = %v, want injected failure", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("rename already published the manifest; stat error = %v", statErr)
	}
}

func durabilityManifest(root string) cache.Manifest {
	return cache.Manifest{
		SchemaVersion: cache.CurrentSchemaVersion,
		Root:          root,
		Generation:    1,
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
}
