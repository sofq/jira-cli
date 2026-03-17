package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"unicode"
)

// goKeywords is the set of Go reserved words that cannot be used as identifiers.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// toGoIdentifier replaces -, . with _ and filters out non-identifier characters.
// Appends _ suffix if the result is a Go keyword.
func toGoIdentifier(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	var b strings.Builder
	for i, r := range s {
		if unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if goKeywords[result] {
		result += "_"
	}
	return result
}

// resourceVarName returns the cobra var name for a resource command.
func resourceVarName(resource string) string {
	return toGoIdentifier(resource) + "Cmd"
}

// opVarName returns the cobra var name for an operation command.
func opVarName(resource, verb string) string {
	return toGoIdentifier(resource) + "_" + toGoIdentifier(verb)
}

// buildPathTemplate converts {param} placeholders to %s and builds the
// corresponding url.PathEscape argument list.
func buildPathTemplate(path string) (tmpl, args string) {
	var result strings.Builder
	var argParts []string

	i := 0
	for i < len(path) {
		if path[i] == '{' {
			end := strings.Index(path[i:], "}")
			if end == -1 {
				result.WriteByte(path[i])
				i++
				continue
			}
			paramName := path[i+1 : i+end]
			result.WriteString("%s")
			goName := toGoIdentifier(paramName)
			argParts = append(argParts, ", url.PathEscape("+goName+")")
			i += end + 1
		} else {
			result.WriteByte(path[i])
			i++
		}
	}

	return result.String(), strings.Join(argParts, "")
}

// TemplateParam holds flag metadata for a single parameter.
type TemplateParam struct {
	Name        string
	GoName      string
	Description string
	CmdVarName  string
}

// TemplateOp holds all data needed to render one operation in the resource template.
type TemplateOp struct {
	VarName         string
	Verb            string
	Method          string
	Summary         string
	Description     string
	PathTemplate    string
	PathArgs        string
	PathParams      []TemplateParam
	QueryParams     []TemplateParam
	QueryFlagNames  string
	HasBody         bool
	ResourceVarName string
}

// TemplateResource holds all data for the resource.go.tmpl template.
type TemplateResource struct {
	ResourceName    string
	ResourceVarName string
	Operations      []TemplateOp
}

// SchemaFlagData holds flag data for schema_data.go.tmpl.
type SchemaFlagData struct {
	Name        string
	Required    bool
	Type        string
	Description string
	In          string
}

// SchemaOpData holds operation data for schema_data.go.tmpl.
type SchemaOpData struct {
	Resource string
	Verb     string
	Method   string
	Path     string
	Summary  string
	HasBody  bool
	Flags    []SchemaFlagData
}

// SchemaData holds all data for schema_data.go.tmpl.
type SchemaData struct {
	Operations []SchemaOpData
	Resources  []string
}

// ResourceEntry holds data for a single resource in init.go.tmpl.
type ResourceEntry struct {
	VarName string
}

// InitData holds all data for init.go.tmpl.
type InitData struct {
	Resources []ResourceEntry
}

// loadTemplateFn is the function used to load templates. It can be overridden
// in tests to inject errors or custom template content.
var loadTemplateFn = loadTemplateDefault

// loadTemplate loads a template by name via loadTemplateFn.
func loadTemplate(name string) (string, error) {
	return loadTemplateFn(name)
}

// loadTemplateDefault searches gen/templates/ relative to the source file
// location, then falls back to templates/ in the cwd.
func loadTemplateDefault(name string) (string, error) {
	// Try relative to this source file (runtime caller).
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(filename)
		p := filepath.Join(dir, "templates", name)
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}

	// Try relative to cwd.
	for _, base := range []string{"gen/templates", "templates"} {
		p := filepath.Join(base, name)
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}

	return "", fmt.Errorf("template %q not found", name)
}

// renderTemplate parses and executes a named template with the given data,
// then runs gofmt on the result.
func renderTemplate(name string, data any) ([]byte, error) {
	src, err := loadTemplate(name)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(name).Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parsing template %q: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing template %q: %w", name, err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Return unformatted with error context for debugging.
		return buf.Bytes(), fmt.Errorf("formatting generated code for %q: %w", name, err)
	}
	return formatted, nil
}

// GenerateResource generates a Go source file for one resource group.
// deduplicateVerbs ensures no two operations in the same resource have the same verb.
// When collisions happen, the operationId is used as-is (kebab-cased) for disambiguation.
func deduplicateVerbs(ops []Operation, resource string) []string {
	verbs := make([]string, len(ops))
	verbCount := map[string]int{}
	for i, op := range ops {
		verbs[i] = DeriveVerb(op.OperationID, op.Method, op.Path, resource)
		verbCount[verbs[i]]++
	}
	// For any duplicates, use the full operationId (kebab-cased, resource stripped)
	for i, op := range ops {
		if verbCount[verbs[i]] > 1 && op.OperationID != "" {
			// Use full operationId converted to kebab-case
			words := splitCamelCase(op.OperationID)
			parts := make([]string, len(words))
			for j, w := range words {
				parts[j] = strings.ToLower(w)
			}
			verbs[i] = strings.Join(parts, "-")
		}
	}
	return verbs
}

