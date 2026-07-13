package cache

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	manifestMagic               = "TCMF"
	CurrentSchemaVersion uint32 = 1
	MaxManifestBytes     int64  = 256 << 20
	MaxManifestEntries   uint32 = 1_000_000
	MaxManifestMethods   uint32 = 64
	MaxManifestString    uint32 = 4096
	MaxManifestRoot      uint32 = 4096
)

// EncodeManifest serializes a deterministic, bounded binary manifest.
func EncodeManifest(manifest Manifest) ([]byte, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	paths := sortedPaths(manifest.Entries)
	writer := manifestWriter{data: make([]byte, 0, 128)}
	writer.raw([]byte(manifestMagic))
	writer.u32(manifest.SchemaVersion)
	writer.string(manifest.Root)
	writer.u64(manifest.Generation)
	writer.u32(uint32(len(paths)))
	for _, path := range paths {
		writer.string(path)
		writeFileEntry(&writer, manifest.Entries[path])
	}
	if writer.err != nil {
		return nil, writer.err
	}
	return writer.data, nil
}

// DecodeManifest validates and decodes a bounded binary manifest.
func DecodeManifest(data []byte) (Manifest, error) {
	if int64(len(data)) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("manifest exceeds %d bytes", MaxManifestBytes)
	}
	reader := manifestReader{data: data}
	magic, err := reader.raw(len(manifestMagic))
	if err != nil {
		return Manifest{}, err
	}
	if string(magic) != manifestMagic {
		return Manifest{}, errors.New("invalid manifest magic")
	}
	version, err := reader.u32()
	if err != nil {
		return Manifest{}, err
	}
	if version != CurrentSchemaVersion {
		return Manifest{}, fmt.Errorf("%w: unsupported manifest schema version %d", ErrCacheIncompatible, version)
	}
	root, err := reader.string(MaxManifestRoot)
	if err != nil {
		return Manifest{}, err
	}
	generation, err := reader.u64()
	if err != nil {
		return Manifest{}, err
	}
	count, err := reader.u32()
	if err != nil {
		return Manifest{}, err
	}
	if count > MaxManifestEntries {
		return Manifest{}, fmt.Errorf("manifest entry count %d exceeds limit", count)
	}
	manifest := Manifest{SchemaVersion: version, Root: root, Generation: generation, Entries: make(map[string]FileEntry, count)}
	for i := uint32(0); i < count; i++ {
		path, err := reader.string(MaxManifestString)
		if err != nil {
			return Manifest{}, err
		}
		if _, exists := manifest.Entries[path]; exists {
			return Manifest{}, fmt.Errorf("duplicate manifest path %q", path)
		}
		entry, err := readFileEntry(&reader)
		if err != nil {
			return Manifest{}, fmt.Errorf("entry %q: %w", path, err)
		}
		manifest.Entries[path] = entry
	}
	if reader.remaining() != 0 {
		return Manifest{}, errors.New("manifest has trailing bytes")
	}
	return manifest, nil
}

// MergeEntries creates a new generation without mutating the base snapshot.
func MergeEntries(base Manifest, updates UpdateSet) (Manifest, error) {
	if err := validateManifest(base); err != nil {
		return Manifest{}, err
	}
	if uint64(len(base.Entries))+uint64(len(updates)) > uint64(MaxManifestEntries) {
		return Manifest{}, errors.New("merged manifest exceeds entry limit")
	}
	merged := Manifest{SchemaVersion: base.SchemaVersion, Root: base.Root, Generation: base.Generation + 1, Entries: make(map[string]FileEntry, len(base.Entries)+len(updates))}
	for path, entry := range base.Entries {
		merged.Entries[path] = cloneFileResult(entry)
	}
	for path, entry := range updates {
		if err := validatePath(path); err != nil {
			return Manifest{}, err
		}
		if err := validateFileEntry(entry); err != nil {
			return Manifest{}, fmt.Errorf("update %q: %w", path, err)
		}
		if previous, exists := merged.Entries[path]; exists {
			merged.Entries[path] = mergeFileResult(previous, entry)
		} else {
			merged.Entries[path] = cloneFileResult(entry)
		}
	}
	return merged, nil
}

// WriteManifestAtomic publishes a complete manifest through a same-directory
// temporary file, sync, close, and rename sequence.
func WriteManifestAtomic(ctx context.Context, path string, manifest Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := EncodeManifest(manifest)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".manifest-*")
	if err != nil {
		return fmt.Errorf("creating manifest temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("setting manifest permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("writing manifest: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("syncing manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("closing manifest: %w", err)
	}
	if err := runManifestTestHook(ctx, path, temporaryPath); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publishing manifest: %w", err)
	}
	return nil
}

