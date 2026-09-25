package provider

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// planModifierHelper is one helper that attaches a plan modifier, together with the two strings
// that say which one it attached: the framework's own description of the modifier, and a
// fragment of the consequence the helper passes into its diagnostics. A count of attached
// modifiers cannot tell two same-typed modifiers apart; the description can.
type planModifierHelper struct {
	apply       func(map[string]schema.Attribute, string, *diag.Diagnostics)
	description string
	consequence string
}

// Every helper that attaches a plan modifier. Each one goes through the same type switch, so a
// type missing from it is missing from all of them.
var planModifierHelpers = map[string]planModifierHelper{
	"requiresReplace": {
		apply:       requiresReplace,
		description: "If the value of this attribute changes, Terraform will destroy and recreate the resource.",
		consequence: "in-place update the API does not implement",
	},
	"useNonNullStateForUnknown": {
		apply:       useNonNullStateForUnknown,
		description: "Once set to a non-null value, the value of this attribute in state will not change.",
		consequence: "plan the attribute as unknown on every update",
	},
	"useStateForUnknown": {
		apply:       useStateForUnknown,
		description: "Once set, the value of this attribute in state will not change.",
		consequence: "replace the resource when it also requires replacement",
	},
}

// everyAttributeType is one of each type the generated schemas can contain.
func everyAttributeType() map[string]schema.Attribute {
	elem := types.StringType
	return map[string]schema.Attribute{
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
	}
}

// unhandledAttribute is an attribute type applyPlanModifier's switch does not know, standing in
// for a seventeenth framework type or a custom-typed attribute from the generator. It embeds
// StringAttribute only to inherit the schema.Attribute method set, whose Equal takes a type
// from an internal framework package and so cannot be implemented from here. A type switch
// matches dynamic types exactly, so this still lands in the default arm.
type unhandledAttribute struct{ schema.StringAttribute }

// An unhandled type or an absent attribute is reported at runtime rather than dropped, but the
// error only surfaces once someone plans against that resource, so the coverage is pinned here.
func TestEveryHelperAttachesItsOwnModifierToTheNamedAttributeOnly(t *testing.T) {
	for helper, h := range planModifierHelpers {
		for name := range everyAttributeType() {
			t.Run(helper+"/"+name, func(t *testing.T) {
				// The whole table, not a single-entry map: selecting the named attribute out of
				// its fifteen siblings is part of what is being tested.
				attrs := everyAttributeType()
				var diags diag.Diagnostics

				h.apply(attrs, "resource."+name, &diags)

				if diags.HasError() {
					t.Fatalf("reported %v", diags.Errors())
				}
				mods := planModifiers(attrs[name])
				if len(mods) != 1 {
					t.Fatalf("got %d plan modifiers, want 1", len(mods))
				}
				// A planModifierSet literal that omits a field supplies a nil interface there,
				// which counts as an attached modifier and panics the provider when the
				// framework calls it.
				if mods[0] == nil {
					t.Fatal("the attached plan modifier is nil, which panics at plan time")
				}
				// The description is what distinguishes a modifier from its near-namesake —
				// UseNonNullStateForUnknown from UseStateForUnknown — which a count cannot. One
				// wrong line in a planModifierSet literal is otherwise invisible.
				if got := mods[0].Description(t.Context()); got != h.description {
					t.Errorf("attached %q, want %q", got, h.description)
				}
				for sibling := range attrs {
					if sibling == name {
						continue
					}
					if got := len(planModifiers(attrs[sibling])); got != 0 {
						t.Errorf("%s gained %d plan modifiers, want 0", sibling, got)
					}
				}
			})
		}
	}
}

func TestEveryHelperReportsAnAttributeTheSchemaDoesNotHave(t *testing.T) {
	for helper, h := range planModifierHelpers {
		t.Run(helper, func(t *testing.T) {
			var diags diag.Diagnostics

			h.apply(map[string]schema.Attribute{}, "resource.renamed", &diags)

			if !diags.HasError() {
				t.Fatal("a marked attribute absent from the schema must not pass in silence")
			}
			detail := diags.Errors()[0].Detail()
			if !strings.Contains(detail, "resource.renamed") {
				t.Errorf("the error must name the attribute, got %q", detail)
			}
			// The consequence argument exists so the diagnostic names the effect rather than
			// only the missing case; nothing else checks it is passed through.
			if !strings.Contains(detail, h.consequence) {
				t.Errorf("the error must state the consequence %q, got %q", h.consequence, detail)
			}
		})
	}
}

// The arm that fires when the framework grows a seventeenth attribute type, or the generator
// starts emitting a custom-typed one — the case the design says must not be silent.
func TestEveryHelperReportsAnAttributeTypeItHasNoCaseFor(t *testing.T) {
	for helper, h := range planModifierHelpers {
		t.Run(helper, func(t *testing.T) {
			attrs := map[string]schema.Attribute{"custom": unhandledAttribute{}}
			var diags diag.Diagnostics

			h.apply(attrs, "resource.custom", &diags)

			if !diags.HasError() {
				t.Fatal("an attribute type the switch does not handle must not pass in silence")
			}
			detail := diags.Errors()[0].Detail()
			for _, want := range []string{"resource.custom", "unhandledAttribute", h.consequence} {
				if !strings.Contains(detail, want) {
					t.Errorf("the error must mention %q, got %q", want, detail)
				}
			}
		})
	}
}

