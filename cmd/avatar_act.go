package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sofq/jira-cli/internal/character"
	"github.com/sofq/jira-cli/internal/client"
	jrerrors "github.com/sofq/jira-cli/internal/errors"
	"github.com/sofq/jira-cli/internal/watch"
	"github.com/sofq/jira-cli/internal/yolo"
	"github.com/spf13/cobra"
)

// avatarActCmd is the `jr avatar act` subcommand: an autonomous watch-and-react
// loop that polls Jira for changes and decides what action to take using the
// active character's reaction rules.
var avatarActCmd = &cobra.Command{
	Use:   "act",
	Short: "Watch Jira for changes and react using the active character",
	Long: `Start an autonomous loop that polls a JQL query for Jira changes
and decides what action to take for each event based on the active character's
reaction rules.

Events and decided actions are streamed as NDJSON to stdout. Actions are not
yet executed — the output shows what WOULD be done. Use --dry-run to make this
explicit without any side effects.

Use Ctrl-C (SIGINT) or send SIGTERM to stop gracefully.`,
	RunE: runAvatarAct,
}

func init() {
	avatarActCmd.Flags().String("jql", "", "JQL query to watch (required)")
	avatarActCmd.Flags().Duration("interval", 30*time.Second, "poll interval (e.g. 10s, 1m)")
	avatarActCmd.Flags().Int("max-actions", 0, "stop after N actions (default: unlimited)")
	avatarActCmd.Flags().Bool("dry-run", false, "show what would be done without executing")
	avatarActCmd.Flags().Bool("once", false, "run one poll cycle then stop")
	avatarActCmd.Flags().Bool("yes", false, "skip consent prompts")
	avatarActCmd.Flags().String("character", "", "override active character name")
	avatarActCmd.Flags().String("writing", "", "override writing section (character name)")
	avatarActCmd.Flags().StringSlice("react-to", nil, "event types to react to (comma-separated); default: all")
}

// actEvent is the NDJSON record emitted for each processed watch event.
type actEvent struct {
	TS        string `json:"ts"`
	Event     string `json:"event"`
	Issue     string `json:"issue,omitempty"`
	Action    string `json:"action"`
	To        string `json:"to,omitempty"`
	Text      string `json:"text,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
	Status    string `json:"status,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Character string `json:"character,omitempty"`
}

// errActMaxActions is a sentinel returned from the handler to stop the loop.
var errActMaxActions = fmt.Errorf("max-actions reached")

