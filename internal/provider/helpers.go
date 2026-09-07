package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	sdk "github.com/monte-carlo-data/mc-sdk-go"
)

const (
	transientRetryTimeout  = 5 * time.Minute
	transientRetryInterval = 15 * time.Second
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
// sends seconds, and a wrong guess at a date is worse than the default.
func retryAfter(httpResp *http.Response) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(httpResp.Header.Get("Retry-After")))
	if err != nil || seconds < 0 {
		return transientRetryInterval
	}
	return time.Duration(seconds) * time.Second
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
// An unhandled type is reported rather than skipped: skipping restores the silent behaviour
// the caller attached a modifier to prevent. `consequence` says what that behaviour is, so
// the diagnostic names the effect rather than only the missing case.
func applyPlanModifier(
	attrs map[string]schema.Attribute,
	name string,
	mods planModifierSet,
	consequence string,
	diags *diag.Diagnostics,
) {
	attribute := name[strings.LastIndex(name, ".")+1:]
	switch a := attrs[attribute].(type) {
	case schema.StringAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.str)
		attrs[attribute] = a
	case schema.BoolAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.boolean)
		attrs[attribute] = a
	case schema.Int32Attribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.i32)
		attrs[attribute] = a
	case schema.Int64Attribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.i64)
		attrs[attribute] = a
	case schema.Float32Attribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.f32)
		attrs[attribute] = a
	case schema.Float64Attribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.f64)
		attrs[attribute] = a
	case schema.DynamicAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.dynamic)
		attrs[attribute] = a
	case schema.NumberAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.number)
		attrs[attribute] = a
	case schema.ListAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.list)
		attrs[attribute] = a
	case schema.MapAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.mapping)
		attrs[attribute] = a
	case schema.SetAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.set)
		attrs[attribute] = a
	case schema.ObjectAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.object)
		attrs[attribute] = a
	case schema.SingleNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.object)
		attrs[attribute] = a
	case schema.ListNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.list)
		attrs[attribute] = a
	case schema.SetNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mods.set)
		attrs[attribute] = a
	case schema.MapNestedAttribute:
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
// The generated code calls this for every response field marked `x-mc-terraform-stable`.
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
func mapOfStrings(ctx context.Context, m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(m.Elements()))
	m.ElementsAs(ctx, &out, false)
	return out
}

// listOfStrings converts a Terraform string list, treating null and unknown as absent.
func listOfStrings(ctx context.Context, l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(l.Elements()))
	l.ElementsAs(ctx, &out, false)
	return out
}

// apiErr renders an API failure with its response body.
//
// The client's error alone reports the status and nothing else, while the body carries the
// problem detail explaining what to change. Without this a plan failure reads as "400 Bad
// Request" with no indication of which attribute was at fault.
func apiErr(err error) string {
	var generic sdk.GenericOpenAPIError
	if e, ok := err.(*sdk.GenericOpenAPIError); ok {
		return fmt.Sprintf("%s: %s", e.Error(), string(e.Body()))
	}
	if e, ok := err.(sdk.GenericOpenAPIError); ok {
		generic = e
		return fmt.Sprintf("%s: %s", generic.Error(), string(generic.Body()))
	}
	return err.Error()
}
