package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	"github.com/spf13/cobra"
	"github.com/tidwall/pretty"
)

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// Client is the core HTTP client for jr. It wraps net/http with auth,
// pagination, jq filtering, and structured error output.
type Client struct {
	BaseURL    string
	Auth       config.AuthConfig
	HTTPClient *http.Client
	Stdout     io.Writer // JSON responses go here
	Stderr     io.Writer // structured errors go here
	JQFilter   string    // --jq filter expression
	Paginate   bool      // auto-paginate GET responses
	DryRun     bool      // output request as JSON, don't execute
	Verbose    bool      // log request/response to stderr
	Pretty     bool      // pretty-print JSON
}

// NewContext stores the client in the given context and returns the new context.
func NewContext(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext retrieves the Client stored in ctx. Returns an error if no
// client was stored.
func FromContext(ctx context.Context) (*Client, error) {
	c, ok := ctx.Value(contextKey{}).(*Client)
	if !ok || c == nil {
		return nil, fmt.Errorf("client: no client found in context")
	}
	return c, nil
}

// QueryFromFlags extracts only the flags from names that have actually been
// changed by the user (i.e. are "set" in the flag set) and returns them as
// url.Values.
func QueryFromFlags(cmd *cobra.Command, names ...string) url.Values {
	q := url.Values{}
	for _, name := range names {
		f := cmd.Flags().Lookup(name)
		if f == nil || !f.Changed {
			continue
		}
		q.Set(name, f.Value.String())
	}
	return q
}

// applyAuth sets authentication headers on the request according to the
// client's Auth configuration.
func (c *Client) applyAuth(req *http.Request) {
	switch strings.ToLower(c.Auth.Type) {
	case "basic":
		req.SetBasicAuth(c.Auth.Username, c.Auth.Token)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.Auth.Token)
	}
}

// Do executes an HTTP request and returns an exit code. It constructs the URL
// from BaseURL+path+query, applies auth, handles pagination, jq filtering, and
// pretty-printing. Errors are written as structured JSON to Stderr.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body io.Reader) int {
	rawURL := c.BaseURL + path
	if len(query) > 0 {
		rawURL = rawURL + "?" + query.Encode()
	}

	// DryRun: emit the request as JSON and return immediately.
	if c.DryRun {
		out, _ := json.Marshal(map[string]string{
			"method": method,
			"url":    rawURL,
		})
		fmt.Fprintf(c.Stdout, "%s\n", out)
		return jrerrors.ExitOK
	}

	// Pagination only for GET requests.
	if c.Paginate && method == "GET" {
		return c.doWithPagination(ctx, method, rawURL, path, query)
	}


	return c.doOnce(ctx, method, rawURL, path, body)
}