// A planModifierSet is a plain struct, so a literal that omits a field supplies a nil interface
// there rather than failing to compile. Appending it would attach a modifier the framework
// dereferences during `terraform plan` — a plugin crash rather than a diagnostic.
func TestApplyPlanModifierReportsASetWithNoModifierForTheMatchedType(t *testing.T) {
	attrs := map[string]schema.Attribute{"flag": schema.BoolAttribute{}}
	var diags diag.Diagnostics

	applyPlanModifier(
		attrs,
		"resource.flag",
		planModifierSet{str: stringplanmodifier.RequiresReplace()},
		"the consequence",
		&diags,
	)

	if !diags.HasError() {
		t.Fatal("a set with no modifier for the matched type must be reported, not attached")
	}
	if got := len(planModifiers(attrs["flag"])); got != 0 {
		t.Errorf("attached %d plan modifiers, want 0 — a nil modifier panics at plan time", got)
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "the consequence") {
		t.Errorf("the error must state the consequence, got %q", detail)
	}
}

// The null-state case is the whole reason for the non-null variant: an attribute the API fills
// in later has nothing to keep on the first plan and must stay unknown, or Terraform rejects
// the apply as an inconsistent result. `deployment.created_time` is a tagged example.
//
// req.State carries a real resource value in every case. The plain UseStateForUnknown gates on
// req.State.Raw rather than on req.StateValue, so against an unpopulated request it returns
// early everywhere and this test would pass for it too — proving nothing about the null-copy
// semantics the design rests on.
func TestUseNonNullStateForUnknownKeepsStateOnlyOnceThereIsAValue(t *testing.T) {
	const attribute = "external_id"
	for name, tc := range map[string]struct {
		stateValue  types.String
		configValue types.String
		raw         tftypes.Value
		want        types.String
	}{
		"a value in state is planned": {
			stateValue:  types.StringValue("abc"),
			configValue: types.StringNull(),
			raw:         resourceRaw(attribute, "abc"),
			want:        types.StringValue("abc"),
		},
		"a null state stays unknown": {
			stateValue:  types.StringNull(),
			configValue: types.StringNull(),
			raw:         resourceRaw(attribute, nil),
			want:        types.StringUnknown(),
		},
		// An unknown configuration value must be left alone, or interpolation of the reference
		// that made it unknown breaks. Both variants bail here.
		"an unknown config value stays unknown": {
			stateValue:  types.StringValue("abc"),
			configValue: types.StringUnknown(),
			raw:         resourceRaw(attribute, "abc"),
			want:        types.StringUnknown(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := map[string]schema.Attribute{attribute: schema.StringAttribute{Computed: true}}
			var diags diag.Diagnostics
			useNonNullStateForUnknown(attrs, "resource."+attribute, &diags)
			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}

			resp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
			attachedStringModifier(t, attrs[attribute]).PlanModifyString(t.Context(), planmodifier.StringRequest{
				State:       tfsdk.State{Schema: stringResourceSchema(attribute), Raw: tc.raw},
				Plan:        tfsdk.Plan{Schema: stringResourceSchema(attribute), Raw: tc.raw},
				StateValue:  tc.stateValue,
				PlanValue:   types.StringUnknown(),
				ConfigValue: tc.configValue,
			}, resp)

			if !resp.PlanValue.Equal(tc.want) {
				t.Fatalf("planned %s, want %s", resp.PlanValue, tc.want)
			}
		})
	}
}

// The same invariant for the object field, which is what a SingleNestedAttribute gets — the
// Azure credential blocks. The framework documents the non-null variant as the one to use for a
// child of a nested attribute that can be null after create, so this is where substituting the
// plain variant does the most damage and where a modifier count proves the least.
func TestUseNonNullStateForUnknownKeepsANestedObjectOnlyOnceItHasAValue(t *testing.T) {
	const attribute = "service_principal"
	nested := map[string]attr.Type{"client_id": types.StringType}
	nestedRaw := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"client_id": tftypes.String}}
	populated := types.ObjectValueMust(nested, map[string]attr.Value{"client_id": types.StringValue("abc")})

	for name, tc := range map[string]struct {
		stateValue types.Object
		raw        tftypes.Value
		want       types.Object
	}{
		"a value in state is planned": {
			stateValue: populated,
			raw: tftypes.NewValue(nestedRaw, map[string]tftypes.Value{
				"client_id": tftypes.NewValue(tftypes.String, "abc"),
			}),
			want: populated,
		},
		"a null state stays unknown": {
			stateValue: types.ObjectNull(nested),
			raw:        tftypes.NewValue(nestedRaw, nil),
			want:       types.ObjectUnknown(nested),
		},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := map[string]schema.Attribute{attribute: schema.SingleNestedAttribute{
				Computed:   true,
				Attributes: map[string]schema.Attribute{"client_id": schema.StringAttribute{Computed: true}},
			}}
			var diags diag.Diagnostics
			useNonNullStateForUnknown(attrs, "resource."+attribute, &diags)
			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}

			mods := attrs[attribute].(schema.SingleNestedAttribute).PlanModifiers
			if len(mods) != 1 {
				t.Fatalf("got %d plan modifiers, want 1", len(mods))
			}
			resourceValue := tftypes.NewValue(
				tftypes.Object{AttributeTypes: map[string]tftypes.Type{attribute: nestedRaw}},
				map[string]tftypes.Value{attribute: tc.raw},
			)
			state := tfsdk.State{Schema: schema.Schema{Attributes: attrs}, Raw: resourceValue}
			resp := &planmodifier.ObjectResponse{PlanValue: types.ObjectUnknown(nested)}
			mods[0].PlanModifyObject(t.Context(), planmodifier.ObjectRequest{
				State:       state,
				Plan:        tfsdk.Plan{Schema: schema.Schema{Attributes: attrs}, Raw: resourceValue},
				StateValue:  tc.stateValue,
				PlanValue:   types.ObjectUnknown(nested),
				ConfigValue: types.ObjectNull(nested),
			}, resp)

			if !resp.PlanValue.Equal(tc.want) {
				t.Fatalf("planned %s, want %s", resp.PlanValue, tc.want)
			}
		})
	}
}

