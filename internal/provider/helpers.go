package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/numberplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	sdk "github.com/monte-carlo-data/mc-sdk-go/montecarlo"
)

// The retry helper's timings. Variables rather than constants only so a test exercising the
// retry loop can shrink them and not spend real seconds sleeping; they are unexported, and
// nothing in the provider writes them at runtime. Not user-configurable: the budget is a
// property of how long the API's transient failures last, not a preference.
var (
	transientRetryTimeout  = 5 * time.Minute
	transientRetryInterval = 15 * time.Second

	// retryAfterFloor is the shortest wait retryAfter will honour. `Retry-After: 0` is a legal
	// value — a server truncating a sub-second window produces it — but waiting zero seconds
	// would re-issue the request as fast as the network allows for the whole retry budget.
	retryAfterFloor = 1 * time.Second
)

// withRetryOnTransient retries while the API reports a failure it asked to have retried, up
// to a bounded budget.
//
// Provisioning reaches systems that are not immediately consistent — a freshly created role
// is not usable the instant it exists — so the API reports a transient failure rather than
// pretending. Terraform has no notion of "try again shortly", so the wait happens here.
// Exhausting the budget returns the last error to the caller.
//
// The generated code calls this for operations the spec marks as retryable.
func withRetryOnTransient[T any](
	ctx context.Context, call func() (*T, *http.Response, error),
) (*T, error) {
	deadline := time.Now().Add(transientRetryTimeout)
	for {
		out, httpResp, err := call()
		if err == nil {
			return out, nil
		}
		if httpResp == nil || !retryableStatus(httpResp.StatusCode) {
			return nil, err
		}
		wait := retryAfter(httpResp)
		// Waiting past the deadline would return the same error having slept for nothing.
		if remaining := time.Until(deadline); wait >= remaining {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// retryableStatus reports whether a status means the request was not carried out.
//
// Both of these say so, which is what makes repeating the request safe even for an operation
// that is not idempotent: 503 is Monte Carlo being unable to serve it, and 429 is the request
// being refused before it ran.
//
// 504 is deliberately absent. A gateway timeout leaves the outcome unknown, and creating a
// deployment is not idempotent — it allocates a new one per call and counts each against the
// account's limit — so retrying an ambiguous failure would strand one nothing knows about.
func retryableStatus(status int) bool {
	return status == http.StatusServiceUnavailable || status == http.StatusTooManyRequests
}

// retryAfter is how long to wait before the next attempt.
//
// The API sends `Retry-After` in seconds with a rate limit, so honouring it retries neither
// sooner than Monte Carlo asked nor a fixed interval longer than it needs. A missing or
// unparseable value falls back to the fixed interval, as does a date-form value: the API
// sends seconds, and a wrong guess at a date is worse than the default. The result is always
// within (0, transientRetryTimeout]: a value that would busy-loop the request is floored at
// retryAfterFloor, and a value at or beyond the retry budget — including one that would
// overflow time.Duration's int64 nanosecond range on conversion — falls back to the fixed
// interval instead of being converted at all, so the overflow never happens.
func retryAfter(httpResp *http.Response) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(httpResp.Header.Get("Retry-After")))
	if err != nil || seconds < 0 || seconds > int(transientRetryTimeout/time.Second) {
		return transientRetryInterval
	}
	if wait := time.Duration(seconds) * time.Second; wait > retryAfterFloor {
		return wait
	}
	return retryAfterFloor
}

// planModifierSet is one plan modifier, expressed once per attribute type.
//
// A plan modifier is typed, so attaching one means a switch over every attribute type the
// generated schemas can hold. Taking the set as a parameter keeps that switch in one place
// instead of one copy per modifier, where the copies could drift.
type planModifierSet struct {
	str     planmodifier.String
	boolean planmodifier.Bool
	i32     planmodifier.Int32
	i64     planmodifier.Int64
	f32     planmodifier.Float32
	f64     planmodifier.Float64
	dynamic planmodifier.Dynamic
	number  planmodifier.Number
	list    planmodifier.List
	mapping planmodifier.Map
	set     planmodifier.Set
	object  planmodifier.Object
}

// applyPlanModifier attaches the modifier matching the named attribute's type.
//
// name is exactly "<resource>.<attribute>". The resource half exists only to make the
// diagnostics below read like a schema path — it is discarded before the lookup, which is
// always against the top-level attrs map. A name that is not of that shape, including a
// nested "<resource>.<block>.<attribute>" path, cannot be resolved against that map and is
// rejected outright: silently falling back to the last segment would resolve a nested path
// against a same-named top-level attribute instead of the one actually intended.
//
// An unhandled attribute type, and a planModifierSet with no modifier for the matched type,
// are both reported rather than skipped: skipping either restores the silent behaviour the
// caller attached a modifier to prevent. `consequence` says what that behaviour is, so the
// diagnostic names the effect rather than only the missing case.
func applyPlanModifier(
	attrs map[string]schema.Attribute,
	name string,
	mods planModifierSet,
	consequence string,
	diags *diag.Diagnostics,
) {
	_, attribute, ok := strings.Cut(name, ".")
	if !ok || strings.Contains(attribute, ".") {
		diags.AddError(
			"provider bug",
			fmt.Sprintf("%s is not a resolvable <resource>.<attribute> path; nested paths are not supported. %s", name, consequence),
		)
		return
	}
	switch a := attrs[attribute].(type) {
	case schema.StringAttribute:
		if mods.str == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.str)
		attrs[attribute] = a
	case schema.BoolAttribute:
		if mods.boolean == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.boolean)
		attrs[attribute] = a
	case schema.Int32Attribute:
		if mods.i32 == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.i32)
		attrs[attribute] = a
	case schema.Int64Attribute:
		if mods.i64 == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.i64)
		attrs[attribute] = a
	case schema.Float32Attribute:
		if mods.f32 == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.f32)
		attrs[attribute] = a
	case schema.Float64Attribute:
		if mods.f64 == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.f64)
		attrs[attribute] = a
	case schema.DynamicAttribute:
		if mods.dynamic == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.dynamic)
		attrs[attribute] = a
	case schema.NumberAttribute:
		if mods.number == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.number)
		attrs[attribute] = a
	case schema.ListAttribute:
		if mods.list == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.list)
		attrs[attribute] = a
	case schema.MapAttribute:
		if mods.mapping == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.mapping)
		attrs[attribute] = a
	case schema.SetAttribute:
		if mods.set == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.set)
		attrs[attribute] = a
	case schema.ObjectAttribute:
		if mods.object == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.object)
		attrs[attribute] = a
	case schema.SingleNestedAttribute:
		if mods.object == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.object)
		attrs[attribute] = a
	case schema.ListNestedAttribute:
		if mods.list == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.list)
		attrs[attribute] = a
	case schema.SetNestedAttribute:
		if mods.set == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.set)
		attrs[attribute] = a
	case schema.MapNestedAttribute:
		if mods.mapping == nil {
			missingModifier(name, a, consequence, diags)
			return
		}
		a.PlanModifiers = append(a.PlanModifiers, mods.mapping)
		attrs[attribute] = a
	case nil:
		diags.AddError(
			"provider bug",
			fmt.Sprintf("%s is not in the generated schema. %s", name, consequence),
		)
	default:
		diags.AddError(
			"provider bug",
			fmt.Sprintf("%T has no plan modifier here for %s. %s", a, name, consequence),
		)
	}
}

