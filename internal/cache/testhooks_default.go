//go:build !container

package cache

import "context"

func runManifestTestHook(context.Context, string, string) error { return nil }
