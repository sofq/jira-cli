package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/sofq/jira-cli/internal/adf"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/jq"
	tmpl "github.com/sofq/jira-cli/internal/template"
	"github.com/spf13/cobra"
	"github.com/tidwall/pretty"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage and apply issue creation templates",
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available templates (builtin + user-defined)",
	RunE:  runTemplateList,
}

var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a template's full definition including variables and fields",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateShow,
}

var templateApplyCmd = &cobra.Command{
	Use:   "apply <name>",
	Short: "Create an issue from a template with variable substitution",
	Long:  "Applies a template to create a Jira issue. Use --project to specify the project and --var key=value for variable substitution.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateApply,
}

var templateCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a user-defined template (optionally from an existing issue)",
	Long:  "Creates a new template. Use --from to reverse-engineer a template from an existing issue's fields.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateCreate,
}

func init() {
	templateApplyCmd.Flags().String("project", "", "project key (e.g. PROJ)")
	_ = templateApplyCmd.MarkFlagRequired("project")
	templateApplyCmd.Flags().StringArray("var", nil, "variable value in key=value format (repeatable)")
	templateApplyCmd.Flags().String("assign", "", "assignee: display name, email, 'me', 'none', or 'unassign'")

	templateCreateCmd.Flags().String("from", "", "existing issue key to create template from (e.g. PROJ-123)")
	templateCreateCmd.Flags().Bool("overwrite", false, "overwrite existing template with the same name")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateApplyCmd)
	templateCmd.AddCommand(templateCreateCmd)
	rootCmd.AddCommand(templateCmd)
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	data, err := tmpl.List()
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   "failed to list templates: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	jqFilter, _ := cmd.Flags().GetString("jq")
	prettyFlag, _ := cmd.Flags().GetBool("pretty")

	if jqFilter != "" {
		filtered, err := jq.Apply(data, jqFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		data = filtered
	}

	if prettyFlag {
		data = pretty.Pretty(data)
	}

	fmt.Fprintf(os.Stdout, "%s\n", strings.TrimRight(string(data), "\n"))
	return nil
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	t, ok, err := tmpl.Lookup(name)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   "failed to load templates: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}
	if !ok {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("template %q not found; use `jr template list` to see available templates", name),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	data, _ := marshalNoEscape(t)

	jqFilter, _ := cmd.Flags().GetString("jq")
	prettyFlag, _ := cmd.Flags().GetBool("pretty")

	if jqFilter != "" {
		filtered, err := jq.Apply(data, jqFilter)
		if err != nil {
			apiErr := &jrerrors.APIError{
				ErrorType: "jq_error",
				Message:   "jq: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
		}
		data = filtered
	}

	if prettyFlag {
		data = pretty.Pretty(data)
	}

	fmt.Fprintf(os.Stdout, "%s\n", strings.TrimRight(string(data), "\n"))
	return nil
}

// parseVars parses --var key=value flags into a map.
func parseVars(varFlags []string) (map[string]string, error) {
	vars := make(map[string]string, len(varFlags))
	for _, v := range varFlags {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid --var format %q; expected key=value", v)
		}
		vars[parts[0]] = parts[1]
	}
	return vars, nil
}

func runTemplateApply(cmd *cobra.Command, args []string) error {
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	templateName := args[0]
	project, _ := cmd.Flags().GetString("project")
	varFlags, _ := cmd.Flags().GetStringArray("var")
	assign, _ := cmd.Flags().GetString("assign")

	t, ok, err := tmpl.Lookup(templateName)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "config_error",
			Message:   "failed to load templates: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}
	if !ok {
		apiErr := &jrerrors.APIError{
			ErrorType: "not_found",
			Message:   fmt.Sprintf("template %q not found; use `jr template list` to see available templates", templateName),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitNotFound}
	}

	vars, err := parseVars(varFlags)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	rendered, err := tmpl.RenderFields(t, vars)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// Build Jira fields from rendered template.
	fields := map[string]any{
		"project":   map[string]string{"key": project},
		"issuetype": map[string]string{"name": t.IssueType},
	}

	if v, ok := rendered["summary"]; ok {
		fields["summary"] = v
	}
	if v, ok := rendered["description"]; ok {
		fields["description"] = adf.FromText(v)
	}
	if v, ok := rendered["priority"]; ok {
		fields["priority"] = map[string]string{"name": v}
	}
	if v, ok := rendered["labels"]; ok {
		fields["labels"] = strings.Split(v, ",")
	}
	// Handle parent from template variables (e.g. subtask template).
	if v, ok := vars["parent"]; ok && v != "" {
		fields["parent"] = map[string]string{"key": v}
	}

	// Dry-run support.
	if c.DryRun {
		bodyPreview, _ := marshalNoEscape(map[string]any{"fields": fields})
		out, _ := marshalNoEscape(map[string]any{
			"method":   "POST",
			"url":      c.BaseURL + "/rest/api/3/issue",
			"template": templateName,
			"body":     json.RawMessage(bodyPreview),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	// Resolve assignee if provided (from flag or template variable).
	if assign == "" {
		if v, ok := vars["assignee"]; ok {
			assign = v
		}
	}
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

func runTemplateCreate(cmd *cobra.Command, args []string) error {
	templateName := args[0]
	fromIssue, _ := cmd.Flags().GetString("from")
	overwrite, _ := cmd.Flags().GetBool("overwrite")

	if err := tmpl.ValidateName(templateName); err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "validation_error",
			Message:   err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	if fromIssue == "" {
		// Create an empty template scaffold.
		t := &tmpl.Template{
			Name:        templateName,
			IssueType:   "Task",
			Description: "Custom template",
			Variables: []tmpl.Variable{
				{Name: "summary", Required: true, Description: "Issue summary"},
			},
			Fields: map[string]string{
				"summary": "{{.summary}}",
			},
		}

		path, err := tmpl.Save(t, overwrite)
		if err != nil {
			exitCode := jrerrors.ExitError
			errType := "config_error"
			if strings.Contains(err.Error(), "already exists") {
				exitCode = jrerrors.ExitConflict
				errType = "conflict"
			}
			apiErr := &jrerrors.APIError{
				ErrorType: errType,
				Message:   "failed to save template: " + err.Error(),
			}
			apiErr.WriteJSON(os.Stderr)
			return &jrerrors.AlreadyWrittenError{Code: exitCode}
		}

		out, _ := marshalNoEscape(map[string]string{
			"status":   "created",
			"template": templateName,
			"path":     path,
		})
		fmt.Fprintf(os.Stdout, "%s\n", out)
		return nil
	}

	// Create template from existing issue.
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	if c.DryRun {
		out, _ := marshalNoEscape(map[string]string{
			"method": "GET",
			"url":    c.BaseURL + fmt.Sprintf("/rest/api/3/issue/%s", url.PathEscape(fromIssue)),
			"note":   fmt.Sprintf("would fetch %s to create template %q", fromIssue, templateName),
		})
		if code := c.WriteOutput(out); code != jrerrors.ExitOK {
			return &jrerrors.AlreadyWrittenError{Code: code}
		}
		return nil
	}

	body, exitCode := c.Fetch(cmd.Context(), "GET",
		fmt.Sprintf("/rest/api/3/issue/%s", url.PathEscape(fromIssue)), nil)
	if exitCode != jrerrors.ExitOK {
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	t, err := templateFromIssue(templateName, body)
	if err != nil {
		apiErr := &jrerrors.APIError{
			ErrorType: "server_error",
			Message:   "failed to parse issue response: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	path, err := tmpl.Save(t, overwrite)
	if err != nil {
		exitCode := jrerrors.ExitError
		errType := "config_error"
		if strings.Contains(err.Error(), "already exists") {
			exitCode = jrerrors.ExitConflict
			errType = "conflict"
		}
		apiErr := &jrerrors.APIError{
			ErrorType: errType,
			Message:   "failed to save template: " + err.Error(),
		}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}

	out, _ := marshalNoEscape(map[string]string{
		"status":   "created",
		"template": templateName,
		"from":     fromIssue,
		"path":     path,
	})
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}

// templateFromIssue extracts a template structure from a Jira issue's JSON response.
func templateFromIssue(name string, issueJSON []byte) (*tmpl.Template, error) {
	var issue struct {
		Fields struct {
			Summary   string `json:"summary"`
			IssueType struct {
				Name string `json:"name"`
			} `json:"issuetype"`
			Description json.RawMessage `json:"description"`
			Priority    struct {
				Name string `json:"name"`
			} `json:"priority"`
			Labels []string `json:"labels"`
			Parent struct {
				Key string `json:"key"`
			} `json:"parent"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(issueJSON, &issue); err != nil {
		return nil, err
	}

	t := &tmpl.Template{
		Name:        name,
		IssueType:   issue.Fields.IssueType.Name,
		Description: fmt.Sprintf("Template created from issue (type: %s)", issue.Fields.IssueType.Name),
		Variables: []tmpl.Variable{
			{Name: "summary", Required: true, Description: "Issue summary"},
		},
		Fields: map[string]string{
			"summary": "{{.summary}}",
		},
	}

	if issue.Fields.Description != nil && string(issue.Fields.Description) != "null" {
		t.Variables = append(t.Variables, tmpl.Variable{
			Name:        "description",
			Description: "Issue description",
		})
		t.Fields["description"] = "{{.description}}"
	}

	if issue.Fields.Priority.Name != "" {
		t.Variables = append(t.Variables, tmpl.Variable{
			Name:        "priority",
			Default:     issue.Fields.Priority.Name,
			Description: "Priority name",
		})
		t.Fields["priority"] = "{{.priority}}"
	}

	if len(issue.Fields.Labels) > 0 {
		t.Variables = append(t.Variables, tmpl.Variable{
			Name:        "labels",
			Default:     strings.Join(issue.Fields.Labels, ","),
			Description: "Comma-separated labels",
		})
		t.Fields["labels"] = "{{.labels}}"
	}

	if issue.Fields.Parent.Key != "" {
		t.Variables = append(t.Variables, tmpl.Variable{
			Name:        "parent",
			Description: "Parent issue key",
		})
		t.Fields["parent"] = "{{.parent}}"
	}

	return t, nil
}
