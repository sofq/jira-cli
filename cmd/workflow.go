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
	"github.com/sofq/jira-cli/internal/duration"
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

var logWorkCmd = &cobra.Command{
	Use:   "log-work",
	Short: "Add a worklog entry with human-friendly duration (e.g. 2h 30m)",
	RunE:  runLogWork,
}

var sprintCmd = &cobra.Command{
	Use:   "sprint",
	Short: "Move an issue to a sprint by name (resolves sprint ID)",
	RunE:  runSprint,
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

	logWorkCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = logWorkCmd.MarkFlagRequired("issue")
	logWorkCmd.Flags().String("time", "", "time spent (e.g. 2h 30m, 1d, 45m)")
	_ = logWorkCmd.MarkFlagRequired("time")
	logWorkCmd.Flags().String("comment", "", "optional worklog comment (plain text)")

	sprintCmd.Flags().String("issue", "", "issue key (e.g. PROJ-123)")
	_ = sprintCmd.MarkFlagRequired("issue")
	sprintCmd.Flags().String("to", "", "sprint name (case-insensitive match)")
	_ = sprintCmd.MarkFlagRequired("to")

	workflowCmd.AddCommand(transitionCmd)
	workflowCmd.AddCommand(assignCmd)
	workflowCmd.AddCommand(commentCmd)
	workflowCmd.AddCommand(createIssueCmd)
	workflowCmd.AddCommand(moveCmd)
	workflowCmd.AddCommand(linkCmd)
	workflowCmd.AddCommand(logWorkCmd)
	workflowCmd.AddCommand(sprintCmd)
}

// writeValidationError writes a validation error to the client's stderr and returns ExitValidation.
func writeValidationError(c *client.Client, message string) int {
	apiErr := &jrerrors.APIError{
		ErrorType: "validation_error",
		Message:   message,
	}
	apiErr.WriteJSON(c.Stderr)
	return jrerrors.ExitValidation
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
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)), nil)
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

	if code := doTransition(cmd.Context(), c, issueKey, toStatus); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doTransition is the shared core for workflow transition, used by both
// the Cobra RunE handler and the batch executor.
func doTransition(ctx context.Context, c *client.Client, issueKey, toStatus string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		})
		return c.WriteOutput(out)
	}

	match, code := resolveTransition(ctx, c, issueKey, toStatus)
	if code != jrerrors.ExitOK {
		return code
	}

	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": match.ID}})
	_, code = c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
		bytes.NewReader(transBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status":     "transitioned",
		"issue":      issueKey,
		"transition": match.Name,
	})
	return c.WriteOutput(out)
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

	if code := doMove(cmd.Context(), c, issueKey, toStatus, assign); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doMove is the shared core for workflow move (transition + optional assign),
// used by both the Cobra RunE handler and the batch executor.
func doMove(ctx context.Context, c *client.Client, issueKey, toStatus, assign string) int {
	if c.DryRun {
		dryOut := map[string]any{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would transition %s to %q (transition ID resolved at runtime)", issueKey, toStatus),
		}
		if assign != "" {
			dryOut["assign"] = fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, assign)
		}
		out, _ := marshalNoEscape(dryOut)
		return c.WriteOutput(out)
	}

	match, code := resolveTransition(ctx, c, issueKey, toStatus)
	if code != jrerrors.ExitOK {
		return code
	}

	transBody, _ := json.Marshal(map[string]any{"transition": map[string]any{"id": match.ID}})
	_, code = c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey)),
		bytes.NewReader(transBody))
	if code != jrerrors.ExitOK {
		return code
	}

	result := map[string]any{
		"status":     "moved",
		"issue":      issueKey,
		"transition": match.Name,
	}

	if assign != "" {
		accountID, isUnassign, code := resolveAssignee(ctx, c, assign)
		if code != jrerrors.ExitOK {
			return code
		}

		if isUnassign {
			assignBody := `{"accountId":null}`
			_, code = c.Fetch(ctx, "PUT",
				fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
				strings.NewReader(assignBody))
			if code != jrerrors.ExitOK {
				return code
			}
			result["assigned"] = "unassigned"
		} else {
			marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
			_, code = c.Fetch(ctx, "PUT",
				fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
				bytes.NewReader(marshaledAssign))
			if code != jrerrors.ExitOK {
				return code
			}
			result["assigned"] = assign
		}
	}

	out, _ := marshalNoEscape(result)
	return c.WriteOutput(out)
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

	if code := doAssign(cmd.Context(), c, issueKey, to); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doAssign is the shared core for workflow assign, used by both the Cobra
