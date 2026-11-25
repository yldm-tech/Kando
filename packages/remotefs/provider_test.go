package remotefs_test

import (
	"testing"

	"kando/packages/remotefs"
	"kando/packages/remotefs/providertest"
)

// TestProviders checks the rules every provider keeps against the WebDAV, SFTP and SMB providers.
func TestProviders(t *testing.T) {
	providertest.Run(t, remotefs.RemoteProviders)
}
