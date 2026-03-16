package cmd

import (
	"context"
	"io"

	"github.com/sofq/jira-cli/internal/client"
)

// BatchTransitionForTest exposes batchTransition for unit tests.
func BatchTransitionForTest(ctx context.Context, c *client.Client, issueKey, toStatus string) int {
	return batchTransition(ctx, c, issueKey, toStatus)
}

// ExportTestConnection exposes testConnection for unit tests.
func ExportTestConnection(baseURL, authType, username, token string) error {
	return testConnection(baseURL, authType, username, token)
}

// Exported wrappers for testing internal functions.
var FetchJSONWithBody = func(c *client.Client, ctx context.Context, method, path string, body io.Reader) ([]byte, int) {
	return fetchJSONWithBody(c, ctx, method, path, body)
}
var MarshalNoEscape = marshalNoEscape
