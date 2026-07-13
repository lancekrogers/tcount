package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalRootLocationIsStableAndNamespaced(t *testing.T) {
	resolver, err := NewLocationResolverAt("/tmp/tcount-location-test")
	if err != nil {
		t.Fatal(err)
	}
	first, err := resolver.Resolve("/workspace/project/../project")
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.Resolve("/workspace/project")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent roots resolved differently:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if filepath.Dir(first.ManifestPath) != first.Directory || filepath.Base(first.ManifestPath) != manifestName {
		t.Fatalf("manifest path = %q, directory=%q", first.ManifestPath, first.Directory)
	}
	if first.RootHash == "" || first.RootHash == first.Root {
		t.Fatalf("unexpected root hash %q", first.RootHash)
	}
	if filepath.Base(filepath.Dir(first.Directory)) != "roots" {
		t.Fatalf("location escaped roots namespace: %q", first.Directory)
	}
}

func TestDefaultResolverUsesUserCacheDirectory(t *testing.T) {
	resolver, err := NewLocationResolver()
	if err != nil {
		t.Fatal(err)
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	location, err := resolver.Resolve("/workspace/project")
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := filepath.Join(userCache, cacheNamespace, cacheVersion, "roots") + string(filepath.Separator)
	if len(location.Directory) <= len(wantPrefix) || location.Directory[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("default location = %q, want prefix %q", location.Directory, wantPrefix)
	}
}

func TestLocationRejectsEmptyInputs(t *testing.T) {
	if _, err := NewLocationResolverAt(""); err == nil {
		t.Fatal("empty cache base was accepted")
	}
	resolver, err := NewLocationResolverAt("/tmp/tcount-location-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(""); err == nil {
		t.Fatal("empty root was accepted")
	}
}
