package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	"github.com/tidwall/pretty"
)

// maxBackoff is the ceiling for error backoff. Exported as var for testing.
var maxBackoff = 5 * time.Minute

// Options configures the watch loop.
type Options struct {
	JQL       string
	Issue     string // single-issue shorthand (converted to JQL)
	Interval  time.Duration
	Fields    []string // fields to request from Jira
	MaxEvents int // 0 = unlimited
}

// jqlSearchRequest is the POST body for /rest/api/3/search/jql.
type jqlSearchRequest struct {
	JQL    string   `json:"jql"`
	Fields []string `json:"fields,omitempty"`
}

// searchResponse is the response from /rest/api/3/search/jql.
type searchResponse struct {
	Issues []json.RawMessage `json:"issues"`
}

// issueFingerprint extracts key + updated timestamp for change detection.
type issueFingerprint struct {
	Key     string `json:"key"`
	Updated string `json:"updated"`
}

// Event is a single change event emitted as NDJSON.
type Event struct {
	Type  string          `json:"type"` // "created", "updated", "removed"
	Issue json.RawMessage `json:"issue"`
}

// Run starts the polling loop and writes NDJSON events to the client's Stdout.
// It returns when ctx is cancelled, maxEvents is reached, or a fatal error occurs.
func Run(ctx context.Context, c *client.Client, opts Options) int {
	jqlQuery := opts.JQL
	if opts.Issue != "" {
		jqlQuery = "key = " + opts.Issue
	}

	// Build the request body.
	searchReq := jqlSearchRequest{JQL: jqlQuery}
	if len(opts.Fields) > 0 {
		searchReq.Fields = opts.Fields
	}

	seen := make(map[string]string) // key -> updated timestamp
	emitted := 0
	firstPoll := true

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	// Run first poll immediately.
	if code := poll(ctx, c, searchReq, seen, &emitted, opts.MaxEvents, firstPoll); code != jrerrors.ExitOK {
		if code == -1 { // maxEvents reached
			return jrerrors.ExitOK
		}
		return code
	}
	firstPoll = false

	consecutiveErrors := 0
	for {
		select {
		case <-ctx.Done():
			return jrerrors.ExitOK
		case <-ticker.C:
			code := poll(ctx, c, searchReq, seen, &emitted, opts.MaxEvents, firstPoll)
			if code == -1 { // maxEvents reached
				return jrerrors.ExitOK
			}
			if code == jrerrors.ExitAuth {
				// Auth errors won't self-heal; stop immediately.
				return code
			}
			if code != jrerrors.ExitOK {
				consecutiveErrors++
				// Backoff: skip ticks on repeated errors to avoid flooding.
				if consecutiveErrors > 1 {
					backoff := time.Duration(consecutiveErrors) * opts.Interval
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
					select {
					case <-ctx.Done():
						return jrerrors.ExitOK
					case <-time.After(backoff):
					}
				}
				continue
			}
			consecutiveErrors = 0
		}
	}
}

// poll performs a single JQL search and emits events for new/changed issues.
// Returns -1 when maxEvents is reached, an exit code on fatal error, or ExitOK.
func poll(ctx context.Context, c *client.Client, searchReq jqlSearchRequest, seen map[string]string, emitted *int, maxEvents int, firstPoll bool) int {
	reqBody, _ := json.Marshal(searchReq)
	respBody, exitCode := c.Fetch(ctx, "POST", "/rest/api/3/search/jql", bytes.NewReader(reqBody))
	if exitCode != jrerrors.ExitOK {
		return exitCode
	}

	var resp searchResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Message:   "failed to parse search response: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitError
	}

	// Build current set of keys for removal detection.
	currentKeys := make(map[string]bool, len(resp.Issues))

	for _, raw := range resp.Issues {
		var fp issueFingerprint
		if err := json.Unmarshal(raw, &fp); err != nil {
			continue
		}
		// Also extract fields.updated if key-level updated is empty.
		if fp.Updated == "" {
			var withFields struct {
				Fields struct {
					Updated string `json:"updated"`
				} `json:"fields"`
			}
			if json.Unmarshal(raw, &withFields) == nil {
				fp.Updated = withFields.Fields.Updated
			}
		}

		currentKeys[fp.Key] = true

		prevUpdated, exists := seen[fp.Key]
		if !exists {
			// New issue.
			seen[fp.Key] = fp.Updated
			eventType := "created"
			if firstPoll {
				eventType = "initial"
			}
			if code := emitEvent(c, eventType, raw, emitted, maxEvents); code != jrerrors.ExitOK {
				return code
			}
		} else if fp.Updated != prevUpdated {
			// Changed issue.
			seen[fp.Key] = fp.Updated
			if code := emitEvent(c, "updated", raw, emitted, maxEvents); code != jrerrors.ExitOK {
				return code
			}
		}
	}

	// Detect removed issues (only after first poll).
	// Collect keys to remove first, then process them — this avoids
	// partial map mutation if emitEvent returns early on maxEvents.
	if !firstPoll {
		var removedKeys []string
		for key := range seen {
			if !currentKeys[key] {
				removedKeys = append(removedKeys, key)
			}
		}
		for _, key := range removedKeys {
			delete(seen, key)
			removedJSON, _ := marshalNoEscape(map[string]string{"key": key})
			if code := emitEvent(c, "removed", removedJSON, emitted, maxEvents); code != jrerrors.ExitOK {
				return code
			}
		}
	}

	return jrerrors.ExitOK
}

// emitEvent writes a single NDJSON event to stdout.
// Returns -1 when maxEvents is reached.
func emitEvent(c *client.Client, eventType string, issueData json.RawMessage, emitted *int, maxEvents int) int {
	event := Event{
		Type:  eventType,
		Issue: issueData,
	}
	data, _ := marshalNoEscape(event)

	// Apply JQ filter if set.
	output := data
	if c.JQFilter != "" {
		filtered, err := jq.Apply(data, c.JQFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(c.Stderr)
			return jrerrors.ExitValidation
		}
		output = filtered
	}

	if c.Pretty {
		output = pretty.Pretty(output)
	}

	fmt.Fprintf(c.Stdout, "%s\n", strings.TrimRight(string(output), "\n"))
	*emitted++

	if maxEvents > 0 && *emitted >= maxEvents {
		return -1 // sentinel: maxEvents reached
	}
	return jrerrors.ExitOK
}

// marshalNoEscape marshals v to JSON without HTML escaping.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// DryRunOutput returns the request that would be made.
func DryRunOutput(w io.Writer, baseURL string, opts Options) {
	jqlQuery := opts.JQL
	if opts.Issue != "" {
		jqlQuery = "key = " + opts.Issue
	}
	out, _ := marshalNoEscape(map[string]any{
		"method":   "POST",
		"url":      baseURL + "/rest/api/3/search/jql",
		"interval": opts.Interval.String(),
		"jql":      jqlQuery,
		"note":     "would poll this query at the given interval, emitting NDJSON events for changes",
	})
	fmt.Fprintf(w, "%s\n", out)
}
