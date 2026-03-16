package main

import (
	"strings"
	"unicode"
)

// GroupOperations groups operations by resource name extracted from their path.
func GroupOperations(ops []Operation) map[string][]Operation {
	result := make(map[string][]Operation)
	for _, op := range ops {
		resource := ExtractResource(op.Path)
		result[resource] = append(result[resource], op)
	}
	return result
}

// ExtractResource extracts the resource name from a path.
// For /rest/api/3/issue/{id} → "issue"
// For /rest/atlassian-connect/1/... → "atlassian-connect"
// Fallback: first non-empty path segment.
func ExtractResource(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Try to match /rest/api/3/<resource>
	if len(segments) >= 4 && segments[0] == "rest" && segments[1] == "api" {
		return segments[3]
	}

	// Try to match /rest/<product>/<version>/... e.g. /rest/atlassian-connect/1/...
	if len(segments) >= 2 && segments[0] == "rest" {
		return segments[1]
	}

	// Fallback: first non-empty segment
	for _, s := range segments {
		if s != "" {
			return s
		}
	}

	return path
}

// DeriveVerb derives a CLI verb from an operation's metadata.
func DeriveVerb(operationID, method, path, resource string) string {
	words := splitCamelCase(operationID)
	if len(words) == 0 {
		return strings.ToLower(method)
	}

	verb := strings.ToLower(words[0])
	rest := words[1:]

	if len(rest) == 0 {
		return verb
	}

	// Normalize resource: "project" might match "Projects", etc.
	// Build a lowercase version of rest
	restLower := make([]string, len(rest))
	for i, w := range rest {
		restLower[i] = strings.ToLower(w)
	}

	// Singularize resource for comparison (simple: strip trailing 's')
	resourceLower := strings.ToLower(resource)
	resourceSingular := singularize(resourceLower)

	// Singularize rest words for comparison
	restSingular := make([]string, len(restLower))
	for i, w := range restLower {
		restSingular[i] = singularize(w)
	}

	// Case 1: rest == resource (e.g., "getIssue" → rest=["issue"], resource="issue" → "get")
	if len(rest) == 1 && (restSingular[0] == resourceSingular || restLower[0] == resourceLower) {
		return verb
	}

	// Case 2: rest ENDS with resource name → strip it, keep verb + prefix
	// e.g., "getAllProjects" → rest=["All","Projects"], resource="project"
	// → strip "Projects" → prefix = ["All"] → "get-all"
	if singularize(restSingular[len(rest)-1]) == resourceSingular || restSingular[len(rest)-1] == resourceSingular {
		prefix := restLower[:len(rest)-1]
		if len(prefix) == 0 {
			return verb
		}
		return verb + "-" + strings.Join(prefix, "-")
	}

	// Case 3: rest STARTS with resource name → strip it, keep verb + suffix
	// e.g., "getIssueTransitions" → rest=["Issue","Transitions"], resource="issue"
	// → strip "Issue" → suffix = ["transitions"] → "get-transitions"
	if restSingular[0] == resourceSingular || restLower[0] == resourceLower {
		// len(rest)==1 is already handled by Case 1 (identical condition), so suffix is never empty here.
		return verb + "-" + strings.Join(restLower[1:], "-")
	}

	// Fallback: verb + kebab-joined rest
	return verb + "-" + strings.Join(restLower, "-")
}

// singularizeExceptions maps words whose singular form does not follow simple stripping rules.
var singularizeExceptions = map[string]string{
	"statuses":  "status",
	"status":    "status",
	"class":     "class",
	"classes":   "class",
	"address":   "address",
	"addresses": "address",
	"bus":       "bus",
	"buses":     "bus",
}

// singularize does a simple singularization with an exceptions map for known irregular words.
func singularize(s string) string {
	if exc, ok := singularizeExceptions[s]; ok {
		return exc
	}
	if strings.HasSuffix(s, "sses") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") && !strings.HasSuffix(s, "us") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
}

// splitCamelCase splits a camelCase string into words.
// e.g., "getIssueTransitions" → ["get", "Issue", "Transitions"]
func splitCamelCase(s string) []string {
	if s == "" {
		return nil
	}

	var words []string
	var current strings.Builder

	runes := []rune(s)
	for i, r := range runes {
		if i == 0 {
			current.WriteRune(r)
			continue
		}
		if unicode.IsUpper(r) {
			// Check if previous char was lower (standard camel boundary)
			// or next char is lower (handles "HTMLParser" → "HTML", "Parser")
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				words = append(words, current.String())
				current.Reset()
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				// e.g., "HTMLParser": at 'P', prev='L' (upper), next='a' (lower)
				if current.Len() > 0 {
					words = append(words, current.String())
					current.Reset()
				}
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}
