package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/sofq/jira-cli/internal/adf"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/spf13/cobra"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "High-level workflow commands (transition, assign, comment)",
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

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Add a plain-text comment to an issue",
	Long:  "Posts a comment to a Jira issue. The --text value is converted to Atlassian Document Format automatically.",
	RunE:  runComment,
}

var moveCmd = &cobra.Command{
	Use:   "move",
	Short: "Transition an issue and optionally reassign in one step",
	Long:  "Transitions an issue to a new status and optionally reassigns it. Use --to for the target status and --assign for the assignee.",
	RunE:  runMove,
}

var createIssueCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an issue from flags (no raw JSON needed)",
	Long:  "Creates a Jira issue using individual flags. Required: --project, --type, --summary. Optional: --description, --assign, --priority, --labels, --parent.",
	RunE:  runCreateIssue,
}

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Create an issue link by type name (resolves link type ID)",
	Long:  "Creates a link between two issues. Use --from, --to, and --type. The link type name is matched case-insensitively against name, inward, and outward fields.",
	RunE:  runLink,
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

	commentCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = commentCmd.MarkFlagRequired("issue")
	commentCmd.Flags().String("text", "", "comment text (plain text, converted to ADF)")
	_ = commentCmd.MarkFlagRequired("text")

	moveCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = moveCmd.MarkFlagRequired("issue")
	moveCmd.Flags().String("to", "", "target status name (case-insensitive match)")
	_ = moveCmd.MarkFlagRequired("to")
	moveCmd.Flags().String("assign", "", "assignee after transition: display name, email, 'me', 'none', or 'unassign'")

	createIssueCmd.Flags().String("project", "", "project key (e.g. PROJ)")
	_ = createIssueCmd.MarkFlagRequired("project")
	createIssueCmd.Flags().String("type", "", "issue type name (e.g. Bug, Story, Task)")
	_ = createIssueCmd.MarkFlagRequired("type")
	createIssueCmd.Flags().String("summary", "", "issue summary/title")
	_ = createIssueCmd.MarkFlagRequired("summary")
	createIssueCmd.Flags().String("description", "", "issue description (plain text, converted to ADF)")
	createIssueCmd.Flags().String("assign", "", "assignee: display name, email, 'me', 'none', or 'unassign'")
	createIssueCmd.Flags().String("priority", "", "priority name (e.g. High, Medium, Low)")
	createIssueCmd.Flags().String("labels", "", "comma-separated list of labels")
	createIssueCmd.Flags().String("parent", "", "parent issue key (e.g. PROJ-100)")

	linkCmd.Flags().String("from", "", "source issue key (e.g. PROJ-1)")
	_ = linkCmd.MarkFlagRequired("from")
	linkCmd.Flags().String("to", "", "target issue key (e.g. PROJ-2)")
	_ = linkCmd.MarkFlagRequired("to")
	linkCmd.Flags().String("type", "", "link type name (e.g. blocks, clones, relates to)")
	_ = linkCmd.MarkFlagRequired("type")

	workflowCmd.AddCommand(transitionCmd)
	workflowCmd.AddCommand(assignCmd)
	workflowCmd.AddCommand(commentCmd)
	workflowCmd.AddCommand(createIssueCmd)
	workflowCmd.AddCommand(moveCmd)
	workflowCmd.AddCommand(linkCmd)
}

// transitionMatch holds a matched transition's ID and name.
type transitionMatch struct {
	ID   string
	Name string
}

// resolveTransition fetches available transitions for an issue and matches one by name.
// Returns the matched transition or writes an error to c.Stderr and returns a non-zero exit code.
func resolveTransition(ctx context.Context, c *client.Client, issueKey, toStatus string) (*transitionMatch, int) {
	transitionsBody, exitCode := c.Fetch(ctx, "GET",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey), nil)
	if exitCode != jrerrors.ExitOK {
		return nil, exitCode
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
		return nil, jrerrors.ExitError
	}

	toLower := strings.ToLower(toStatus)
	// Exact match first.
	for _, tr := range transResp.Transitions {
		if strings.ToLower(tr.Name) == toLower || strings.ToLower(tr.To.Name) == toLower {
			return &transitionMatch{ID: tr.ID, Name: tr.Name}, jrerrors.ExitOK
		}
	}
	// Substring match.
	for _, tr := range transResp.Transitions {
		if strings.Contains(strings.ToLower(tr.Name), toLower) ||
			strings.Contains(strings.ToLower(tr.To.Name), toLower) {
			return &transitionMatch{ID: tr.ID, Name: tr.Name}, jrerrors.ExitOK
		}
	}

	names := make([]string, len(transResp.Transitions))
	for i, tr := range transResp.Transitions {
		names[i] = tr.Name
	}
	apiErr := &jrerrors.APIError{
		ErrorType: "validation_error",
		Message:   fmt.Sprintf("no transition matching %q; available: %s", toStatus, strings.Join(names, ", ")),
	}
	apiErr.WriteJSON(c.Stderr)
	return nil, jrerrors.ExitValidation
}

