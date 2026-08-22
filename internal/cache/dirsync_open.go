//go:build !windows

package cache

import "os"

func openDirectoryForSync(directory string) (*os.File, error) {
	return os.Open(directory)
}
