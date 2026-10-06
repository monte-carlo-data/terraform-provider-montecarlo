package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A connection has one parent, a warehouse or a BI container, and the other parent id is null
// for its whole life. A push-only ETL container and a warehouse with no deployment have the same
// null deployment_id. Renaming one of them replaced it: the hold left the null unknown, and
// requiresReplace read null to unknown as a change. These run the generated schema's own
// modifiers, in the order it attaches them, the way the framework runs them.
func TestAParentThatStaysNullIsHeldAndAChangedParentStillReplaces(t *testing.T) {
	for _, tc := range []struct {
		newResource func() resource.Resource
		attribute   string
	}{
		{NewConnectionResource, "bi_container_id"},
		{NewConnectionResource, "warehouse_id"},
		{NewEtlContainerResource, "deployment_id"},
		{NewWarehouseResource, "deployment_id"},
	} {
		var resp resource.SchemaResponse
		tc.newResource().Schema(t.Context(), resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("reported %v", resp.Diagnostics.Errors())
		}
		attr, ok := resp.Schema.Attributes[tc.attribute]
		if !ok {
			t.Fatalf("the schema has no %s attribute", tc.attribute)
		}

		t.Run(tc.attribute+"/a rename keeps the null and updates in place", func(t *testing.T) {
			planned, replace := chainStringModifiers(t, attr, planmodifier.StringRequest{
				State:       tfsdk.State{Schema: stringResourceSchema(tc.attribute), Raw: resourceRaw(tc.attribute, nil)},
				Plan:        tfsdk.Plan{Schema: stringResourceSchema(tc.attribute), Raw: resourceRaw(tc.attribute, tftypes.UnknownValue)},
				StateValue:  types.StringNull(),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			})
			if !planned.Equal(types.StringNull()) {
				t.Errorf("planned %s, want null", planned)
			}
			if replace {
				t.Error("RequiresReplace = true, so renaming the resource destroys it")
			}
		})

		t.Run(tc.attribute+"/a changed parent still replaces", func(t *testing.T) {
			planned, replace := chainStringModifiers(t, attr, planmodifier.StringRequest{
				State:       tfsdk.State{Schema: stringResourceSchema(tc.attribute), Raw: resourceRaw(tc.attribute, "old-parent")},
				Plan:        tfsdk.Plan{Schema: stringResourceSchema(tc.attribute), Raw: resourceRaw(tc.attribute, "new-parent")},
				StateValue:  types.StringValue("old-parent"),
				PlanValue:   types.StringValue("new-parent"),
				ConfigValue: types.StringValue("new-parent"),
			})
			if !planned.Equal(types.StringValue("new-parent")) {
				t.Errorf("planned %s, want new-parent", planned)
			}
			if !replace {
				t.Error("RequiresReplace = false, but the API takes the parent only on create")
			}
		})
	}
}
