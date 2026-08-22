//go:build windows

package cache

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func isUnsupportedDirectorySync(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_NOT_SUPPORTED)
}

// openDirectoryForSync opens a directory so FlushFileBuffers can run.
// os.Open is GENERIC_READ only; FlushFileBuffers requires GENERIC_WRITE.
// FILE_FLAG_BACKUP_SEMANTICS is required to open a directory handle.
func openDirectoryForSync(directory string) (*os.File, error) {
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), directory), nil
}
