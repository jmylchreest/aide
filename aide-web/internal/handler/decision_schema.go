package handler

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// RegisterDecisionSchemas describes the JSON union without coupling the memory
// model to the HTTP framework. Reflection alone only sees the object form.
func RegisterDecisionSchemas(reg huma.Registry) {
	reg.RegisterTypeAlias(reflect.TypeFor[memory.Applicability](), reflect.TypeFor[decisionApplicability]())
}

type decisionApplicability struct{}

func (decisionApplicability) Schema(reg huma.Registry) *huma.Schema {
	type structuredApplicability memory.Applicability
	return &huma.Schema{
		OneOf: []*huma.Schema{
			{Type: "string"},
			huma.SchemaFromType(reg, reflect.TypeFor[structuredApplicability]()),
			{Type: "null"},
		},
	}
}