// Over-marking costs as much as under-marking: replacement must happen when an update changes
// the value, and must not happen on create, on destroy, or when the value is unchanged — those
// would destroy a resource the API would have updated in place. A count of attached modifiers
// cannot tell RequiresReplace from any other string modifier.
func TestRequiresReplaceReplacesOnlyOnAnUpdateThatChangesTheValue(t *testing.T) {
	const attribute = "runtime_platform"
	for name, tc := range map[string]struct {
		state       tftypes.Value
		plan        tftypes.Value
		stateValue  types.String
		planValue   types.String
		wantReplace bool
	}{
		"an update that changes the value replaces": {
			state:       resourceRaw(attribute, "linux"),
			plan:        resourceRaw(attribute, "windows"),
			stateValue:  types.StringValue("linux"),
			planValue:   types.StringValue("windows"),
			wantReplace: true,
		},
		"an update that keeps the value does not": {
			state:      resourceRaw(attribute, "linux"),
			plan:       resourceRaw(attribute, "linux"),
			stateValue: types.StringValue("linux"),
			planValue:  types.StringValue("linux"),
		},
		// No prior state: the resource is being created, so there is nothing to replace.
		"create does not replace": {
			state:      nullResourceRaw(attribute),
			plan:       resourceRaw(attribute, "linux"),
			stateValue: types.StringNull(),
			planValue:  types.StringValue("linux"),
		},
		// No planned resource: it is being destroyed, which replacement cannot improve on.
		"destroy does not replace": {
			state:      resourceRaw(attribute, "linux"),
			plan:       nullResourceRaw(attribute),
			stateValue: types.StringValue("linux"),
			planValue:  types.StringNull(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := map[string]schema.Attribute{attribute: schema.StringAttribute{Required: true}}
			var diags diag.Diagnostics
			requiresReplace(attrs, "resource."+attribute, &diags)
			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}

			resp := &planmodifier.StringResponse{PlanValue: tc.planValue}
			attachedStringModifier(t, attrs[attribute]).PlanModifyString(t.Context(), planmodifier.StringRequest{
				State:       tfsdk.State{Schema: stringResourceSchema(attribute), Raw: tc.state},
				Plan:        tfsdk.Plan{Schema: stringResourceSchema(attribute), Raw: tc.plan},
				StateValue:  tc.stateValue,
				PlanValue:   tc.planValue,
				ConfigValue: tc.planValue,
			}, resp)

			if resp.RequiresReplace != tc.wantReplace {
				t.Errorf("RequiresReplace = %t, want %t", resp.RequiresReplace, tc.wantReplace)
			}
		})
	}
}

// An unreturned input is null in state once the config omits it, and the framework re-plans it
// unknown whenever the resource changes at all. requiresReplace reads that unknown as a change,
// so before the null was held a warehouse rename destroyed the warehouse and every connection on
// it. Both modifiers are attached here in the order the generated schema attaches them, and run
// the way the framework runs them: each one handed the value the one before it planned.
func TestAnUnreturnedAttributeThatIsNullInStateIsPlannedInPlace(t *testing.T) {
	const attribute = "connection_type"
	for name, tc := range map[string]struct {
		state       tftypes.Value
		plan        tftypes.Value
		stateValue  types.String
		planValue   types.String
		configValue types.String
		want        types.String
		wantReplace bool
	}{
		// The rename: the config changes another attribute and leaves this one out.
		"an omitted unreturned attribute keeps its null": {
			state:       resourceRaw(attribute, nil),
			plan:        resourceRaw(attribute, tftypes.UnknownValue),
			stateValue:  types.StringNull(),
			planValue:   types.StringUnknown(),
			configValue: types.StringNull(),
			want:        types.StringNull(),
		},
		// Held or not, a value the customer actually changes has to replace: the API will not
		// accept it on an update.
		"a configured unreturned attribute that changes still replaces": {
			state:       resourceRaw(attribute, "presto"),
			plan:        resourceRaw(attribute, "trino"),
			stateValue:  types.StringValue("presto"),
			planValue:   types.StringValue("trino"),
			configValue: types.StringValue("trino"),
			want:        types.StringValue("trino"),
			wantReplace: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := map[string]schema.Attribute{
				attribute: schema.StringAttribute{Optional: true, Computed: true},
			}
			var diags diag.Diagnostics
			useStateForUnknown(attrs, "resource."+attribute, &diags)
			requiresReplace(attrs, "resource."+attribute, &diags)
			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}

			planned, replace := chainStringModifiers(t, attrs[attribute], planmodifier.StringRequest{
				State:       tfsdk.State{Schema: stringResourceSchema(attribute), Raw: tc.state},
				Plan:        tfsdk.Plan{Schema: stringResourceSchema(attribute), Raw: tc.plan},
				StateValue:  tc.stateValue,
				PlanValue:   tc.planValue,
				ConfigValue: tc.configValue,
			})

			if !planned.Equal(tc.want) {
				t.Errorf("planned %s, want %s", planned, tc.want)
			}
			if replace != tc.wantReplace {
				t.Errorf("RequiresReplace = %t, want %t", replace, tc.wantReplace)
			}
		})
	}
}

