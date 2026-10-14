//go:build integration

package worker_test

import (
	"os"
	"testing"

	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}
