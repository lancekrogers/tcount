package cache

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryStoreCommitLoadStatusAndClear(t *testing.T) {
	store := NewMemoryStore()
	root := "/workspace/project"
	path := "main.go"
	entry := FileResult{Size: 10, ModTimeNS: 20, Classification: ClassificationText, Characters: 10, Words: 2, Lines: 1, Methods: map[ContractKey]int{{Method: "bpe"}: 3}}

	if _, err := store.Load(context.Background(), root); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("empty Load error = %v, want ErrSnapshotNotFound", err)
	}
	if err := store.Commit(context.Background(), root, 0, UpdateSet{path: entry}); err != nil {
		t.Fatal(err)
	}

	status, err := store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Present || status.Generation != 1 || status.Entries != 1 {
		t.Fatalf("status = %+v", status)
	}

	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Entries[path].Methods[ContractKey{Method: "mutated"}] = 99
	loadedAgain, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := loadedAgain.Entries[path].Methods[ContractKey{Method: "mutated"}]; exists {
		t.Fatal("Load returned an aliased method map")
	}

	if err := store.Clear(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	status, err = store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Present {
		t.Fatalf("status after Clear = %+v", status)
	}
}

func TestMemoryStoreRejectsGenerationConflict(t *testing.T) {
	store := NewMemoryStore()
	root := "/workspace/project"
	if err := store.Commit(context.Background(), root, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(context.Background(), root, 0, nil); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("stale commit error = %v, want ErrGenerationConflict", err)
	}
}

func TestMemoryStoreHonorsCancellation(t *testing.T) {
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := "/workspace/project"
	if _, err := store.Load(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context.Canceled", err)
	}
	if err := store.Commit(ctx, root, 0, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Commit error = %v, want context.Canceled", err)
	}
	if _, err := store.Status(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("Status error = %v, want context.Canceled", err)
	}
	if err := store.Clear(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("Clear error = %v, want context.Canceled", err)
	}
}

func TestMemoryStoreRejectsEmptyRoot(t *testing.T) {
	store := NewMemoryStore()
	if err := store.Commit(context.Background(), "", 0, nil); !errors.Is(err, ErrInvalidRoot) {
		t.Fatalf("empty root error = %v, want ErrInvalidRoot", err)
	}
}
