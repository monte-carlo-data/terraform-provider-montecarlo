package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Every helper that attaches a plan modifier. Each one goes through the same type switch, so
// a type missing from it is missing from both.
var planModifierHelpers = map[string]func(map[string]schema.Attribute, string, *diag.Diagnostics){
	"requiresReplace":           requiresReplace,
	"useNonNullStateForUnknown": useNonNullStateForUnknown,
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

// An unhandled type is reported at runtime rather than dropped, but the error only surfaces
// once someone plans against that resource, so the coverage is pinned here instead.
func TestEveryHelperCoversEveryAttributeType(t *testing.T) {
	for helper, apply := range planModifierHelpers {
		for name, attribute := range everyAttributeType() {
			t.Run(helper+"/"+name, func(t *testing.T) {
				attrs := map[string]schema.Attribute{name: attribute}
				var diags diag.Diagnostics

				apply(attrs, "resource."+name, &diags)

				if diags.HasError() {
					t.Fatalf("reported %v", diags.Errors())
				}
				if got := planModifierCount(attrs[name]); got != 1 {
					t.Fatalf("got %d plan modifiers, want 1", got)
				}
			})
		}
	}
}

func TestEveryHelperReportsAnAttributeTheSchemaDoesNotHave(t *testing.T) {
	for helper, apply := range planModifierHelpers {
		t.Run(helper, func(t *testing.T) {
			var diags diag.Diagnostics

			apply(map[string]schema.Attribute{}, "resource.renamed", &diags)

			if !diags.HasError() {
				t.Fatal("a marked attribute absent from the schema must not pass in silence")
			}
			if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "resource.renamed") {
				t.Fatalf("the error must name the attribute, got %q", detail)
			}
		})
	}
}

// The null-state case is what makes tagging `aws_external_id` safe. The API mints it after the
// deployment exists, so the first plan has no value to keep and must leave it unknown.
func TestUseNonNullStateForUnknownKeepsStateOnlyOnceThereIsAValue(t *testing.T) {
	for name, tc := range map[string]struct {
		state types.String
		want  types.String
	}{
		"a value in state is planned": {state: types.StringValue("abc"), want: types.StringValue("abc")},
		"a null state stays unknown":  {state: types.StringNull(), want: types.StringUnknown()},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := map[string]schema.Attribute{"external_id": schema.StringAttribute{Computed: true}}
			var diags diag.Diagnostics
			useNonNullStateForUnknown(attrs, "resource.external_id", &diags)
			if diags.HasError() {
				t.Fatalf("reported %v", diags.Errors())
			}

			modifier, ok := attrs["external_id"].(schema.StringAttribute).PlanModifiers[0].(planmodifier.String)
			if !ok {
				t.Fatal("the attached modifier does not modify a string")
			}
			resp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
			modifier.PlanModifyString(context.Background(), planmodifier.StringRequest{
				StateValue:  tc.state,
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			}, resp)

			if !resp.PlanValue.Equal(tc.want) {
				t.Fatalf("planned %s, want %s", resp.PlanValue, tc.want)
			}
		})
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

// A transient failure is one the API says it did not carry out, so the retry has to fire on
// both statuses that mean that and on neither that does not.
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
		"seconds":            {header: "30", want: 30 * time.Second},
		"zero means at once": {header: "0", want: 0},
		"surrounding space":  {header: " 30 ", want: 30 * time.Second},
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

func TestWithRetryOnTransientRetriesUntilItSucceeds(t *testing.T) {
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

func rateLimited(retryAfter string) *http.Response {
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	resp.Header.Set("Retry-After", retryAfter)
	return resp
}