// The order the two modifiers are attached in is load-bearing, and it is decided in api-codegen's
// template rather than here. Attached after the replacement, the hold runs too late: the
// replacement has already compared an unknown plan value against the null in state.
func TestHoldingAnUnreturnedValueAfterTheReplacementComesTooLate(t *testing.T) {
	const attribute = "connection_type"
	attrs := map[string]schema.Attribute{
		attribute: schema.StringAttribute{Optional: true, Computed: true},
	}
	var diags diag.Diagnostics
	requiresReplace(attrs, "resource."+attribute, &diags)
	useStateForUnknown(attrs, "resource."+attribute, &diags)
	if diags.HasError() {
		t.Fatalf("reported %v", diags.Errors())
	}

	planned, replace := chainStringModifiers(t, attrs[attribute], planmodifier.StringRequest{
		State:       tfsdk.State{Schema: stringResourceSchema(attribute), Raw: resourceRaw(attribute, nil)},
		Plan:        tfsdk.Plan{Schema: stringResourceSchema(attribute), Raw: resourceRaw(attribute, tftypes.UnknownValue)},
		StateValue:  types.StringNull(),
		PlanValue:   types.StringUnknown(),
		ConfigValue: types.StringNull(),
	})

	if !planned.Equal(types.StringNull()) {
		t.Errorf("planned %s, want null", planned)
	}
	if !replace {
		t.Error("RequiresReplace = false, so this order is safe and the generated one need not be")
	}
}

// chainStringModifiers runs every modifier attached to a string attribute the way the framework
// runs them, handing each the value the one before it planned and keeping any replacement.
func chainStringModifiers(t *testing.T, a schema.Attribute, req planmodifier.StringRequest) (types.String, bool) {
	t.Helper()
	mods := a.(schema.StringAttribute).PlanModifiers
	if len(mods) != 2 {
		t.Fatalf("got %d plan modifiers, want 2", len(mods))
	}
	planned, replace := req.PlanValue, false
	for _, mod := range mods {
		req.PlanValue = planned
		resp := &planmodifier.StringResponse{PlanValue: planned}
		mod.PlanModifyString(t.Context(), req, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("reported %v", resp.Diagnostics.Errors())
		}
		planned, replace = resp.PlanValue, replace || resp.RequiresReplace
	}
	return planned, replace
}

// Every name the generated resource schemas pass to the helpers has to resolve against the
// generated attribute maps. Nothing else checks that: the two sides are joined by strings
// derived in another repo, so a renamed attribute compiles here and surfaces first as a
// "provider bug" diagnostic on a practitioner's plan. Reading the registration list means new
// resources are covered without touching this test.
//
// It also runs the framework's schema validation, which otherwise runs only in a provider server:
// that is what rejects a write-only attribute under a Computed nested attribute.
func TestEveryGeneratedResourceSchemaIsValidAndResolvesTheNamesTheHelpersAreGiven(t *testing.T) {
	ctx := t.Context()
	resources := (&mcProvider{}).Resources(ctx)
	if len(resources) == 0 {
		t.Fatal("the provider registers no resources, so this proves nothing")
	}

	for _, newResource := range resources {
		r := newResource()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "montecarlo"}, &meta)

		t.Run(meta.TypeName, func(t *testing.T) {
			var resp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &resp)
			resp.Diagnostics.Append(resp.Schema.ValidateImplementation(ctx)...)

			if resp.Diagnostics.HasError() {
				t.Fatalf("reported %v", resp.Diagnostics.Errors())
			}
			if len(resp.Schema.Attributes) == 0 {
				t.Fatal("the schema has no attributes, so no name was resolved against it")
			}
		})
	}
}

// credentialAttributes is shaped like a generated resource with secrets: top-level and nested,
// string and map. Fresh per call, since writeOnly edits in place.
func credentialAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"private_key_wo": schema.StringAttribute{Required: true, Sensitive: true},
		"name":           schema.StringAttribute{Required: true},
		"credentials": schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"client_secret_wo": schema.StringAttribute{Required: true, Sensitive: true},
				"headers_wo":       schema.MapAttribute{Required: true, Sensitive: true, ElementType: types.StringType},
				"client_id":        schema.StringAttribute{Required: true},
			},
		},
	}
}

// attributeAt reads a dotted path back through the original map, which catches a leaf flag set on
// a copy and never written back.
func attributeAt(t *testing.T, attrs map[string]schema.Attribute, path string) schema.Attribute {
	t.Helper()
	segments := strings.Split(path, ".")
	for _, segment := range segments[:len(segments)-1] {
		attrs = attrs[segment].(schema.SingleNestedAttribute).Attributes
	}
	return attrs[segments[len(segments)-1]]
}

