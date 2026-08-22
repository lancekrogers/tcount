//go:build unix

package cache

import (
	"errors"
	"syscall"
)

func isUnsupportedDirectorySync(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EOPNOTSUPP)
}
