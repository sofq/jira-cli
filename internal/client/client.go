package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sofq/jira-cli/internal/audit"
	"github.com/sofq/jira-cli/internal/cache"
	"github.com/sofq/jira-cli/internal/config"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	"github.com/sofq/jira-cli/internal/policy"
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
	Fields     string        // --fields comma-separated field names for GET
	CacheTTL   time.Duration // --cache duration; 0 means no caching

	// Security fields
	AuditLogger *audit.Logger  // nil means no audit logging
	Profile     string         // profile name for audit entries
	Operation   string         // "resource verb" for audit entries
	Policy      *policy.Policy // nil means unrestricted
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

// ApplyAuth sets authentication headers on the request according to the
// client's Auth configuration. Returns an error if authentication setup fails
// (e.g. OAuth2 token fetch failure or empty token).
func (c *Client) ApplyAuth(req *http.Request) error {
	switch strings.ToLower(c.Auth.Type) {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.Auth.Token)
	case "oauth2":
		token, err := c.fetchOAuth2Token()
		if err != nil {
			return fmt.Errorf("oauth2 authentication failed: %w", err)
		}
		if token == "" {
			return fmt.Errorf("oauth2 token endpoint returned an empty access_token")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	default: // "basic" and any unrecognized type default to basic auth
		req.SetBasicAuth(c.Auth.Username, c.Auth.Token)
	}
	return nil
}

// fetchOAuth2Token performs a client_credentials grant and returns the access token.
func (c *Client) fetchOAuth2Token() (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.Auth.ClientID},
		"client_secret": {c.Auth.ClientSecret},
	}
	if c.Auth.Scopes != "" {
		data.Set("scope", c.Auth.Scopes)
	}

	resp, err := c.HTTPClient.Post(c.Auth.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("oauth2 token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("oauth2 token request failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("oauth2 token response decode failed: %w", err)
	}
	return tokenResp.AccessToken, nil
}

// Do executes an HTTP request and returns an exit code. It constructs the URL
// from BaseURL+path+query, applies auth, handles pagination, jq filtering, and
// pretty-printing. Errors are written as structured JSON to Stderr.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body io.Reader) int {
	origPath := path

	// Append --fields as query param for GET requests.
	if c.Fields != "" && method == "GET" {
		if query == nil {
			query = url.Values{}
		}
		query.Set("fields", c.Fields)
	}

	// Ensure path separator between BaseURL and path.
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	rawURL := c.BaseURL + path
	if len(query) > 0 {
		if strings.Contains(rawURL, "?") {
			rawURL = rawURL + "&" + query.Encode()
		} else {
			rawURL = rawURL + "?" + query.Encode()
		}
	}

	// DryRun: emit the request as JSON and return immediately.
	if c.DryRun {
		dryOut := map[string]any{
			"method": method,
			"url":    rawURL,
		}
		if body != nil {
			bodyBytes, err := io.ReadAll(body)
			if err == nil && len(bodyBytes) > 0 {
				var parsed any
				if json.Unmarshal(bodyBytes, &parsed) == nil {
					dryOut["body"] = parsed
				} else {
					dryOut["body"] = string(bodyBytes)
				}
			}
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(dryOut)
		exitCode := c.WriteOutput(bytes.TrimRight(buf.Bytes(), "\n"))
		c.auditLog(method, origPath, 0, exitCode)
		return exitCode
	}

	// Pagination only for GET requests.
	if c.Paginate && method == "GET" {
		return c.doWithPagination(ctx, method, rawURL, path, query)
	}


	return c.doOnce(ctx, method, rawURL, path, body)
}

