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

func TestLocationEnsureAndStoredRootCollisionInContainer(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(base)
	if err != nil {
		t.Fatal(err)
	}
	location, err := resolver.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := location.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	directoryInfo, err := os.Stat(location.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("cache directory permissions = %o, want 700", directoryInfo.Mode().Perm())
	}

	manifest := cache.Manifest{SchemaVersion: cache.CurrentSchemaVersion, Root: location.Root, Generation: 1, Entries: map[string]cache.FileEntry{
		"main.go": {Size: 1, Classification: cache.ClassificationText},
	}}
	if err := cache.WriteManifestForRoot(context.Background(), location, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.LoadManifestForRoot(context.Background(), location); err != nil {
		t.Fatal(err)
	}

	wrongRoot := manifest
	wrongRoot.Root = filepath.Join(base, "other-repository")
	if err := cache.WriteManifestAtomic(context.Background(), location.ManifestPath, wrongRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.LoadManifestForRoot(context.Background(), location); !errors.Is(err, cache.ErrLocationCollision) {
		t.Fatalf("collision load error = %v, want ErrLocationCollision", err)
	}
}