// RunE handler and the batch executor.
func doAssign(ctx context.Context, c *client.Client, issueKey, to string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "PUT",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would assign %s to %q (account ID resolved at runtime)", issueKey, to),
		})
		return c.WriteOutput(out)
	}

	accountID, isUnassign, code := resolveAssignee(ctx, c, to)
	if code != jrerrors.ExitOK {
		return code
	}

	if isUnassign {
		assignBody := `{"accountId":null}`
		_, code := c.Fetch(ctx, "PUT",
			fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
			strings.NewReader(assignBody))
		if code != jrerrors.ExitOK {
			return code
		}
		out, _ := marshalNoEscape(map[string]string{
			"status": "unassigned",
			"issue":  issueKey,
		})
		return c.WriteOutput(out)
	}

	marshaledAssign, _ := json.Marshal(map[string]string{"accountId": accountID})
	_, code = c.Fetch(ctx, "PUT",
		fmt.Sprintf("/rest/api/3/issue/%s/assignee", url.PathEscape(issueKey)),
		bytes.NewReader(marshaledAssign))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "assigned",
		"issue":  issueKey,
		"to":     to,
	})
	return c.WriteOutput(out)
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

	args2 := map[string]string{
		"project":     project,
		"type":        issueType,
		"summary":     summary,
		"description": description,
		"assign":      assign,
		"priority":    priority,
		"labels":      labels,
		"parent":      parent,
	}

	if code := doCreateIssue(cmd.Context(), c, args2); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doCreateIssue is the shared core for workflow create, used by both the
// Cobra RunE handler and the batch executor.
func doCreateIssue(ctx context.Context, c *client.Client, args map[string]string) int {
	project := args["project"]
	if project == "" {
		return writeValidationError(c, "workflow create requires a 'project' arg")
	}
	issueType := args["type"]
	if issueType == "" {
		return writeValidationError(c, "workflow create requires a 'type' arg")
	}
	summary := args["summary"]
	if summary == "" {
		return writeValidationError(c, "workflow create requires a 'summary' arg")
	}
	description := args["description"]
	assign := args["assign"]
	priority := args["priority"]
	labels := args["labels"]
	parent := args["parent"]

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

	if c.DryRun {
		bodyPreview, _ := marshalNoEscape(map[string]any{"fields": fields})
		out, _ := marshalNoEscape(map[string]any{
			"method": "POST",
			"url":    c.BaseURL + "/rest/api/3/issue",
			"body":   json.RawMessage(bodyPreview),
		})
		return c.WriteOutput(out)
	}

	// Resolve assignee if provided.
	if assign != "" {
		accountID, isUnassign, code := resolveAssignee(ctx, c, assign)
		if code != jrerrors.ExitOK {
			return code
		}
		if !isUnassign {
			fields["assignee"] = map[string]string{"accountId": accountID}
		}
	}

	createBody, _ := json.Marshal(map[string]any{"fields": fields})
	respBody, code := c.Fetch(ctx, "POST", "/rest/api/3/issue", bytes.NewReader(createBody))
	if code != jrerrors.ExitOK {
		return code
	}

	return c.WriteOutput(respBody)
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

	if code := doLink(cmd.Context(), c, from, to, linkType); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doLink is the shared core for workflow link, used by both the Cobra RunE
// handler and the batch executor.
func doLink(ctx context.Context, c *client.Client, from, to, linkType string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + "/rest/api/3/issueLink",
			"note":   fmt.Sprintf("would link %s -> %s with type %q (link type ID resolved at runtime)", from, to, linkType),
		})
		return c.WriteOutput(out)
	}

	resolvedType, code := resolveLinkType(ctx, c, linkType)
	if code != jrerrors.ExitOK {
		return code
	}

	linkBody, _ := json.Marshal(map[string]any{
		"type":         map[string]string{"name": resolvedType},
		"inwardIssue":  map[string]string{"key": to},
		"outwardIssue": map[string]string{"key": from},
	})
	_, code = c.Fetch(ctx, "POST", "/rest/api/3/issueLink", bytes.NewReader(linkBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "linked",
		"from":   from,
		"to":     to,
		"type":   resolvedType,
	})
	return c.WriteOutput(out)
}

func runLogWork(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	timeStr, _ := cmd.Flags().GetString("time")
	comment, _ := cmd.Flags().GetString("comment")

	if code := doLogWork(cmd.Context(), c, issueKey, timeStr, comment); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doLogWork is the shared core for workflow log-work, used by both the Cobra
// RunE handler and the batch executor.
func doLogWork(ctx context.Context, c *client.Client, issueKey, timeStr, comment string) int {
	seconds, parseErr := duration.Parse(timeStr)
	if parseErr != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   "invalid duration: " + parseErr.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return jrerrors.ExitValidation
	}

	if c.DryRun {
		out, _ := marshalNoEscape(map[string]any{
			"method":           "POST",
			"url":              c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/worklog", url.PathEscape(issueKey)),
			"timeSpentSeconds": seconds,
		})
		return c.WriteOutput(out)
	}

	worklogBody := map[string]any{
		"timeSpentSeconds": seconds,
	}
	if comment != "" {
		worklogBody["comment"] = adf.FromText(comment)
	}

	body, _ := json.Marshal(worklogBody)
	_, code := c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/worklog", url.PathEscape(issueKey)),
		bytes.NewReader(body))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "logged",
		"issue":  issueKey,
		"time":   timeStr,
	})
	return c.WriteOutput(out)
}