// LoadManifest reads a bounded manifest from disk.
func LoadManifest(ctx context.Context, path string) (Manifest, error) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return Manifest{}, err
	}
	if info.Size() > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: manifest exceeds %d bytes", ErrCacheCorrupt, MaxManifestBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxManifestBytes+1))
	if err != nil {
		return Manifest{}, err
	}
	if int64(len(data)) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: manifest exceeds %d bytes", ErrCacheCorrupt, MaxManifestBytes)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	manifest, err := DecodeManifest(data)
	if err == nil || errors.Is(err, ErrCacheIncompatible) {
		return manifest, err
	}
	return Manifest{}, fmt.Errorf("%w: %w", ErrCacheCorrupt, err)
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("%w: unsupported manifest schema version %d", ErrCacheIncompatible, manifest.SchemaVersion)
	}
	if len(manifest.Root) == 0 || uint64(len(manifest.Root)) > uint64(MaxManifestRoot) {
		return errors.New("manifest root is empty or too long")
	}
	if uint64(len(manifest.Entries)) > uint64(MaxManifestEntries) {
		return errors.New("manifest entry count exceeds limit")
	}
	for path, entry := range manifest.Entries {
		if err := validatePath(path); err != nil {
			return err
		}
		if err := validateFileEntry(entry); err != nil {
			return fmt.Errorf("entry %q: %w", path, err)
		}
	}
	return nil
}