// doOnce performs a single HTTP request and writes the response to Stdout.
func (c *Client) doOnce(ctx context.Context, method, rawURL, path string, body io.Reader) int {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuth(req)

	if c.Verbose {
		fmt.Fprintf(c.Stderr, "> %s %s\n", method, rawURL)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}
	defer resp.Body.Close()

	if c.Verbose {
		fmt.Fprintf(c.Stderr, "< %s\n", resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   "reading response body: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}

	// HTTP error (>=400): write structured error to stderr.
	if resp.StatusCode >= 400 {
		apiErr := jrerrors.NewFromHTTP(resp.StatusCode, strings.TrimSpace(string(respBody)), method, path, resp)
		apiErr.WriteJSON(c.Stderr)
		return apiErr.ExitCode()
	}

	return c.writeOutput(respBody)
}

// paginatedPage represents Jira's standard paginated response envelope.
type paginatedPage struct {
	StartAt    int               `json:"startAt"`
	MaxResults int               `json:"maxResults"`
	Total      int               `json:"total"`
	IsLast     *bool             `json:"isLast,omitempty"`
	Values     []json.RawMessage `json:"values"`
}

// isPaginated checks whether the raw JSON body looks like a Jira paginated
// response by verifying the presence of the "values" key.
func isPaginated(body []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	_, ok := probe["values"]
	return ok
}

// doWithPagination fetches all pages of a Jira paginated response (startAt /
// maxResults / total / values pattern), merges the values into the original
// response envelope, and writes it to Stdout. Non-paginated responses are
// passed through unchanged.
func (c *Client) doWithPagination(ctx context.Context, method, firstURL, path string, query url.Values) int {
	// Fetch the first page using the original URL (preserves caller's query params).
	firstBody, code := c.fetchPage(ctx, method, firstURL, path)
	if code != jrerrors.ExitOK {
		return code
	}

	// If it doesn't look like a paginated response, pass through as-is.
	if !isPaginated(firstBody) {
		return c.writeOutput(firstBody)
	}

	var firstPage paginatedPage
	if err := json.Unmarshal(firstBody, &firstPage); err != nil {
		return c.writeOutput(firstBody)
	}

	allValues := append([]json.RawMessage{}, firstPage.Values...)
	lastPage := firstPage

	// Determine if we need more pages.
	for !c.isLastPage(lastPage, len(allValues)) && len(lastPage.Values) > 0 {
		startAt := len(allValues)
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("startAt", fmt.Sprintf("%d", startAt))
		nextURL := c.BaseURL + path + "?" + q.Encode()

		body, code := c.fetchPage(ctx, method, nextURL, path)
		if code != jrerrors.ExitOK {
			return code
		}

		// Use a fresh struct to avoid json.Unmarshal reusing RawMessage
		// backing arrays from previous pages, which would corrupt allValues.
		var nextPage paginatedPage
		if err := json.Unmarshal(body, &nextPage); err != nil {
			break
		}
		allValues = append(allValues, nextPage.Values...)
		lastPage = nextPage
	}

	// Re-serialize the original envelope with all values merged.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(firstBody, &envelope); err != nil {
		return c.writeOutput(firstBody)
	}

	mergedValues, _ := json.Marshal(allValues)
	envelope["values"] = mergedValues
	envelope["startAt"] = json.RawMessage("0")
	total, _ := json.Marshal(len(allValues))
	envelope["total"] = json.RawMessage(total)
	isLastTrue, _ := json.Marshal(true)
	envelope["isLast"] = json.RawMessage(isLastTrue)

	result, err := json.Marshal(envelope)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   "marshaling paginated results: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}

	return c.writeOutput(result)
}

// isLastPage returns true when there are no more pages to fetch.
func (c *Client) isLastPage(page paginatedPage, fetched int) bool {
	if page.IsLast != nil {
		return *page.IsLast
	}
	if page.Total > 0 {
		return fetched >= page.Total
	}
	return len(page.Values) == 0
}

// fetchPage performs a single GET request and returns the response body.
func (c *Client) fetchPage(ctx context.Context, method, rawURL, path string) ([]byte, int) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)

	if c.Verbose {
		fmt.Fprintf(c.Stderr, "> %s %s\n", method, rawURL)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}
	defer resp.Body.Close()

	if c.Verbose {
		fmt.Fprintf(c.Stderr, "< %s\n", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Status:    0,
			Message:   "reading response body: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}

	if resp.StatusCode >= 400 {
		apiErr := jrerrors.NewFromHTTP(resp.StatusCode, strings.TrimSpace(string(body)), method, path, resp)
		apiErr.WriteJSON(c.Stderr)
		return nil, apiErr.ExitCode()
	}

	return body, jrerrors.ExitOK
}

// writeOutput applies optional JQ filtering and pretty-printing, then writes
// the final JSON bytes to Stdout. Returns an exit code.
func (c *Client) writeOutput(data []byte) int {
	// Apply JQ filter.
	if c.JQFilter != "" {
		filtered, err := jq.Apply(data, c.JQFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Status:    0,
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(c.Stderr)
			return jrerrors.ExitError
		}
		data = filtered
	}

	// Pretty-print if requested.
	if c.Pretty {
		data = pretty.Pretty(data)
	}

	fmt.Fprintf(c.Stdout, "%s\n", strings.TrimRight(string(data), "\n"))
	return jrerrors.ExitOK
}
