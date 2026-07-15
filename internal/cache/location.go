package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	cacheNamespace = "tcount"
	cacheVersion   = "v1"
	manifestName   = "manifest"
)

// LocationResolver maps every canonical root to one user-cache namespace.
// baseDir is the parent of the tcount namespace and is injectable for tests.
type LocationResolver struct {
	baseDir string
}

// CacheLocation is the complete resolved location for one canonical root.
type CacheLocation struct {
	Root         string
	RootHash     string
	Directory    string
	ManifestPath string
}

func (resolver LocationResolver) rootsDirectory() string {
	return filepath.Join(resolver.baseDir, cacheNamespace, cacheVersion, "roots")
}

// NewLocationResolver uses os.UserCacheDir as the default parent. It does not
// create directories or modify the counted repository.
func NewLocationResolver() (LocationResolver, error) {
	baseDir, err := os.UserCacheDir()
	if err != nil {
		return LocationResolver{}, fmt.Errorf("resolving user cache directory: %w", err)
	}
	return NewLocationResolverAt(baseDir)
}

// NewLocationResolverAt injects a parent cache directory for tests and
// constrained deployments. The returned resolver still appends tcount/v1.
func NewLocationResolverAt(baseDir string) (LocationResolver, error) {
	if baseDir == "" {
		return LocationResolver{}, fmt.Errorf("%w: cache base is empty", ErrInvalidRoot)
	}
	absolute, err := filepath.Abs(baseDir)
	if err != nil {
		return LocationResolver{}, fmt.Errorf("resolving cache base %q: %w", baseDir, err)
	}
	return LocationResolver{baseDir: filepath.Clean(absolute)}, nil
}

// CanonicalRoot returns the stable root spelling shared by count, status, and
// clear. Existing symlinks are evaluated; a not-yet-existing path still gets a
// clean absolute spelling so management commands resolve the same hash.
func CanonicalRoot(root string) (string, error) {
	if root == "" {
		return "", ErrInvalidRoot
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving root %q: %w", root, err)
	}
	clean := filepath.Clean(absolute)
	resolved, err := filepath.EvalSymlinks(clean)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if os.IsNotExist(err) {
		return clean, nil
	}
	return "", fmt.Errorf("evaluating root %q: %w", root, err)
}

// Resolve returns the user-cache location for root without creating it.
func (resolver LocationResolver) Resolve(root string) (CacheLocation, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return CacheLocation{}, err
	}
	digest := sha256.Sum256([]byte(canonical))
	rootHash := hex.EncodeToString(digest[:])
	directory := filepath.Join(resolver.baseDir, cacheNamespace, cacheVersion, "roots", rootHash)
	return CacheLocation{
		Root:         canonical,
		RootHash:     rootHash,
		Directory:    directory,
		ManifestPath: filepath.Join(directory, manifestName),
	}, nil
}

// Ensure creates the root cache directory with user-only permissions. It is
// intentionally separate from Resolve so discovery remains side-effect free.
func (location CacheLocation) Ensure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(location.Directory, 0o700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	if err := os.Chmod(location.Directory, 0o700); err != nil {
		return fmt.Errorf("setting cache directory permissions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// LoadManifestForRoot validates the stored canonical root after decoding. A
// hash collision or manually moved manifest therefore becomes a cold-safe
// location error instead of a reusable snapshot from another repository.
func LoadManifestForRoot(ctx context.Context, location CacheLocation) (Manifest, error) {
	manifest, err := LoadManifest(ctx, location.ManifestPath)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Root != location.Root {
		return Manifest{}, fmt.Errorf("%w: stored %q, requested %q", ErrLocationCollision, manifest.Root, location.Root)
	}
	return manifest, nil
}

// WriteManifestForRoot refuses to publish a snapshot under a different root.
func WriteManifestForRoot(ctx context.Context, location CacheLocation, manifest Manifest) error {
	if manifest.Root != location.Root {
		return fmt.Errorf("%w: manifest %q, location %q", ErrLocationCollision, manifest.Root, location.Root)
	}
	if err := location.Ensure(ctx); err != nil {
		return err
	}
	return WriteManifestAtomic(ctx, location.ManifestPath, manifest)
}
