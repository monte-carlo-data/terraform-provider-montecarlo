package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	sdk "github.com/monte-carlo-data/mc-sdk-go"
)

const (
	transientRetryTimeout  = 5 * time.Minute
	transientRetryInterval = 15 * time.Second
)

// createWithRetryOn503 retries while the API answers 503, up to a bounded budget.
//
// Provisioning reaches systems that are not immediately consistent — a freshly created role
// is not usable the instant it exists — so the API reports a transient failure rather than
// pretending. Terraform has no notion of "try again shortly", so the wait happens here.
// Anything other than a 503, or exhausting the budget, is returned to the caller.
//
// The generated code calls this for operations the spec marks as retryable.
func createWithRetryOn503[T any](
	ctx context.Context, call func() (*T, *http.Response, error),
) (*T, error) {
	deadline := time.Now().Add(transientRetryTimeout)
	for {
		out, httpResp, err := call()
		if err == nil {
			return out, nil
		}
		retryable := httpResp != nil &&
			httpResp.StatusCode == http.StatusServiceUnavailable &&
			time.Now().Before(deadline)
		if !retryable {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(transientRetryInterval):
		}
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