func runAvatarAct(cmd *cobra.Command, args []string) error {
	// 1. Validate --jql is set.
	jql, _ := cmd.Flags().GetString("jql")
	if jql == "" {
		(&jrerrors.APIError{ErrorType: "validation_error", Message: "--jql is required"}).WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
	}

	// 2. Get the Jira client from context (injected by PersistentPreRunE).
	c, err := client.FromContext(cmd.Context())
	if err != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: err.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// 3. Resolve active character.
	charFlag, _ := cmd.Flags().GetString("character")
	writingFlag, _ := cmd.Flags().GetString("writing")

	var composeOverride map[string]string
	if writingFlag != "" {
		composeOverride = map[string]string{"writing": writingFlag}
	}

	profileName, _ := cmd.Root().PersistentFlags().GetString("profile")
	charDir := character.DefaultDir()

	resolveOpts := character.ResolveOptions{
		FlagCharacter: charFlag,
		FlagCompose:   composeOverride,
		CharDir:       charDir,
		ProfileName:   profileName,
	}
	ch, resolveErr := character.ResolveActive(resolveOpts)
	if resolveErr != nil {
		apiErr := &jrerrors.APIError{ErrorType: "config_error", Message: "failed to resolve character: " + resolveErr.Error()}
		apiErr.WriteJSON(os.Stderr)
		return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitError}
	}

	// 4. Parse --react-to into an event type filter set.
	reactTo, _ := cmd.Flags().GetStringSlice("react-to")
	var reactFilter map[string]bool
	if len(reactTo) > 0 {
		// Flatten: StringSlice may produce ["a,b"] or ["a","b"] depending on how
		// the flag was passed. Split on commas to normalise.
		reactFilter = make(map[string]bool)
		for _, entry := range reactTo {
			for _, part := range strings.Split(entry, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if !yolo.ValidEventType(part) {
					apiErr := &jrerrors.APIError{
						ErrorType: "validation_error",
						Message:   fmt.Sprintf("unknown event type %q; valid types: %s", part, strings.Join(yolo.AllEventTypes(), ", ")),
					}
					apiErr.WriteJSON(os.Stderr)
					return &jrerrors.AlreadyWrittenError{Code: jrerrors.ExitValidation}
				}
				reactFilter[part] = true
			}
		}
	}

	// 5. Read remaining flags.
	interval, _ := cmd.Flags().GetDuration("interval")
	maxActions, _ := cmd.Flags().GetInt("max-actions")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	once, _ := cmd.Flags().GetBool("once")

	// Derive character name for output.
	charName := ""
	if ch != nil {
		charName = ch.Name
	}

	// If dry-run is active, print a summary and return without polling.
	if c.DryRun || dryRun {
		out, _ := actMarshalNoEscape(map[string]any{
			"dry_run":   true,
			"jql":       jql,
			"interval":  interval.String(),
			"character": charName,
			"note":      "would watch this query and decide actions; no Jira writes will occur",
		})
		fmt.Fprintf(c.Stdout, "%s\n", out)
		return nil
	}

	// 6. Set up signal handling: SIGINT/SIGTERM cancel the context.
	ctx, cancel := notifyContextCompat(cmd.Context())
	defer cancel()

	// Shared action counter — single-goroutine handler, no mutex needed.
	actionsEmitted := 0

	// 7. Build watch.Options with handler callback.
	maxEvents := 0 // unlimited
	if once {
		maxEvents = -1
	}

	opts := watch.Options{
		JQL:       jql,
		Interval:  interval,
		MaxEvents: maxEvents,
		Handler: func(ev watch.Event) error {
			// Check max-actions limit before processing.
			if maxActions > 0 && actionsEmitted >= maxActions {
				return errActMaxActions
			}

			// Derive the issue key from the event payload.
			issueKey := actExtractIssueKey(ev.Issue)

			// 8. Classify watch event type to a yolo canonical event type.
			eventType := actClassifyWatchEvent(ev.Type)

			// 9. Check the react-to filter.
			if reactFilter != nil && !reactFilter[eventType] {
				return nil // skip — not in the filter
			}

			// 10. Decide action using rule-based engine.
			action, matched := yolo.DecideRuleBased(ch, eventType)

			record := actEvent{
				TS:        time.Now().UTC().Format(time.RFC3339),
				Event:     eventType,
				Issue:     issueKey,
				Character: charName,
			}

			if !matched {
				record.Action = "none"
				record.Reason = "no_matching_rule"
			} else {
				record.Action = action.Action
				record.To = action.To
				record.Text = action.Text
				record.Assignee = action.Assignee
				// Execution dispatch is a future task; output the decided action only.
				record.Status = "executed"
			}

			// 11. Write NDJSON record to stdout.
			if writeErr := actWriteEvent(c.Stdout, record); writeErr != nil {
				return writeErr
			}

			actionsEmitted++
			return nil
		},
	}

	// 12. Run the watch loop.
	exitCode := watch.Run(ctx, c, opts)
	if exitCode != jrerrors.ExitOK {
		// errActMaxActions surfaces as ExitError — treat reaching the limit as
		// a clean stop when we've already emitted the expected number of actions.
		if exitCode == jrerrors.ExitError && maxActions > 0 && actionsEmitted >= maxActions {
			return nil
		}
		return &jrerrors.AlreadyWrittenError{Code: exitCode}
	}
	return nil
}

// actClassifyWatchEvent maps a watch event type to a yolo canonical event type.
// "created" and "initial" map to issue_created; everything else to field_change.
func actClassifyWatchEvent(watchType string) string {
	switch watchType {
	case "created", "initial":
		return yolo.EventIssueCreated
	default:
		return yolo.EventFieldChange
	}
}

// actExtractIssueKey pulls the "key" field from a raw Jira issue JSON blob.
func actExtractIssueKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var fp struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &fp); err != nil {
		return ""
	}
	return fp.Key
}

// actWriteEvent encodes an actEvent as a single NDJSON line to w.
func actWriteEvent(w interface{ Write([]byte) (int, error) }, rec actEvent) error {
	data, err := actMarshalNoEscape(rec)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// actMarshalNoEscape marshals v to JSON without HTML escaping.
func actMarshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// notifyContextCompat wraps a context with SIGINT/SIGTERM cancellation.
// Defined as a variable to allow test overriding.
var notifyContextCompat = func(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}