func validatePath(path string) error {
	if path == "" || uint64(len(path)) > uint64(MaxManifestString) {
		return errors.New("manifest path is empty or too long")
	}
	if strings.IndexByte(path, 0) >= 0 || strings.Contains(path, `\`) {
		return errors.New("manifest path must use normalized slash separators")
	}
	if pathpkg.IsAbs(path) || path == "." || path == ".." || strings.HasPrefix(path, "../") || pathpkg.Clean(path) != path {
		return errors.New("manifest path must be normalized and relative")
	}
	return nil
}

func validateFileEntry(entry FileEntry) error {
	if entry.Size < 0 || entry.Characters < 0 || entry.Words < 0 || entry.Lines < 0 {
		return errors.New("manifest entry has a negative numeric field")
	}
	if entry.Classification != ClassificationText && entry.Classification != ClassificationBinary {
		return fmt.Errorf("invalid file classification %d", entry.Classification)
	}
	if uint64(len(entry.Methods)) > uint64(MaxManifestMethods) {
		return errors.New("manifest method count exceeds limit")
	}
	for key, tokens := range entry.Methods {
		if tokens < 0 {
			return errors.New("manifest method has negative token count")
		}
		for _, value := range []string{key.Method, key.Encoding, key.Implementation, key.VocabularyDigest, key.NormalizationPolicy, key.SpecialTokenPolicy} {
			if uint64(len(value)) > uint64(MaxManifestString) {
				return errors.New("manifest contract field is too long")
			}
		}
	}
	return nil
}

func sortedPaths(entries map[string]FileEntry) []string {
	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func writeFileEntry(writer *manifestWriter, entry FileEntry) {
	writer.i64(entry.Size)
	writer.i64(entry.ModTimeNS)
	writer.raw(entry.ContentDigest[:])
	writer.u8(uint8(entry.Classification))
	writer.u64(uint64(entry.Characters))
	writer.u64(uint64(entry.Words))
	writer.u64(uint64(entry.Lines))
	keys := sortedContractKeys(entry.Methods)
	writer.u32(uint32(len(keys)))
	for _, key := range keys {
		writer.string(key.Method)
		writer.string(key.Encoding)
		writer.string(key.Implementation)
		writer.string(key.VocabularyDigest)
		writer.string(key.NormalizationPolicy)
		writer.string(key.SpecialTokenPolicy)
		writer.u64(uint64(entry.Methods[key]))
	}
}

func readFileEntry(reader *manifestReader) (FileEntry, error) {
	size, err := reader.i64()
	if err != nil {
		return FileEntry{}, err
	}
	mtime, err := reader.i64()
	if err != nil {
		return FileEntry{}, err
	}
	digest, err := reader.raw(32)
	if err != nil {
		return FileEntry{}, err
	}
	classification, err := reader.u8()
	if err != nil {
		return FileEntry{}, err
	}
	characters, err := reader.intValue()
	if err != nil {
		return FileEntry{}, err
	}
	words, err := reader.intValue()
	if err != nil {
		return FileEntry{}, err
	}
	lines, err := reader.intValue()
	if err != nil {
		return FileEntry{}, err
	}
	methodCount, err := reader.u32()
	if err != nil {
		return FileEntry{}, err
	}
	if methodCount > MaxManifestMethods {
		return FileEntry{}, errors.New("manifest method count exceeds limit")
	}
	methods := make(map[ContractKey]int, methodCount)
	for i := uint32(0); i < methodCount; i++ {
		values := make([]string, 6)
		for j := range values {
			values[j], err = reader.string(MaxManifestString)
			if err != nil {
				return FileEntry{}, err
			}
		}
		tokens, err := reader.intValue()
		if err != nil {
			return FileEntry{}, err
		}
		key := ContractKey{Method: values[0], Encoding: values[1], Implementation: values[2], VocabularyDigest: values[3], NormalizationPolicy: values[4], SpecialTokenPolicy: values[5]}
		if _, exists := methods[key]; exists {
			return FileEntry{}, errors.New("duplicate manifest contract")
		}
		methods[key] = tokens
	}
	var contentDigest [32]byte
	copy(contentDigest[:], digest)
	entry := FileEntry{Size: size, ModTimeNS: mtime, ContentDigest: contentDigest, Classification: FileClassification(classification), Characters: characters, Words: words, Lines: lines, Methods: methods}
	return entry, validateFileEntry(entry)
}

func sortedContractKeys(methods map[ContractKey]int) []ContractKey {
	keys := make([]ContractKey, 0, len(methods))
	for key := range methods {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return contractKeyLess(keys[i], keys[j])
	})
	return keys
}

func contractKeyLess(left, right ContractKey) bool {
	leftValues := [...]string{left.Method, left.Encoding, left.Implementation, left.VocabularyDigest, left.NormalizationPolicy, left.SpecialTokenPolicy}
	rightValues := [...]string{right.Method, right.Encoding, right.Implementation, right.VocabularyDigest, right.NormalizationPolicy, right.SpecialTokenPolicy}
	for i := range leftValues {
		if leftValues[i] == rightValues[i] {
			continue
		}
		return leftValues[i] < rightValues[i]
	}
	return false
}

type manifestWriter struct {
	data []byte
	size int64
	err  error
}

func (writer *manifestWriter) raw(value []byte) {
	if writer.err != nil {
		return
	}
	if writer.size > MaxManifestBytes-int64(len(value)) {
		writer.err = fmt.Errorf("manifest exceeds %d bytes", MaxManifestBytes)
		return
	}
	writer.data = append(writer.data, value...)
	writer.size += int64(len(value))
}
func (writer *manifestWriter) u8(value uint8) { writer.raw([]byte{value}) }
func (writer *manifestWriter) u32(value uint32) {
	var encoded [4]byte
	binary.LittleEndian.PutUint32(encoded[:], value)
	writer.raw(encoded[:])
}
func (writer *manifestWriter) u64(value uint64) {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], value)
	writer.raw(encoded[:])
}
func (writer *manifestWriter) i64(value int64) { writer.u64(uint64(value)) }
func (writer *manifestWriter) string(value string) {
	writer.u32(uint32(len(value)))
	writer.raw([]byte(value))
}

type manifestReader struct {
	data   []byte
	offset int
}

func (reader *manifestReader) raw(size int) ([]byte, error) {
	if size < 0 || len(reader.data)-reader.offset < size {
		return nil, io.ErrUnexpectedEOF
	}
	value := reader.data[reader.offset : reader.offset+size]
	reader.offset += size
	return value, nil
}
func (reader *manifestReader) u8() (uint8, error) {
	value, err := reader.raw(1)
	if err != nil {
		return 0, err
	}
	return value[0], nil
}
func (reader *manifestReader) u32() (uint32, error) {
	value, err := reader.raw(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(value), nil
}
func (reader *manifestReader) u64() (uint64, error) {
	value, err := reader.raw(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(value), nil
}
func (reader *manifestReader) i64() (int64, error) {
	value, err := reader.u64()
	return int64(value), err
}
func (reader *manifestReader) intValue() (int, error) {
	value, err := reader.u64()
	if err != nil {
		return 0, err
	}
	maxInt := uint64(^uint(0) >> 1)
	if value > maxInt {
		return 0, errors.New("manifest integer exceeds platform limit")
	}
	return int(value), nil
}
func (reader *manifestReader) string(max uint32) (string, error) {
	length, err := reader.u32()
	if err != nil {
		return "", err
	}
	if length > max {
		return "", errors.New("manifest string exceeds limit")
	}
	value, err := reader.raw(int(length))
	return string(value), err
}
func (reader *manifestReader) remaining() int { return len(reader.data) - reader.offset }