// Marking anything beyond the secret would hide a value Terraform needs to diff.
func TestWriteOnlyMarksTheNamedSecretAtAnyDepthAndNothingElse(t *testing.T) {
	everyPath := []string{
		"private_key_wo", "name", "credentials",
		"credentials.client_secret_wo", "credentials.headers_wo", "credentials.client_id",
	}
	for _, target := range []string{"private_key_wo", "credentials.client_secret_wo", "credentials.headers_wo"} {
		t.Run(target, func(t *testing.T) {
			attrs := credentialAttributes()
			var diags diag.Diagnostics

			writeOnly(attrs, "resource."+target, &diags)

			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}
			for _, path := range everyPath {
				got := attributeAt(t, attrs, path).IsWriteOnly()
				if want := path == target; got != want {
					t.Errorf("%s: WriteOnly = %t, want %t", path, got, want)
				}
			}
			block := attrs["credentials"].(schema.SingleNestedAttribute)
			if !block.Optional || block.Computed {
				t.Errorf("the credential block's flags changed: Optional = %t, Computed = %t", block.Optional, block.Computed)
			}
			if !attributeAt(t, attrs, target).IsSensitive() {
				t.Error("the secret lost Sensitive")
			}
		})
	}
}

// An unresolved secret would stay an ordinary attribute, stored in state.
func TestWriteOnlyReportsAPathItCannotResolve(t *testing.T) {
	for name, path := range map[string]string{
		"no resource prefix":                 "private_key_wo",
		"an absent top-level attribute":      "resource.renamed_wo",
		"an absent nested attribute":         "resource.credentials.renamed_wo",
		"an absent block":                    "resource.renamed.client_secret_wo",
		"a path through a non-nested string": "resource.name.client_secret_wo",
	} {
		t.Run(name, func(t *testing.T) {
			attrs := credentialAttributes()
			var diags diag.Diagnostics

			writeOnly(attrs, path, &diags)

			if !diags.HasError() {
				t.Fatal("an unresolved path must not pass in silence")
			}
			detail := diags.Errors()[0].Detail()
			for _, want := range []string{path, "store the secret in state"} {
				if !strings.Contains(detail, want) {
					t.Errorf("the error must mention %q, got %q", want, detail)
				}
			}
		})
	}
}

// Every secret the spec declares is a string or a map; anything else is a generator change.
func TestWriteOnlyMarksOnlyStringAndMapAttributes(t *testing.T) {
	attrTypes := everyAttributeType()
	attrTypes["unhandled"] = unhandledAttribute{}

	for name, a := range attrTypes {
		t.Run(name, func(t *testing.T) {
			wantOK := name == "string" || name == "map"
			attrs := map[string]schema.Attribute{"secret_wo": a}
			var diags diag.Diagnostics

			writeOnly(attrs, "resource.secret_wo", &diags)

			if diags.HasError() == wantOK {
				t.Fatalf("HasError() = %t, want %t", diags.HasError(), !wantOK)
			}
			if got := attrs["secret_wo"].IsWriteOnly(); got != wantOK {
				t.Errorf("IsWriteOnly() = %t, want %t", got, wantOK)
			}
			if !wantOK {
				detail := diags.Errors()[0].Detail()
				for _, want := range []string{"resource.secret_wo", "store the secret in state"} {
					if !strings.Contains(detail, want) {
						t.Errorf("the error must mention %q, got %q", want, detail)
					}
				}
			}
		})
	}
}

// This is what TestWriteOnlyMarksTheNamedSecretAtAnyDepthAndNothingElse leans on: a Computed leaf
// passes schema validation, so writeOnly itself has to catch it.
func TestSchemaValidationRejectsAWriteOnlySecretUnderAComputedBlock(t *testing.T) {
	attrs := map[string]schema.Attribute{
		"credentials": schema.SingleNestedAttribute{
			Optional: true,
			Computed: true,
			Attributes: map[string]schema.Attribute{
				"secret_wo": schema.StringAttribute{Optional: true},
			},
		},
	}
	var diags diag.Diagnostics

	writeOnly(attrs, "resource.credentials.secret_wo", &diags)

	if diags.HasError() {
		t.Fatalf("reported %v", diags.Errors())
	}
	d := (schema.Schema{Attributes: attrs}).ValidateImplementation(context.Background())
	if !d.HasError() {
		t.Fatal("a write-only attribute under a Computed nested attribute must fail schema validation")
	}
}

// A Computed leaf paired with WriteOnly passes ValidateImplementation but fails at plan time on
// the practitioner's machine, so writeOnly must reject it instead of shipping an unusable resource.
func TestWriteOnlyReportsAComputedSecret(t *testing.T) {
	for name, tc := range map[string]struct {
		attrs map[string]schema.Attribute
		path  string
	}{
		"top-level string": {
			attrs: map[string]schema.Attribute{"secret_wo": schema.StringAttribute{Optional: true, Computed: true}},
			path:  "resource.secret_wo",
		},
		"nested string": {
			attrs: map[string]schema.Attribute{
				"credentials": schema.SingleNestedAttribute{
					Optional: true,
					Attributes: map[string]schema.Attribute{
						"secret_wo": schema.StringAttribute{Optional: true, Computed: true},
					},
				},
			},
			path: "resource.credentials.secret_wo",
		},
		"nested map": {
			attrs: map[string]schema.Attribute{
				"credentials": schema.SingleNestedAttribute{
					Optional: true,
					Attributes: map[string]schema.Attribute{
						"secret_wo": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType},
					},
				},
			},
			path: "resource.credentials.secret_wo",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics

			writeOnly(tc.attrs, tc.path, &diags)

			if !diags.HasError() {
				t.Fatal("a Computed secret must not pass in silence")
			}
			detail := diags.Errors()[0].Detail()
			for _, want := range []string{tc.path, "store the secret in state"} {
				if !strings.Contains(detail, want) {
					t.Errorf("the error must mention %q, got %q", want, detail)
				}
			}
			_, resolved, _ := strings.Cut(tc.path, ".")
			if attributeAt(t, tc.attrs, resolved).IsWriteOnly() {
				t.Error("the Computed attribute was marked anyway")
			}
		})
	}
}

