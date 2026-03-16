package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
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
	Long:  "Assigns an issue. Use --to with a display name, email, 'me', 'none', or 'unassign'. Use 'none' or 'unassign' to remove the assignee.",
	RunE:  runAssign,
}

func init() {
	transitionCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = transitionCmd.MarkFlagRequired("issue")
	transitionCmd.Flags().String("to", "", "target status name (case-insensitive match)")
	_ = transitionCmd.MarkFlagRequired("to")

	assignCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = assignCmd.MarkFlagRequired("issue")
	assignCmd.Flags().String("to", "", "assignee: display name, email, 'me', 'none', or 'unassign'")
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

	// Respect --dry-run: emit the request details without executing.
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	// 1. Fetch available transitions.
	transitionsBody, exitCode := c.Fetch(cmd.Context(), "GET",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey), nil)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
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
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
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
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// 3. Execute transition. Use c.Fetch to avoid c.Do() writing "{}" to stdout.
	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": matchedID}})
	body := string(transBody)
	_, exitCode = c.Fetch(cmd.Context(), "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
		strings.NewReader(body))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status":     "transitioned",
		"issue":      issueKey,
		"transition": matchedName,
	})
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

func runAssign(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		return err
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	to, _ := cmd.Flags().GetString("to")

	// Respect --dry-run: emit the request details without executing.
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "PUT",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
			"note":   fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, to),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	var accountID string

	switch strings.ToLower(to) {
	case "me":
		body, code := c.Fetch(cmd.Context(), "GET", "/rest/api/3/myself", nil)
		if code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		var me struct {
			AccountID string `json:"accountId"`
		}
		if err := json.Unmarshal(body, &me); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Message:   "failed to parse /myself response: " + err.Error(),
			}
			apiErr.WriteJSON(c.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
		}
		if me.AccountID == "" {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Message:   "could not determine your account ID from /myself",
			}
			apiErr.WriteJSON(c.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
		}
		accountID = me.AccountID

	case "none", "unassign":
		assignBody := `{"accountId":null}`
		_, exitCode := c.Fetch(cmd.Context(), "PUT",
			fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
			strings.NewReader(assignBody))
		if exitCode != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: exitCode}
		}
		out, _ := marshalNoEscape(map[string]string{
			"status": "unassigned",
			"issue":  issueKey,
		})
		if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: exitCode}
		}
		return nil

	default:
		body, code := c.Fetch(cmd.Context(), "GET",
			fmt.Sprintf("/rest/api/3/user/search?query=%s", url.QueryEscape(to)), nil)
		if code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		var users []struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		}
		if err := json.Unmarshal(body, &users); err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Message:   "failed to parse user search response: " + err.Error(),
			}
			apiErr.WriteJSON(c.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
		}
		if len(users) == 0 {
			apiErr := &jrerrors.APIError{
				ErrorType: "not_found",
				Message:   fmt.Sprintf("no user found matching %q", to),
			}
			apiErr.WriteJSON(c.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
		}
		accountID = users[0].AccountID
	}

	marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
	assignBody := string(marshaledAssign)
	_, exitCode := c.Fetch(cmd.Context(), "PUT",
		fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
		strings.NewReader(assignBody))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "assigned",
		"issue":  issueKey,
		"to":     to,
	})
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