// doOnce performs a single HTTP request and writes the response to Stdout.
func (c *Client) doOnce(ctx context.Context, method, rawURL, path string, body io.Reader) int {
	// Cache check for GET requests.
	var cacheKey string
	if c.CacheTTL > 0 && method == "GET" {
		cacheKey = cache.Key(method, rawURL, c.cacheAuthContext())
		if data, ok := cache.Get(cacheKey, c.CacheTTL); ok {
			exitCode := c.WriteOutput(data)
			c.auditLog(method, path, 0, exitCode)
			return exitCode
		}
	}

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
	if err := c.ApplyAuth(req); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "auth_error",
			Status:    0,
			Message:   err.Error(),
			Hint:      "Check your oauth2 configuration: client_id, client_secret, token_url must be set",
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitAuth
	}

	c.VerboseLog(map[string]any{"type": "request", "method": method, "url": rawURL})

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

	c.VerboseLog(map[string]any{"type": "response", "status": resp.StatusCode})

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
		c.auditLog(method, path, resp.StatusCode, apiErr.ExitCode())
		return apiErr.ExitCode()
	}

	// HTTP 204 No Content — emit empty JSON object to maintain JSON-only contract.
	if len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		respBody = []byte("{}")
	}

	// Cache successful GET responses.
	if cacheKey != "" {
		if err := cache.Set(cacheKey, respBody); err != nil {
			c.VerboseLog(map[string]any{"type": "warning", "message": "cache write failed: " + err.Error()})
		}
	}

	exitCode := c.WriteOutput(respBody)
	c.auditLog(method, path, resp.StatusCode, exitCode)
	return exitCode
}

// paginatedPage represents Jira's standard paginated response envelope.
type paginatedPage struct {
	StartAt    int               `json:"startAt"`
	MaxResults int               `json:"maxResults"`
	Total      int               `json:"total"`
	IsLast     *bool             `json:"isLast,omitempty"`
	Values     []json.RawMessage `json:"values"`
}

// tokenPaginatedPage represents Jira's token-based paginated response
// (e.g. the new /search/jql endpoint which uses "issues" + "nextPageToken").
type tokenPaginatedPage struct {
	Issues        []json.RawMessage `json:"issues"`
	NextPageToken string            `json:"nextPageToken,omitempty"`
	IsLast        *bool             `json:"isLast,omitempty"`
}

// paginationType identifies which pagination pattern a response uses.
type paginationType int

const (
	paginationNone  paginationType = iota
	paginationValue                // "values" + startAt/total
	paginationToken                // "issues" + nextPageToken
)

// detectPagination checks which pagination pattern the response body matches.
func detectPagination(body []byte) paginationType {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return paginationNone
	}
	if _, ok := probe["values"]; ok {
		return paginationValue
	}
	if _, ok := probe["issues"]; ok {
		return paginationToken
	}
	return paginationNone
}

// doWithPagination fetches all pages of a Jira paginated response (startAt /
// maxResults / total / values pattern), merges the values into the original
// response envelope, and writes it to Stdout. Non-paginated responses are
// passed through unchanged.
func (c *Client) doWithPagination(ctx context.Context, method, firstURL, path string, query url.Values) int {
	// Check cache before fetching pages.
	var cacheKey string
	if c.CacheTTL > 0 {
		cacheKey = cache.Key(method, firstURL, c.cacheAuthContext())
		if data, ok := cache.Get(cacheKey, c.CacheTTL); ok {
			exitCode := c.WriteOutput(data)
			c.auditLog(method, path, 0, exitCode)
			return exitCode
		}
	}

	// Fetch the first page using the original URL (preserves caller's query params).
	firstBody, code := c.fetchPage(ctx, method, firstURL, path)
	if code != jrerrors.ExitOK {
		return code
	}

	// Detect pagination type.
	pagType := detectPagination(firstBody)
	if pagType == paginationNone {
		if cacheKey != "" {
			if err := cache.Set(cacheKey, firstBody); err != nil {
				c.VerboseLog(map[string]any{"type": "warning", "message": "cache write failed: " + err.Error()})
			}
		}
		return c.WriteOutput(firstBody)
	}

	// Dispatch to the appropriate pagination handler.
	if pagType == paginationToken {
		return c.doTokenPagination(ctx, method, path, query, firstBody, cacheKey)
	}

	return c.doStartAtPagination(ctx, method, path, query, firstBody, cacheKey)
}

