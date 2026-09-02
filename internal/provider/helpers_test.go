package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Every attribute type the generated schemas can contain. A type missing from requiresReplace
// is reported at runtime rather than dropped, but the error only surfaces once someone plans
// against that resource, so the coverage is pinned here instead.
func TestRequiresReplaceCoversEveryAttributeType(t *testing.T) {
	elem := types.StringType
	for name, attribute := range map[string]schema.Attribute{
		"bool":          schema.BoolAttribute{},
		"string":        schema.StringAttribute{},
		"int32":         schema.Int32Attribute{},
		"int64":         schema.Int64Attribute{},
		"float32":       schema.Float32Attribute{},
		"float64":       schema.Float64Attribute{},
		"number":        schema.NumberAttribute{},
		"dynamic":       schema.DynamicAttribute{},
		"list":          schema.ListAttribute{ElementType: elem},
		"map":           schema.MapAttribute{ElementType: elem},
		"set":           schema.SetAttribute{ElementType: elem},
		"object":        schema.ObjectAttribute{AttributeTypes: map[string]attr.Type{}},
		"single_nested": schema.SingleNestedAttribute{},
		"list_nested":   schema.ListNestedAttribute{},
		"set_nested":    schema.SetNestedAttribute{},
		"map_nested":    schema.MapNestedAttribute{},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := map[string]schema.Attribute{name: attribute}
			var diags diag.Diagnostics

			requiresReplace(attrs, "resource."+name, &diags)

			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}
			if got := planModifierCount(attrs[name]); got != 1 {
				t.Fatalf("got %d plan modifiers, want 1", got)
			}
		})
	}
}

func TestRequiresReplaceReportsAnAttributeTheSchemaDoesNotHave(t *testing.T) {
	var diags diag.Diagnostics

	requiresReplace(map[string]schema.Attribute{}, "resource.renamed", &diags)

	if !diags.HasError() {
		t.Fatal("a marked attribute absent from the schema must not pass in silence")
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "resource.renamed") {
		t.Fatalf("the error must name the attribute, got %q", detail)
	}
}

// planModifierCount reads the modifier slice back off an attribute, whose accessor is typed.
func planModifierCount(a schema.Attribute) int {
	switch a := a.(type) {
	case schema.BoolAttribute:
		return len(a.PlanModifiers)
	case schema.StringAttribute:
		return len(a.PlanModifiers)
	case schema.Int32Attribute:
		return len(a.PlanModifiers)
	case schema.Int64Attribute:
		return len(a.PlanModifiers)
	case schema.Float32Attribute:
		return len(a.PlanModifiers)
	case schema.Float64Attribute:
		return len(a.PlanModifiers)
	case schema.NumberAttribute:
		return len(a.PlanModifiers)
	case schema.DynamicAttribute:
		return len(a.PlanModifiers)
	case schema.ListAttribute:
		return len(a.PlanModifiers)
	case schema.MapAttribute:
		return len(a.PlanModifiers)
	case schema.SetAttribute:
		return len(a.PlanModifiers)
	case schema.ObjectAttribute:
		return len(a.PlanModifiers)
	case schema.SingleNestedAttribute:
		return len(a.PlanModifiers)
	case schema.ListNestedAttribute:
		return len(a.PlanModifiers)
	case schema.SetNestedAttribute:
		return len(a.PlanModifiers)
	case schema.MapNestedAttribute:
		return len(a.PlanModifiers)
	}
	return 0
}