// missingModifier reports a planModifierSet with no entry for a's type.
//
// A struct literal that omits a field supplies a nil interface there, and appending it would
// attach a nil plan modifier that the framework later dereferences — a provider panic during
// `terraform plan` rather than a diagnostic. Reporting it here keeps the failure inside this
// function, at the same severity as the other cases this switch already guards against.
func missingModifier(name string, a schema.Attribute, consequence string, diags *diag.Diagnostics) {
	diags.AddError(
		"provider bug",
		fmt.Sprintf("%s has no plan modifier configured for %T. %s", name, a, consequence),
	)
}

// requiresReplace marks an attribute so that changing it replaces the resource.
//
// The API accepts some attributes when creating a resource and not when updating it. The
// generated schema comes from tfplugingen, which reads only the create body and so cannot
// know that.
//
// The generated code calls this for every attribute absent from the update body.
func requiresReplace(attrs map[string]schema.Attribute, name string, diags *diag.Diagnostics) {
	applyPlanModifier(attrs, name, planModifierSet{
		str:     stringplanmodifier.RequiresReplace(),
		boolean: boolplanmodifier.RequiresReplace(),
		i32:     int32planmodifier.RequiresReplace(),
		i64:     int64planmodifier.RequiresReplace(),
		f32:     float32planmodifier.RequiresReplace(),
		f64:     float64planmodifier.RequiresReplace(),
		dynamic: dynamicplanmodifier.RequiresReplace(),
		number:  numberplanmodifier.RequiresReplace(),
		list:    listplanmodifier.RequiresReplace(),
		mapping: mapplanmodifier.RequiresReplace(),
		set:     setplanmodifier.RequiresReplace(),
		object:  objectplanmodifier.RequiresReplace(),
	}, "Terraform would report an in-place update the API does not implement, and write a value into state the API never received.", diags)
}

