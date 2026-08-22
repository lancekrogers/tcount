package cache

import (
	"fmt"
	"path/filepath"
)

func syncParentDirectory(path string) error {
	directory := filepath.Dir(path)
	if err := runDirectorySyncTestHook(directory); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func syncDirectory(directory string) error {
	if directory == "" || directory == "." {
		return nil
	}
	file, err := openDirectoryForSync(directory)
	if err != nil {
		return fmt.Errorf("opening parent directory for sync: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := file.Sync(); err != nil {
		if isUnsupportedDirectorySync(err) {
			return nil
		}
		return fmt.Errorf("syncing parent directory: %w", err)
	}
	return nil
}
