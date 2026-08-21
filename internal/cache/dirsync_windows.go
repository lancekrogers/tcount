//go:build windows

package cache

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isUnsupportedDirectorySync(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_NOT_SUPPORTED)
}