// stringResourceSchema is a resource schema of one string attribute, enough to give a plan
// modifier request the populated State and Plan its early returns read.
func stringResourceSchema(attribute string) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{attribute: schema.StringAttribute{Computed: true}},
	}
}

// resourceRaw is the whole-resource value a plan modifier request carries in its State and Plan
// fields, holding one string attribute; a nil value makes that attribute null. Both modifiers
// branch on whether the whole-resource value is null, and a zero-value tftypes.Value reads as
// null, so a request that leaves State or Plan unset exercises an early return rather than the
// behaviour under test.
func resourceRaw(attribute string, value any) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{attribute: tftypes.String}},
		map[string]tftypes.Value{attribute: tftypes.NewValue(tftypes.String, value)},
	)
}

// nullResourceRaw is the whole-resource value for a resource that does not exist: a null State
// is a create, a null Plan is a destroy.
func nullResourceRaw(attribute string) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{attribute: tftypes.String}},
		nil,
	)
}

// attachedStringModifier is the single modifier a helper attached to a string attribute.
func attachedStringModifier(t *testing.T, a schema.Attribute) planmodifier.String {
	t.Helper()
	mods := a.(schema.StringAttribute).PlanModifiers
	if len(mods) != 1 {
		t.Fatalf("got %d plan modifiers, want 1", len(mods))
	}
	return mods[0]
}

// planModifiers reads the modifier slice back off an attribute, whose accessor is typed, as the
// one thing every typed modifier shares: a description. That is enough to tell which modifier
// was attached, which a count is not.
func planModifiers(a schema.Attribute) []planmodifier.Describer {
	switch a := a.(type) {
	case schema.BoolAttribute:
		return describers(a.PlanModifiers)
	case schema.StringAttribute:
		return describers(a.PlanModifiers)
	case schema.Int32Attribute:
		return describers(a.PlanModifiers)
	case schema.Int64Attribute:
		return describers(a.PlanModifiers)
	case schema.Float32Attribute:
		return describers(a.PlanModifiers)
	case schema.Float64Attribute:
		return describers(a.PlanModifiers)
	case schema.NumberAttribute:
		return describers(a.PlanModifiers)
	case schema.DynamicAttribute:
		return describers(a.PlanModifiers)
	case schema.ListAttribute:
		return describers(a.PlanModifiers)
	case schema.MapAttribute:
		return describers(a.PlanModifiers)
	case schema.SetAttribute:
		return describers(a.PlanModifiers)
	case schema.ObjectAttribute:
		return describers(a.PlanModifiers)
	case schema.SingleNestedAttribute:
		return describers(a.PlanModifiers)
	case schema.ListNestedAttribute:
		return describers(a.PlanModifiers)
	case schema.SetNestedAttribute:
		return describers(a.PlanModifiers)
	case schema.MapNestedAttribute:
		return describers(a.PlanModifiers)
	}
	return nil
}

// describers widens a typed modifier slice. A nil element widens to a nil Describer rather than
// being hidden, which is the point: that is the shape a planModifierSet with a missing field
// produces.
func describers[T planmodifier.Describer](mods []T) []planmodifier.Describer {
	out := make([]planmodifier.Describer, 0, len(mods))
	for _, m := range mods {
		out = append(out, m)
	}
	return out
}

func TestRetryableStatus(t *testing.T) {
	for status, want := range map[int]bool{
		http.StatusServiceUnavailable: true,
		http.StatusTooManyRequests:    true,
		// Ambiguous: the request may have been carried out. Retrying a create would strand a
		// second deployment.
		http.StatusGatewayTimeout:      false,
		http.StatusRequestTimeout:      false,
		http.StatusConflict:            false,
		http.StatusUnprocessableEntity: false,
		http.StatusInternalServerError: false,
		http.StatusForbidden:           false,
	} {
		if got := retryableStatus(status); got != want {
			t.Errorf("retryableStatus(%d) = %t, want %t", status, got, want)
		}
	}
}

func TestRetryAfterIsHonouredWhenTheAPISendsIt(t *testing.T) {
	for name, tc := range map[string]struct {
		header string
		want   time.Duration
	}{
		"seconds":           {header: "30", want: 30 * time.Second},
		"surrounding space": {header: " 30 ", want: 30 * time.Second},
		// A legal value, and what a server produces when it truncates a sub-second window. It
		// is floored rather than honoured: waiting none of it would re-issue the request as
		// fast as the network allows for the whole budget.
		"zero is floored, not honoured at once": {header: "0", want: retryAfterFloor},
		// Falling back rather than guessing. The API sends seconds.
		"absent":   {header: "", want: transientRetryInterval},
		"a date":   {header: "Wed, 21 Oct 2026 07:28:00 GMT", want: transientRetryInterval},
		"nonsense": {header: "soon", want: transientRetryInterval},
		"negative": {header: "-5", want: transientRetryInterval},
	} {
		t.Run(name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tc.header != "" {
				resp.Header.Set("Retry-After", tc.header)
			}

			if got := retryAfter(resp); got != tc.want {
				t.Errorf("retryAfter(%q) = %s, want %s", tc.header, got, tc.want)
			}
		})
	}
}