// buildURL constructs a URL from base, path, and query, correctly handling
// paths that may already contain query parameters.
func (c *Client) buildURL(path string, query url.Values) string {
	rawURL := c.BaseURL + path
	if strings.Contains(rawURL, "?") {
		return rawURL + "&" + query.Encode()
	}
	return rawURL + "?" + query.Encode()
}

// doStartAtPagination handles traditional startAt/values pagination.
func (c *Client) doStartAtPagination(ctx context.Context, method, path string, query url.Values, firstBody []byte, cacheKey string) int {
	var firstPage paginatedPage
	if err := json.Unmarshal(firstBody, &firstPage); err != nil {
		return c.WriteOutput(firstBody)
	}

	allValues := append([]json.RawMessage{}, firstPage.Values...)
	lastPage := firstPage

	for !c.isLastPage(lastPage, firstPage.StartAt+len(allValues)) && len(lastPage.Values) > 0 {
		startAt := firstPage.StartAt + len(allValues)
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("startAt", fmt.Sprintf("%d", startAt))
		nextURL := c.buildURL(path, q)

		body, code := c.fetchPage(ctx, method, nextURL, path)
		if code != jrerrors.ExitOK {
			return code
		}

		var nextPage paginatedPage
		if err := json.Unmarshal(body, &nextPage); err != nil {
			break
		}
		allValues = append(allValues, nextPage.Values...)
		lastPage = nextPage
	}

	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(firstBody, &envelope) // firstBody already validated above

	var valBuf bytes.Buffer
	valEnc := json.NewEncoder(&valBuf)
	valEnc.SetEscapeHTML(false)
	_ = valEnc.Encode(allValues)
	envelope["values"] = json.RawMessage(bytes.TrimRight(valBuf.Bytes(), "\n"))
	envelope["startAt"] = json.RawMessage("0")
	total, _ := json.Marshal(len(allValues))
	envelope["total"] = json.RawMessage(total)
	isLastTrue, _ := json.Marshal(true)
	envelope["isLast"] = json.RawMessage(isLastTrue)

	return c.encodePaginatedResult(envelope, cacheKey)
}

// doTokenPagination handles nextPageToken/issues pagination (new JQL search endpoint).
func (c *Client) doTokenPagination(ctx context.Context, method, path string, query url.Values, firstBody []byte, cacheKey string) int {
	var firstPage tokenPaginatedPage
	if err := json.Unmarshal(firstBody, &firstPage); err != nil {
		return c.WriteOutput(firstBody)
	}

	allIssues := append([]json.RawMessage{}, firstPage.Issues...)
	pageToken := firstPage.NextPageToken
	isLast := firstPage.IsLast

	lastPageLen := len(firstPage.Issues)
	for pageToken != "" && (isLast == nil || !*isLast) && lastPageLen > 0 {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("nextPageToken", pageToken)
		nextURL := c.buildURL(path, q)

		body, code := c.fetchPage(ctx, method, nextURL, path)
		if code != jrerrors.ExitOK {
			return code
		}

		var nextPage tokenPaginatedPage
		if err := json.Unmarshal(body, &nextPage); err != nil {
			break
		}
		allIssues = append(allIssues, nextPage.Issues...)
		lastPageLen = len(nextPage.Issues)
		pageToken = nextPage.NextPageToken
		isLast = nextPage.IsLast
	}

	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(firstBody, &envelope) // firstBody already validated above

	var issBuf bytes.Buffer
	issEnc := json.NewEncoder(&issBuf)
	issEnc.SetEscapeHTML(false)
	_ = issEnc.Encode(allIssues)
	envelope["issues"] = json.RawMessage(bytes.TrimRight(issBuf.Bytes(), "\n"))
	delete(envelope, "nextPageToken")
	isLastTrue, _ := json.Marshal(true)
	envelope["isLast"] = json.RawMessage(isLastTrue)

	return c.encodePaginatedResult(envelope, cacheKey)
}

