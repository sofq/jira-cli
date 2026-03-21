// export_test.go exposes package-internal symbols for use by external (black-box)
// test files in the same package directory (package client_test).
// This file is compiled only during `go test`.
package client

import "testing"

// SetOAuth2CacheFileForTest redirects the OAuth2 token cache file to a
// temporary location for the duration of test t, then restores the original
// value via t.Cleanup.  Use this in external test files (package client_test)
// to prevent tests from reading or writing the real user cache.
func SetOAuth2CacheFileForTest(t *testing.T, path string) {
	t.Helper()
	old := oauth2CacheFilePathOverride
	oauth2CacheFilePathOverride = path
	t.Cleanup(func() { oauth2CacheFilePathOverride = old })
}