// The wait is server-controlled, so it has to stay inside the budget whatever arrives. Without
// a floor a zero busy-loops the retry; without an upper bound a value at or above ~9.22e9
// seconds overflows time.Duration's int64 nanoseconds and wraps negative, which defeats the
// deadline guard in withRetryOnTransient and lets the loop run without end.
func TestRetryAfterStaysWithinTheRetryBudget(t *testing.T) {
	for name, seconds := range map[string]string{
		"zero must not busy-loop":                  "0",
		"ten billion seconds overflows":            "10000000000",
		"max int64 seconds overflows":              "9223372036854775807",
		"just above the int64-nanosecond boundary": "18446744073",
	} {
		t.Run(name, func(t *testing.T) {
			got := retryAfter(rateLimited(seconds))

			if got < retryAfterFloor {
				t.Errorf("retryAfter(%q) = %s, want a floor of at least %s", seconds, got, retryAfterFloor)
			}
			if got <= 0 || got > transientRetryTimeout {
				t.Errorf("retryAfter(%q) = %s, want a positive duration no greater than the retry budget %s", seconds, got, transientRetryTimeout)
			}
		})
	}
}

// Two retries at the floored one-second wait, so this test really does sleep for about that
// long: the budget and the interval are consts, so there is nothing to shorten. Making them
// injectable is the outstanding half of F18 and needs helpers.go.
// shrinkRetryTimings makes the helper's waits negligible for a test that exercises the retry
// loop rather than the durations it waits. Restored afterwards; these tests do not run in
// parallel, so the override cannot leak into another one.
func shrinkRetryTimings(t *testing.T) {
	t.Helper()
	timeout, interval, floor := transientRetryTimeout, transientRetryInterval, retryAfterFloor
	t.Cleanup(func() {
		transientRetryTimeout, transientRetryInterval, retryAfterFloor = timeout, interval, floor
	})
	transientRetryTimeout = 2 * time.Second
	transientRetryInterval = time.Millisecond
	retryAfterFloor = time.Millisecond
}

func TestWithRetryOnTransientRetriesUntilItSucceeds(t *testing.T) {
	// Without this the two floored waits cost two real seconds, which was most of the suite.
	shrinkRetryTimings(t)

	calls := 0
	out, err := withRetryOnTransient(t.Context(), func() (*string, *http.Response, error) {
		calls++
		if calls < 3 {
			return nil, rateLimited("0"), errors.New("too many requests")
		}
		body := "done"
		return &body, &http.Response{StatusCode: http.StatusOK}, nil
	})

	if err != nil {
		t.Fatalf("returned %v", err)
	}
	if *out != "done" || calls != 3 {
		t.Errorf("got %q after %d calls, want %q after 3", *out, calls, "done")
	}
}

func TestWithRetryOnTransientReturnsAFailureItCannotRetry(t *testing.T) {
	calls := 0
	_, err := withRetryOnTransient(t.Context(), func() (*string, *http.Response, error) {
		calls++
		return nil, &http.Response{StatusCode: http.StatusUnprocessableEntity}, errors.New("rejected")
	})

	if err == nil {
		t.Fatal("a 422 must be returned, not retried")
	}
	if calls != 1 {
		t.Errorf("called %d times, want 1", calls)
	}
}

// No response at all — a dropped connection or a DNS failure. Nothing says the request was
// not carried out, so it is returned rather than repeated.
func TestWithRetryOnTransientReturnsATransportFailure(t *testing.T) {
	calls := 0
	_, err := withRetryOnTransient(t.Context(), func() (*string, *http.Response, error) {
		calls++
		return nil, nil, errors.New("connection reset")
	})

	if err == nil {
		t.Fatal("a transport failure must be returned")
	}
	if calls != 1 {
		t.Errorf("called %d times, want 1", calls)
	}
}

// The budget is the only bound on the loop — there is no attempt cap — so it decides whether a
// sustained rate limit surfaces as an error or as an apply that hangs. A wait of the whole
// budget cannot fit inside what remains of it, so the branch is reached on the first response
// and no test has to sit out five minutes to get there.
func TestWithRetryOnTransientStopsWhenTheWaitCannotFitTheBudget(t *testing.T) {
	wholeBudget := strconv.Itoa(int(transientRetryTimeout / time.Second))
	calls := 0
	started := time.Now()

	_, err := withRetryOnTransient(t.Context(), func() (*string, *http.Response, error) {
		calls++
		return nil, rateLimited(wholeBudget), errors.New("too many requests")
	})

	if err == nil {
		t.Fatal("exhausting the budget must return the last error")
	}
	if calls != 1 {
		t.Errorf("called %d times, want 1", calls)
	}
	if slept := time.Since(started); slept > time.Second {
		t.Errorf("waited %s before giving up, want no wait at all", slept)
	}
}