// encodePaginatedResult serializes a pagination envelope to JSON and writes it to output.
func (c *Client) encodePaginatedResult(envelope map[string]json.RawMessage, cacheKey string) int {
	var resultBuf bytes.Buffer
	enc := json.NewEncoder(&resultBuf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(envelope) // envelope contains only valid json.RawMessage values
	result := bytes.TrimRight(resultBuf.Bytes(), "\n")

	if cacheKey != "" {
		if err := cache.Set(cacheKey, result); err != nil {
			c.VerboseLog(map[string]any{"type": "warning", "message": "cache write failed: " + err.Error()})
		}
	}

	return c.WriteOutput(result)
}

// isLastPage returns true when there are no more pages to fetch.
func (c *Client) isLastPage(page paginatedPage, fetched int) bool {
	if page.IsLast != nil {
		return *page.IsLast
	}
	if page.Total > 0 {
		return fetched >= page.Total
	}
	// When total is 0 (or absent), stop if the page is empty OR if the server
	// claimed total=0 despite returning values (avoids infinite loop).
	if page.Total == 0 && fetched > 0 {
		return true
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
	if err := c.ApplyAuth(req); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "auth_error",
			Status:    0,
			Message:   err.Error(),
			Hint:      "Check your oauth2 configuration: client_id, client_secret, token_url must be set",
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitAuth
	}

	c.VerboseLog(map[string]any{"type": "request", "method": method, "url": rawURL})

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

	c.VerboseLog(map[string]any{"type": "response", "status": resp.StatusCode})

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

// cacheAuthContext returns a string that uniquely identifies the auth configuration,
// so that different profiles/credentials produce different cache keys for the same URL.
func (c *Client) cacheAuthContext() string {
	return c.BaseURL + "\x00" + c.Auth.Type + "\x00" + c.Auth.Username + "\x00" + c.Auth.Token
}

// VerboseLog writes a structured JSON log entry to stderr when verbose mode is enabled.
func (c *Client) VerboseLog(fields map[string]any) {
	if !c.Verbose {
		return
	}
	data, _ := json.Marshal(fields)
	fmt.Fprintf(c.Stderr, "%s\n", data)
}

// auditLog writes an audit entry if an AuditLogger is configured.
func (c *Client) auditLog(method, path string, status, exitCode int) {
	if c.AuditLogger == nil {
		return
	}
	c.AuditLogger.Log(audit.Entry{
		Profile:   c.Profile,
		Operation: c.Operation,
		Method:    method,
		Path:      path,
		Status:    status,
		Exit:      exitCode,
		DryRun:    c.DryRun,
	})
}

// Fetch performs an HTTP request and returns the raw response body and an exit code.
// Unlike Do, it does not write to Stdout — callers handle the body themselves.
// This is used by workflow commands and batch operations that need the raw response.
func (c *Client) Fetch(ctx context.Context, method, path string, body io.Reader) ([]byte, int) {
	fullURL := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Message:   "failed to create request: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.ApplyAuth(req); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "auth_error",
			Message:   err.Error(),
			Hint:      "Check your oauth2 configuration: client_id, client_secret, token_url must be set",
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitAuth
	}

	c.VerboseLog(map[string]any{"type": "request", "method": method, "url": fullURL})

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "connection_error", Message: err.Error()}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}
	defer resp.Body.Close()

	c.VerboseLog(map[string]any{"type": "response", "status": resp.StatusCode})

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Message:   "reading response body: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}
	if resp.StatusCode >= 400 {
		apiErr := jrerrors.NewFromHTTP(resp.StatusCode, strings.TrimSpace(string(respBody)), method, path, resp)
		apiErr.WriteJSON(c.Stderr)
		c.auditLog(method, path, resp.StatusCode, apiErr.ExitCode())
		return nil, apiErr.ExitCode()
	}
	c.auditLog(method, path, resp.StatusCode, jrerrors.ExitOK)
	return respBody, jrerrors.ExitOK
}

// WriteOutput applies optional JQ filtering and pretty-printing, then writes
// the final JSON bytes to Stdout. Returns an exit code.
func (c *Client) WriteOutput(data []byte) int {
	// Apply JQ filter.
	if c.JQFilter != "" {
		filtered, err := jq.Apply(data, c.JQFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Status:    0,
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(c.Stderr)
			return jrerrors.ExitValidation
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