// useNonNullStateForUnknown keeps an attribute's known value in the plan instead of marking
// it unknown.
//
// A computed attribute with no plan modifier is planned as unknown whenever the resource has
// any change at all. That alone is only noise, but a reference to one of them carries the
// unknown into another resource, and an unknown reference that requires replacement destroys
// that resource on an update nothing asked to affect it.
//
// The non-null variant, because the plain `UseStateForUnknown` copies a null prior state into
// the plan. An attribute the API fills in later — an external id it had not minted when the
// row was first read — would then plan as null and apply as a value, which Terraform rejects
// as an inconsistent result. This one leaves a null state unknown.
//
// The generated code calls this for every response field marked `x-mc-terraform-stable`, and
// for every create-only output: a value the API returns once can never change on an update.
func useNonNullStateForUnknown(attrs map[string]schema.Attribute, name string, diags *diag.Diagnostics) {
	applyPlanModifier(attrs, name, planModifierSet{
		str:     stringplanmodifier.UseNonNullStateForUnknown(),
		boolean: boolplanmodifier.UseNonNullStateForUnknown(),
		i32:     int32planmodifier.UseNonNullStateForUnknown(),
		i64:     int64planmodifier.UseNonNullStateForUnknown(),
		f32:     float32planmodifier.UseNonNullStateForUnknown(),
		f64:     float64planmodifier.UseNonNullStateForUnknown(),
		dynamic: dynamicplanmodifier.UseNonNullStateForUnknown(),
		number:  numberplanmodifier.UseNonNullStateForUnknown(),
		list:    listplanmodifier.UseNonNullStateForUnknown(),
		mapping: mapplanmodifier.UseNonNullStateForUnknown(),
		set:     setplanmodifier.UseNonNullStateForUnknown(),
		object:  objectplanmodifier.UseNonNullStateForUnknown(),
	}, "Terraform would plan the attribute as unknown on every update.", diags)
}

// mapOfStrings converts a Terraform string map, treating null and unknown as absent.
//
// Known limitation: a known map containing a null or unknown element fails ElementsAs, and
// this signature has nowhere to report that — unlike its four siblings, which all take a
// *diag.Diagnostics. Reporting it needs that parameter, which needs a paired api-codegen
// template change; until then, an error here returns nil so the generated caller's `!= nil`
// guard omits the field instead of sending an empty collection that clears it. This is a
// known limitation with the signature as it stands, not an oversight.
func mapOfStrings(ctx context.Context, m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(m.Elements()))
	if diags := m.ElementsAs(ctx, &out, false); diags.HasError() {
		return nil
	}
	return out
}

// listOfStrings converts a Terraform string list, treating null and unknown as absent.
//
// Known limitation: see mapOfStrings — the same signature constraint applies here.
func listOfStrings(ctx context.Context, l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(l.Elements()))
	if diags := l.ElementsAs(ctx, &out, false); diags.HasError() {
		return nil
	}
	return out
}

// stringMap converts a map of strings the API returned into a Terraform map attribute value.
//
// The reverse of mapOfStrings. It is total: a nil map is an empty known map, and every string
// is a valid element, so nothing here can fail. The generated model literal needs one
// expression per field, and types.MapValueFrom returns a (value, diags) pair, so this is what
// the generated Read calls for a map-typed response field.
func stringMap(m map[string]string) types.Map {
	elems := make(map[string]attr.Value, len(m))
	for k, v := range m {
		elems[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, elems)
}

// stringList converts a list of strings the API returned into a Terraform list attribute value.
//
// The reverse of listOfStrings; see stringMap for why it is total and why it exists.
func stringList(l []string) types.List {
	elems := make([]attr.Value, 0, len(l))
	for _, v := range l {
		elems = append(elems, types.StringValue(v))
	}
	return types.ListValueMust(types.StringType, elems)
}

// apiErr renders an API failure using the decoded problem's named fields, falling back to the
// raw response body only when the SDK decoded no model.
//
// The client's error alone reports the status and nothing else. Naming the fields — rather
// than forwarding the whole body — also bounds what a diagnostic can ever show: an unnamed
// problem member added on the server side does not reach a customer's screen just because it
// showed up in the response. Without this a plan failure reads as "400 Bad Request" with no
// indication of which attribute was at fault.
func apiErr(err error) string {
	var e *sdk.GenericOpenAPIError
	if !errors.As(err, &e) {
		return err.Error()
	}
	problem, ok := e.Model().(sdk.ProblemOut)
	if !ok {
		// No declared model for this status: the SDK decoded nothing, so the raw body is all
		// there is to show.
		return fmt.Sprintf("%s: %s", e.Error(), e.Body())
	}
	msg := fmt.Sprintf("%s: %s (request %s)", problem.Title, problem.Detail, problem.RequestId)
	for _, fieldErr := range problem.Errors {
		msg += fmt.Sprintf("; %s: %s", strings.Join(fieldErr.Field, "."), fieldErr.Message)
	}
	return msg
}