// Terraform relies on cancellation to honour Ctrl-C and per-operation timeouts. Without it an
// interrupted apply sits in the wait for up to the whole budget. The wait is a second and the
// context is already cancelled when the select runs, so only one case is ready.
func TestWithRetryOnTransientStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0

	_, err := withRetryOnTransient(ctx, func() (*string, *http.Response, error) {
		calls++
		cancel()
		return nil, rateLimited("1"), errors.New("too many requests")
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("returned %v, want %v", err, context.Canceled)
	}
	if calls != 1 {
		t.Errorf("called %d times, want 1", calls)
	}
}

func rateLimited(retryAfter string) *http.Response {
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	resp.Header.Set("Retry-After", retryAfter)
	return resp
}

// A known collection holding a null or unknown element fails ElementsAs rather than being
// coerced, and reflect.Into returns before assigning, so the helper's pre-allocated collection
// survives. Returning that empty collection would pass the generated caller's `!= nil` guard
// and tell the API to clear a field the practitioner never asked to clear, so an error has to
// come back as nil instead.
func TestMapOfStringsAndListOfStringsHonourTheirDocumentedContract(t *testing.T) {
	ctx := context.Background()

	t.Run("mapOfStrings", func(t *testing.T) {
		t.Run("null map is absent", func(t *testing.T) {
			if got := mapOfStrings(ctx, types.MapNull(types.StringType)); got != nil {
				t.Errorf("got %#v, want nil", got)
			}
		})
		t.Run("unknown map is absent", func(t *testing.T) {
			if got := mapOfStrings(ctx, types.MapUnknown(types.StringType)); got != nil {
				t.Errorf("got %#v, want nil", got)
			}
		})
		t.Run("a populated map converts", func(t *testing.T) {
			m := types.MapValueMust(types.StringType, map[string]attr.Value{"env": types.StringValue("prod")})
			got := mapOfStrings(ctx, m)
			want := map[string]string{"env": "prod"}
			if len(got) != len(want) || got["env"] != want["env"] {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
		t.Run("a null element must not silently produce an empty non-nil map", func(t *testing.T) {
			m := types.MapValueMust(types.StringType, map[string]attr.Value{"env": types.StringNull()})
			if got := mapOfStrings(ctx, m); got != nil {
				t.Errorf("got %#v, want nil so the caller's != nil guard omits the field instead of clearing it", got)
			}
		})
	})

	t.Run("listOfStrings", func(t *testing.T) {
		t.Run("null list is absent", func(t *testing.T) {
			if got := listOfStrings(ctx, types.ListNull(types.StringType)); got != nil {
				t.Errorf("got %#v, want nil", got)
			}
		})
		t.Run("unknown list is absent", func(t *testing.T) {
			if got := listOfStrings(ctx, types.ListUnknown(types.StringType)); got != nil {
				t.Errorf("got %#v, want nil", got)
			}
		})
		t.Run("a populated list converts", func(t *testing.T) {
			l := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a"), types.StringValue("b")})
			got := listOfStrings(ctx, l)
			want := []string{"a", "b"}
			if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
		t.Run("a null element must not silently produce an empty non-nil list", func(t *testing.T) {
			l := types.ListValueMust(types.StringType, []attr.Value{types.StringNull()})
			if got := listOfStrings(ctx, l); got != nil {
				t.Errorf("got %#v, want nil so the caller's != nil guard omits the field instead of clearing it", got)
			}
		})
	})
}

func TestStringMapAndStringListAreTheTotalInverseOfTheirWriteSideHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("stringMap", func(t *testing.T) {
		t.Run("a nil map is an empty known map, not null", func(t *testing.T) {
			got := stringMap(nil)
			if got.IsNull() || got.IsUnknown() || len(got.Elements()) != 0 {
				t.Errorf("got %#v, want an empty known map", got)
			}
		})
		t.Run("round-trips through mapOfStrings", func(t *testing.T) {
			want := map[string]string{"env": "prod", "team": "data"}
			got := mapOfStrings(ctx, stringMap(want))
			if len(got) != len(want) || got["env"] != want["env"] || got["team"] != want["team"] {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
	})

	t.Run("stringList", func(t *testing.T) {
		t.Run("a nil list is an empty known list, not null", func(t *testing.T) {
			got := stringList(nil)
			if got.IsNull() || got.IsUnknown() || len(got.Elements()) != 0 {
				t.Errorf("got %#v, want an empty known list", got)
			}
		})
		t.Run("round-trips through listOfStrings, keeping order", func(t *testing.T) {
			want := []string{"b", "a"}
			got := listOfStrings(ctx, stringList(want))
			if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
	})
}

// The last segment of a dotted name is not enough to resolve it: a nested path like
// "azure_collection_agent.service_principal.name" would find the top-level "name" and attach
// the modifier to the wrong attribute, which for requiresReplace destroys a resource the API
// would have updated. It has to be reported instead.
func TestPlanModifierHelpersRejectANestedPathInsteadOfTargetingATopLevelNamesake(t *testing.T) {
	for helper, h := range planModifierHelpers {
		t.Run(helper, func(t *testing.T) {
			attrs := map[string]schema.Attribute{"name": schema.StringAttribute{}}
			var diags diag.Diagnostics

			h.apply(attrs, "azure_collection_agent.service_principal.name", &diags)

			if !diags.HasError() {
				t.Error("a nested path must be reported, not silently resolved to a top-level namesake")
			}
			if got := len(planModifiers(attrs["name"])); got != 0 {
				t.Errorf("got %d plan modifiers on the top-level \"name\" attribute, want 0", got)
			}
		})
	}
}
