// Test Zus filesystem interface
package zus_test

import (
	"testing"

	"github.com/0chain/rclone_zus/backend/zus"
	"github.com/rclone/rclone/fstest/fstests"
)

// TestIntegration runs integration tests against the remote
func TestIntegration(t *testing.T) {
	fstests.Run(t, &fstests.Opt{
		RemoteName: "automation:",
		NilObject:  (*zus.Object)(nil),
	})
}