// sprintInfo holds a matched sprint's ID and name.
type sprintInfo struct {
	ID   int
	Name string
}

// resolveSprint fetches all boards and their sprints, then matches one by name.
// Exact case-insensitive match is tried first, then substring match.
// Returns the matched sprint or writes an error to c.Stderr and returns a non-zero exit code.
func resolveSprint(ctx context.Context, c *client.Client, sprintName string) (*sprintInfo, int) {
	boardsBody, exitCode := c.Fetch(ctx, "GET", "/rest/agile/1.0/board", nil)
	if exitCode != jrerrors.ExitOK {
		return nil, exitCode
	}

	var boardsResp struct {
		Values []struct {
			ID int `json:"id"`
		} `json:"values"`
	}
	if err := json.Unmarshal(boardsBody, &boardsResp); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "connection_error",
			Message:   "failed to parse boards response: " + err.Error(),
		}
		apiErr.WriteJSON(c.Stderr)
		return nil, jrerrors.ExitError
	}

	toLower := strings.ToLower(sprintName)
	var exactMatch *sprintInfo
	var substringMatch *sprintInfo
	var allNames []string

	for _, board := range boardsResp.Values {
		sprintsBody, code := c.Fetch(ctx, "GET",
			fmt.Sprintf("/rest/agile/1.0/board/%d/sprint?state=active,future", board.ID), nil)
		if code != jrerrors.ExitOK {
			// Skip boards that fail (e.g. board type doesn't support sprints).
			continue
		}

		var sprintsResp struct {
			Values []struct {
				ID    int    `json:"id"`
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"values"`
		}
		if err := json.Unmarshal(sprintsBody, &sprintsResp); err != nil {
			continue
		}

		for _, s := range sprintsResp.Values {
			allNames = append(allNames, s.Name)
			sLower := strings.ToLower(s.Name)
			if exactMatch == nil && sLower == toLower {
				match := &sprintInfo{ID: s.ID, Name: s.Name}
				exactMatch = match
			}
			if substringMatch == nil && strings.Contains(sLower, toLower) {
				match := &sprintInfo{ID: s.ID, Name: s.Name}
				substringMatch = match
			}
		}
	}

	if exactMatch != nil {
		return exactMatch, jrerrors.ExitOK
	}
	if substringMatch != nil {
		return substringMatch, jrerrors.ExitOK
	}

	apiErr := &jrerrors.APIError{
		ErrorType: "not_found",
		Message:   fmt.Sprintf("no sprint matching %q; available: %s", sprintName, strings.Join(allNames, ", ")),
	}
	apiErr.WriteJSON(c.Stderr)
	return nil, jrerrors.ExitNotFound
}

func runSprint(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	issueKey, _ := cmd.Flags().GetString("issue")
	sprintName, _ := cmd.Flags().GetString("to")

	if code := doSprint(cmd.Context(), c, issueKey, sprintName); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doSprint is the shared core for workflow sprint, used by both the Cobra
// RunE handler and the batch executor.
func doSprint(ctx context.Context, c *client.Client, issueKey, sprintName string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + "/rest/agile/1.0/sprint/{sprintId}/issue",
			"note":   fmt.Sprintf("would move %s to sprint %q (sprint ID resolved at runtime)", issueKey, sprintName),
		})
		return c.WriteOutput(out)
	}

	sprint, code := resolveSprint(ctx, c, sprintName)
	if code != jrerrors.ExitOK {
		return code
	}

	sprintBody, _ := json.Marshal(map[string]any{"issues": []string{issueKey}})
	_, code = c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue", sprint.ID),
		bytes.NewReader(sprintBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "sprint_set",
		"issue":  issueKey,
		"sprint": sprint.Name,
	})
	return c.WriteOutput(out)
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

	if code := doComment(cmd.Context(), c, issueKey, text); code != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: code}
	}
	return nil
}

// doComment is the shared core for workflow comment, used by both the Cobra
// RunE handler and the batch executor.
func doComment(ctx context.Context, c *client.Client, issueKey, text string) int {
	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "POST",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s/comment", url.PathEscape(issueKey)),
			"note":   fmt.Sprintf("would add comment to %s", issueKey),
		})
		return c.WriteOutput(out)
	}

	commentBody, _ := json.Marshal(map[string]any{"body": adf.FromText(text)})
	_, code := c.Fetch(ctx, "POST",
		fmt.Sprintf("/rest/api/3/issue/%s/comment", url.PathEscape(issueKey)),
		bytes.NewReader(commentBody))
	if code != jrerrors.ExitOK {
		return code
	}

	out, _ := marshalNoEscape(map[string]string{
		"status": "commented",
		"issue":  issueKey,
	})
	return c.WriteOutput(out)
}