func GenerateResource(resource string, ops []Operation, outDir string) error {
	rvn := resourceVarName(resource)
	verbs := deduplicateVerbs(ops, resource)

	var templateOps []TemplateOp
	for i, op := range ops {
		verb := verbs[i]
		vn := opVarName(resource, verb)
		pathTmpl, pathArgs := buildPathTemplate(op.Path)

		var pathParams []TemplateParam
		for _, p := range op.PathParams {
			pathParams = append(pathParams, TemplateParam{
				Name:        p.Name,
				GoName:      toGoIdentifier(p.Name),
				Description: p.Description,
				CmdVarName:  vn,
			})
		}

		var queryParams []TemplateParam
		var queryFlagParts []string
		for _, p := range op.QueryParams {
			queryParams = append(queryParams, TemplateParam{
				Name:        p.Name,
				GoName:      toGoIdentifier(p.Name),
				Description: p.Description,
				CmdVarName:  vn,
			})
			queryFlagParts = append(queryFlagParts, fmt.Sprintf("%q", p.Name))
		}

		queryFlagNames := ""
		if len(queryFlagParts) > 0 {
			queryFlagNames = ", " + strings.Join(queryFlagParts, ", ")
		}

		templateOps = append(templateOps, TemplateOp{
			VarName:         vn,
			Verb:            verb,
			Method:          op.Method,
			Summary:         op.Summary,
			Description:     op.Description,
			PathTemplate:    pathTmpl,
			PathArgs:        pathArgs,
			PathParams:      pathParams,
			QueryParams:     queryParams,
			QueryFlagNames:  queryFlagNames,
			HasBody:         op.HasBody,
			ResourceVarName: rvn,
		})
	}

	data := TemplateResource{
		ResourceName:    resource,
		ResourceVarName: rvn,
		Operations:      templateOps,
	}

	out, err := renderTemplate("resource.go.tmpl", data)
	if err != nil {
		return fmt.Errorf("generating resource %q: %w", resource, err)
	}

	outPath := filepath.Join(outDir, toGoIdentifier(resource)+".go")
	return os.WriteFile(outPath, out, 0o644)
}

// GenerateSchemaData generates the schema_data.go file.
func GenerateSchemaData(groups map[string][]Operation, resources []string, outDir string) error {
	var schemaOps []SchemaOpData
	for _, resource := range resources {
		ops := groups[resource]
		verbs := deduplicateVerbs(ops, resource)
		for i, op := range ops {
			verb := verbs[i]
			var flags []SchemaFlagData
			for _, p := range op.PathParams {
				flags = append(flags, SchemaFlagData{
					Name:        p.Name,
					Required:    true,
					Type:        p.Type,
					Description: p.Description,
					In:          "path",
				})
			}
			for _, p := range op.QueryParams {
				flags = append(flags, SchemaFlagData{
					Name:        p.Name,
					Required:    p.Required,
					Type:        p.Type,
					Description: p.Description,
					In:          "query",
				})
			}
			if op.HasBody {
				flags = append(flags, SchemaFlagData{
					Name:        "body",
					Required:    false,
					Type:        "string",
					Description: "request body (JSON string, @file, or - for stdin)",
					In:          "body",
				})
			}
			schemaOps = append(schemaOps, SchemaOpData{
				Resource: resource,
				Verb:     verb,
				Method:   op.Method,
				Path:     op.Path,
				Summary:  op.Summary,
				HasBody:  op.HasBody,
				Flags:    flags,
			})
		}
	}

	data := SchemaData{
		Operations: schemaOps,
		Resources:  resources,
	}

	out, err := renderTemplate("schema_data.go.tmpl", data)
	if err != nil {
		return fmt.Errorf("generating schema data: %w", err)
	}

	outPath := filepath.Join(outDir, "schema_data.go")
	return os.WriteFile(outPath, out, 0o644)
}

// GenerateInit generates the init.go file that registers all resource commands.
func GenerateInit(resources []string, outDir string) error {
	var entries []ResourceEntry
	for _, r := range resources {
		entries = append(entries, ResourceEntry{
			VarName: resourceVarName(r),
		})
	}

	data := InitData{Resources: entries}

	out, err := renderTemplate("init.go.tmpl", data)
	if err != nil {
		return fmt.Errorf("generating init: %w", err)
	}

	outPath := filepath.Join(outDir, "init.go")
	return os.WriteFile(outPath, out, 0o644)
}
