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

// requiresReplace marks an attribute so that changing it replaces the resource.
//
// The API accepts some attributes when creating a resource and not when updating it. The
// generated schema comes from tfplugingen, which reads only the create body and so cannot
// know that. Without the plan modifier Terraform reports an in-place update, calls an update
// the API does not implement, and writes a value into state that the API never received.
//
// A plan modifier is typed, so every attribute type needs its own case. An unhandled type is
// reported rather than skipped, because skipping it would restore the silent-wrong-state
// behaviour this exists to prevent.
//
// The generated code calls this for every attribute absent from the update body.
func requiresReplace(attrs map[string]schema.Attribute, name string, diags *diag.Diagnostics) {
	attribute := name[strings.LastIndex(name, ".")+1:]
	switch a := attrs[attribute].(type) {
	case schema.StringAttribute:
		a.PlanModifiers = append(a.PlanModifiers, stringplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.BoolAttribute:
		a.PlanModifiers = append(a.PlanModifiers, boolplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.Int32Attribute:
		a.PlanModifiers = append(a.PlanModifiers, int32planmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.Int64Attribute:
		a.PlanModifiers = append(a.PlanModifiers, int64planmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.Float32Attribute:
		a.PlanModifiers = append(a.PlanModifiers, float32planmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.Float64Attribute:
		a.PlanModifiers = append(a.PlanModifiers, float64planmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.DynamicAttribute:
		a.PlanModifiers = append(a.PlanModifiers, dynamicplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.NumberAttribute:
		a.PlanModifiers = append(a.PlanModifiers, numberplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.ListAttribute:
		a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.MapAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mapplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.SetAttribute:
		a.PlanModifiers = append(a.PlanModifiers, setplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.ObjectAttribute:
		a.PlanModifiers = append(a.PlanModifiers, objectplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.SingleNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, objectplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.ListNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.SetNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, setplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case schema.MapNestedAttribute:
		a.PlanModifiers = append(a.PlanModifiers, mapplanmodifier.RequiresReplace())
		attrs[attribute] = a
	case nil:
		diags.AddError(
			"provider bug",
			fmt.Sprintf("%s is not in the generated schema, so it cannot require replacement.", name),
		)
	default:
		diags.AddError(
			"provider bug",
			fmt.Sprintf(
				"%s cannot be updated, and %T has no plan modifier here, so Terraform "+
					"would report an update that does nothing.", name, a,
			),
		)
	}
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
