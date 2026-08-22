//go:build !unix && !windows

package cache

func isUnsupportedDirectorySync(error) bool {
	return true
}
