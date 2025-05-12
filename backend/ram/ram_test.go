// Test ram filesystem interface
package ram

import (
	"testing"

	"github.com/rclone/rclone/fstest/fstests"
)

// TestIntegration runs integration tests against the remote
func TestIntegration(t *testing.T) {
	fstests.Run(t, &fstests.Opt{
		RemoteName:  ":ram:",
		NilObject:   (*Object)(nil),
		QuickTestOK: true,
	})
}