// resolveAssignee resolves a "to" argument into an account ID.
// Special values: "me" (current user), "none"/"unassign" (returns empty string meaning null assignment).
// Returns (accountID, isUnassign, exitCode).
func resolveAssignee(ctx context.Context, c *client.Client, to string) (string, bool, int) {
	switch strings.ToLower(to) {
	case "me":
		body, code := c.Fetch(ctx, "GET", "/rest/api/3/myself", nil)
		if code != jrerrors.ExitOK {
			return "", false, code
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
			return "", false, jrerrors.ExitError
		}
		if me.AccountID == "" {
			apiErr := &jrerrors.APIError{
				ErrorType: "connection_error",
				Message:   "could not determine your account ID from /myself",
			}
			apiErr.WriteJSON(c.Stderr)
			return "", false, jrerrors.ExitError
		}
		return me.AccountID, false, jrerrors.ExitOK

	case "none", "unassign":
		return "", true, jrerrors.ExitOK

	default:
		body, code := c.Fetch(ctx, "GET",
			fmt.Sprintf("/rest/api/3/user/search?query=%s", url.QueryEscape(to)), nil)
		if code != jrerrors.ExitOK {
			return "", false, code
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
			return "", false, jrerrors.ExitError
		}
		if len(users) == 0 {
			apiErr := &jrerrors.APIError{
				ErrorType: "not_found",
				Message:   fmt.Sprintf("no user found matching %q", to),
			}
			apiErr.WriteJSON(c.Stderr)
			return "", false, jrerrors.ExitNotFound
		}
		return users[0].AccountID, false, jrerrors.ExitOK
	}
}

func runTransition(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
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

	match, exitCode := resolveTransition(cmd.Context(), c, issueKey, toStatus)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	// Execute transition. Use c.Fetch to avoid c.Do() writing "{}" to stdout.
	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": match.ID}})
	_, exitCode = c.Fetch(cmd.Context(), "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
		bytes.NewReader(transBody))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status":     "transitioned",
		"issue":      issueKey,
		"transition": match.Name,
	})
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

func runMove(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	toStatus, _ := cmd.Flags().GetString("to")
	assign, _ := cmd.Flags().GetString("assign")

	// Respect --dry-run: emit the request details without executing.
	if c.DryRun {
		dryOut := map[string]any{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		}
		if assign != "" {
			dryOut["assign"] = fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, assign)
		}
		out, _ := marshalNoEscape(dryOut)
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	match, exitCode := resolveTransition(cmd.Context(), c, issueKey, toStatus)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": match.ID}})
	_, exitCode = c.Fetch(cmd.Context(), "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey),
		bytes.NewReader(transBody))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	result := map[string]any{
		"status":     "moved",
		"issue":      issueKey,
		"transition": match.Name,
	}

	if assign != "" {
		accountID, isUnassign, code := resolveAssignee(cmd.Context(), c, assign)
		if code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}

		if isUnassign {
			assignBody := `{"accountId":null}`
			_, code = c.Fetch(cmd.Context(), "PUT",
				fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
				strings.NewReader(assignBody))
			if code != jrerrors.ExitOK {
				return &jrerrors.AlreadyWrittenError{Code: code}
			}
			result["assigned"] = "unassigned"
		} else {
			marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
			_, code = c.Fetch(cmd.Context(), "PUT",
				fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
				bytes.NewReader(marshaledAssign))
			if code != jrerrors.ExitOK {
				return &jrerrors.AlreadyWrittenError{Code: code}
			}
			result["assigned"] = assign
		}
	}

	out, _ := marshalNoEscape(result)
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

