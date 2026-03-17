package main

import (
	"fmt"
	"os"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Param represents a single operation parameter.
type Param struct {
	Name        string
	In          string // path, query
	Required    bool
	Type        string // string, integer, boolean, array
	Description string
}

// Operation represents a single API operation extracted from the spec.
type Operation struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	PathParams  []Param
	QueryParams []Param
	HasBody     bool
}

// ParseSpec reads an OpenAPI spec file at path and returns all operations.
func ParseSpec(path string) ([]Operation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}

	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}

	model, errs := doc.BuildV3Model()
	if errs != nil {
		return nil, fmt.Errorf("building model: %v", errs)
	}

	if model.Model.Paths == nil {
		return nil, fmt.Errorf("no paths in spec")
	}

	var ops []Operation

	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		pathStr := pair.Key()
		pathItem := pair.Value()

		// Collect path-level parameters
		var pathLevelParams []*v3.Parameter
		if pathItem.Parameters != nil {
			pathLevelParams = pathItem.Parameters
		}

		type methodOp struct {
			method string
			op     *v3.Operation
		}

		methods := []methodOp{
			{"GET", pathItem.Get},
			{"POST", pathItem.Post},
			{"PUT", pathItem.Put},
			{"DELETE", pathItem.Delete},
			{"PATCH", pathItem.Patch},
		}

		for _, mo := range methods {
			if mo.op == nil {
				continue
			}

			op := Operation{
				OperationID: mo.op.OperationId,
				Method:      mo.method,
				Path:        pathStr,
				Summary:     mo.op.Summary,
				Description: mo.op.Description,
				HasBody:     mo.op.RequestBody != nil,
			}

			// Merge path-level and operation-level params
			var allParams []*v3.Parameter
			allParams = append(allParams, pathLevelParams...)
			if mo.op.Parameters != nil {
				allParams = append(allParams, mo.op.Parameters...)
			}

			for _, p := range allParams {
				param := Param{
					Name:        p.Name,
					In:          p.In,
					Description: p.Description,
					Type:        schemaType(p.Schema),
				}
				if p.In == "path" {
					param.Required = true
				} else if p.Required != nil {
					param.Required = *p.Required
				}

				switch p.In {
				case "path":
					op.PathParams = append(op.PathParams, param)
				case "query":
					op.QueryParams = append(op.QueryParams, param)
				}
			}

			ops = append(ops, op)
		}
	}

	return ops, nil
}

// schemaType extracts a simple type string from a SchemaProxy.
// Falls back to "string" for nil, unresolvable, or untyped schemas.
func schemaType(schema *base.SchemaProxy) string {
	if schema == nil {
		return "string"
	}
	if s := schema.Schema(); s != nil && len(s.Type) > 0 {
		return s.Type[0]
	}
	return "string"
}
