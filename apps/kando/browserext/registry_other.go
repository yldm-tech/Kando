//go:build !windows

package browserext

// userRegistry is nil: only Windows registers native messaging hosts in a registry.
func userRegistry() Registry { return nil }