func runAssign(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
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

	accountID, isUnassign, exitCode := resolveAssignee(cmd.Context(), c, to)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	if isUnassign {
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
	}

	marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
	_, exitCode = c.Fetch(cmd.Context(), "PUT",
		fmt.Sprintf("/rest/api/3/issue/%s/assignee", issueKey),
		bytes.NewReader(marshaledAssign))
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

func runCreateIssue(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	project, _ := cmd.Flags().GetString("project")
	issueType, _ := cmd.Flags().GetString("type")
	summary, _ := cmd.Flags().GetString("summary")
	description, _ := cmd.Flags().GetString("description")
	assign, _ := cmd.Flags().GetString("assign")
	priority, _ := cmd.Flags().GetString("priority")
	labels, _ := cmd.Flags().GetString("labels")
	parent, _ := cmd.Flags().GetString("parent")

	fields := map[string]any{
		"project":   map[string]string{"key": project},
		"issuetype": map[string]string{"name": issueType},
		"summary":   summary,
	}
	if description != "" {
		fields["description"] = adf.FromText(description)
	}
	if priority != "" {
		fields["priority"] = map[string]string{"name": priority}
	}
	if labels != "" {
		fields["labels"] = strings.Split(labels, ",")
	}
	if parent != "" {
		fields["parent"] = map[string]string{"key": parent}
	}

	// Respect --dry-run: emit the request details without executing.
	if c.DryRun {
		bodyPreview, _ := marshalNoEscape(map[string]any{"fields": fields})
		out, _ := marshalNoEscape(map[string]any{
			"method": "POST",
			"url":    c.BaseURL + "/rest/api/3/issue",
			"body":   json.RawMessage(bodyPreview),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	// Resolve assignee if provided.
	if assign != "" {
		accountID, isUnassign, exitCode := resolveAssignee(cmd.Context(), c, assign)
		if exitCode != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: exitCode}
		}
		if !isUnassign {
			fields["assignee"] = map[string]string{"accountId": accountID}
		}
	}

	createBody, _ := json.Marshal(map[string]any{"fields": fields})
	respBody, exitCode := c.Fetch(cmd.Context(), "POST", "/rest/api/3/issue", bytes.NewReader(createBody))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	if exitCode := c.WriteOutput(respBody); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

// resolveLinkType fetches available issue link types and matches one by name.
// Matches by name, inward, or outward (case-insensitive exact first, then substring).
// Returns the resolved type name or writes an error and returns a non-zero exit code.
func resolveLinkType(ctx context.Context, c *client.Client, typeName string) (string, int) {
	body, exitCode := c.Fetch(ctx, "GET", "/rest/api/3/issueLinkType", nil)
	if exitCode != jrerrors.ExitOK {
		return "", exitCode
	}

	var linkTypesResp struct {
		IssueLinkTypes []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Inward  string `json:"inward"`
			Outward string `json:"outward"`
		} `json:"issueLinkTypes"`
	}
	if err := json.Unmarshal(body, &linkTypesResp); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Message:   "failed to parse issueLinkType response: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return "", jrerrors.ExitError
	}

	toLower := strings.ToLower(typeName)
	// Exact match first (name, inward, or outward).
	for _, lt := range linkTypesResp.IssueLinkTypes {
		if strings.ToLower(lt.Name) == toLower ||
			strings.ToLower(lt.Inward) == toLower ||
			strings.ToLower(lt.Outward) == toLower {
			return lt.Name, jrerrors.ExitOK
		}
	}
	// Substring match.
	for _, lt := range linkTypesResp.IssueLinkTypes {
		if strings.Contains(strings.ToLower(lt.Name), toLower) ||
			strings.Contains(strings.ToLower(lt.Inward), toLower) ||
			strings.Contains(strings.ToLower(lt.Outward), toLower) {
			return lt.Name, jrerrors.ExitOK
		}
	}

	names := make([]string, len(linkTypesResp.IssueLinkTypes))
	for i, lt := range linkTypesResp.IssueLinkTypes {
		names[i] = lt.Name
	}
	apiErr := &jrerrors.APIError{
		ErrorType: "validation_error",
		Message:   fmt.Sprintf("no link type matching %q; available: %s", typeName, strings.Join(names, ", ")),
	}
	apiErr.WriteJSON(c.Stderr)
	return "", jrerrors.ExitValidation
}

func runLink(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	linkType, _ := cmd.Flags().GetString("type")

	// Respect --dry-run: emit the request details without executing.
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + "/rest/api/3/issueLink",
			"note":   fmt.Sprintf("would link %s -> %s with type %q (link type ID resolved at runtime)", from, to, linkType),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	resolvedType, exitCode := resolveLinkType(cmd.Context(), c, linkType)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	linkBody, _ := json.Marshal(map[string]any{
		"type":         map[string]string{"name": resolvedType},
		"inwardIssue":  map[string]string{"key": to},
		"outwardIssue": map[string]string{"key": from},
	})
	_, exitCode = c.Fetch(cmd.Context(), "POST", "/rest/api/3/issueLink", bytes.NewReader(linkBody))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "linked",
		"from":   from,
		"to":     to,
		"type":   resolvedType,
	})
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

func runComment(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	text, _ := cmd.Flags().GetString("text")

	// Respect --dry-run: emit the request details without executing.
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/comment", issueKey),
			"note":   fmt.Sprintf("would add comment to %s", issueKey),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	commentBody, _ := json.Marshal(map[string]any{"body": adf.FromText(text)})
	_, exitCode := c.Fetch(cmd.Context(), "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/comment", issueKey),
		bytes.NewReader(commentBody))
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "commented",
		"issue":  issueKey,
	})
	if exitCode := c.WriteOutput(out); exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}
