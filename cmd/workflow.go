package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "High-level workflow commands (transition, assign)",
}

var transitionCmd = &cobra.Command{
	Use:   "transition",
	Short: "Transition an issue to a new status by name",
	Long:  "Resolves the transition ID automatically. Use --to to specify the target status name (case-insensitive substring match).",
	RunE:  runTransition,
}

var assignCmd = &cobra.Command{
	Use:   "assign",
	Short: "Assign an issue to a user",
	Long:  "Assigns an issue. Use --to with a display name, email, or 'me' to resolve the account ID automatically.",
	RunE:  runAssign,
}

func init() {
	transitionCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = transitionCmd.MarkFlagRequired("issue")
	transitionCmd.Flags().String("to", "", "target status name (case-insensitive match)")
	_ = transitionCmd.MarkFlagRequired("to")

	assignCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = assignCmd.MarkFlagRequired("issue")
	assignCmd.Flags().String("to", "", "assignee: display name, email, or 'me'")
	_ = assignCmd.MarkFlagRequired("to")

	workflowCmd.AddCommand(transitionCmd)
	workflowCmd.AddCommand(assignCmd)
}

func runTransition(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		return err
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	toStatus, _ := cmd.Flags().GetString("to")

	// 1. Fetch available transitions.
	transitionsBody, exitCode := fetchJSON(c, cmd.Context(), "GET",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey))
	if exitCode != jrerrors.ExitOK {
		os.Exit(exitCode)
	}

	var transResp struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal(transitionsBody, &transResp); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Message:   "failed to parse transitions: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		os.Exit(jrerrors.ExitError)
	}

	// 2. Match target status (case-insensitive exact match first, then substring).
	toLower := strings.ToLower(toStatus)
	var matchedID, matchedName string
	for _, tr := range transResp.Transitions {
		if strings.ToLower(tr.Name) == toLower || strings.ToLower(tr.To.Name) == toLower {
			matchedID = tr.ID
			matchedName = tr.Name
			break
		}
	}
	if matchedID == "" {
		for _, tr := range transResp.Transitions {
			if strings.Contains(strings.ToLower(tr.Name), toLower) ||
				strings.Contains(strings.ToLower(tr.To.Name), toLower) {
				matchedID = tr.ID
				matchedName = tr.Name
				break
			}
		}
	}
	if matchedID == "" {
		names := make([]string, len(transResp.Transitions))
		for i, tr := range transResp.Transitions {
			names[i] = tr.Name
		}
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   fmt.Sprintf("no transition matching %q; available: %s", toStatus, strings.Join(names, ", ")),
		}
		apiErr.WriteJSON(c.Stderr)
		os.Exit(jrerrors.ExitValidation)
	}

	// 3. Execute transition.
	body := fmt.Sprintf(`{"transition":{"id":"%s"}}`, matchedID)
	code := c.Do(cmd.Context(), "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
		nil, strings.NewReader(body))
	if code != jrerrors.ExitOK {
		os.Exit(code)
	}

	out, _ := json.Marshal(map[string]string{
		"status":     "transitioned",
		"issue":      issueKey,
		"transition": matchedName,
	})
	fmt.Fprintf(c.Stdout, "%s\n", out)
	return nil
}

func runAssign(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		return err
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	to, _ := cmd.Flags().GetString("to")

	var accountID string

	switch strings.ToLower(to) {
	case "me":
		body, code := fetchJSON(c, cmd.Context(), "GET", "/rest/api/3/myself")
		if code != jrerrors.ExitOK {
			os.Exit(code)
		}
		var me struct {
			AccountID string `json:"accountId"`
		}
		json.Unmarshal(body, &me)
		accountID = me.AccountID

	case "none", "unassign":
		assignBody := `{"accountId":null}`
		code := c.Do(cmd.Context(), "PUT",
			fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
			nil, strings.NewReader(assignBody))
		if code != jrerrors.ExitOK {
			os.Exit(code)
		}
		out, _ := json.Marshal(map[string]string{
			"status": "unassigned",
			"issue":  issueKey,
		})
		fmt.Fprintf(c.Stdout, "%s\n", out)
		return nil

	default:
		body, code := fetchJSON(c, cmd.Context(), "GET",
			fmt.Sprintf("/rest/api/3/user/search?query=%s", to))
		if code != jrerrors.ExitOK {
			os.Exit(code)
		}
		var users []struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(body, &users)
		if len(users) == 0 {
			apiErr := &jrerrors.APIError{
				ErrorType: "not_found",
				Message:   fmt.Sprintf("no user found matching %q", to),
			}
			apiErr.WriteJSON(c.Stderr)
			os.Exit(jrerrors.ExitNotFound)
		}
		accountID = users[0].AccountID
	}

	assignBody := fmt.Sprintf(`{"accountId":"%s"}`, accountID)
	code := c.Do(cmd.Context(), "PUT",
		fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
		nil, strings.NewReader(assignBody))
	if code != jrerrors.ExitOK {
		os.Exit(code)
	}

	out, _ := json.Marshal(map[string]string{
		"status": "assigned",
		"issue":  issueKey,
		"to":     to,
	})
	fmt.Fprintf(c.Stdout, "%s\n", out)
	return nil
}

// fetchJSON performs an HTTP request and returns the raw response body.
func fetchJSON(c *client.Client, ctx context.Context, method, path string) ([]byte, int) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, nil)
	if err != nil {
		return nil, jrerrors.ExitError
	}
	req.Header.Set("Accept", "application/json")
	c.ApplyAuth(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "connection_error", Message: err.Error()}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		apiErr := jrerrors.NewFromHTTP(resp.StatusCode, string(body), method, path, resp)
		apiErr.WriteJSON(c.Stderr)
		return nil, apiErr.ExitCode()
	}
	return body, jrerrors.ExitOK
}
