// Copyright Monte Carlo AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	sdk "github.com/monte-carlo-data/mc-sdk-go/montecarlo"
)

// A deployment can answer its create before it is ready. These call the generated Create the
// way the framework does and check that it waits, what it reads, and what lands in state.

// deploymentAPI answers the create with one deployment, then each read with the next one in
// turn, repeating the last. With unavailable set, every read answers 503 instead. It keeps a
// line per request.
type deploymentAPI struct {
	created     string
	reads       []string
	unavailable bool

	mu       sync.Mutex
	requests []string
}

func (a *deploymentAPI) client(t *testing.T) *clients {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.requests = append(a.requests, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodPost && a.unavailable {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"type": "about:blank", "title": "Service Unavailable", "status": 503,
				"detail": "Try again shortly.", "code": "service_unavailable", "request_id": "req-1"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, a.created)
			return
		}
		if len(a.reads) == 0 {
			t.Errorf("unexpected read: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		read := a.reads[0]
		if len(a.reads) > 1 {
			a.reads = a.reads[1:]
		}
		_, _ = io.WriteString(w, read)
	}))
	t.Cleanup(srv.Close)
	cfg := sdk.NewConfiguration()
	cfg.Servers = sdk.ServerConfigurations{{URL: srv.URL}}
	cfg.HTTPClient = srv.Client()
	return &clients{api: sdk.NewAPIClient(cfg)}
}

func (a *deploymentAPI) log() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.requests...)
}

const waitingDeploymentID = "3f6c1e7a-2b4d-4c8e-9a1f-5d7e9b0c2a4f"

// deploymentBody is a deployment as the API returns it, in the given status.
func deploymentBody(status, detail string) string {
	statusDetail := "null"
	if detail != "" {
		statusDetail = fmt.Sprintf("%q", detail)
	}
	return fmt.Sprintf(`{"id": %q, "name": "traces", "type": "AGENT_OBSERVABILITY", "enabled": false,
		"status": %q, "status_detail": %s, "deletion_protection": true}`,
		waitingDeploymentID, status, statusDetail)
}

// createDeployment runs the generated Create against api and returns its response. The wait
// polls every millisecond, and timeoutUnit is how long one second of its timeout lasts.
func createDeployment(t *testing.T, api *deploymentAPI, timeoutUnit time.Duration) resource.CreateResponse {
	t.Helper()
	shrinkStatusWaitTimings(t)
	statusTimeoutUnit = timeoutUnit
	ctx := context.Background()
	r, empty := configured(t, NewDeploymentResource, api.client(t))
	typ := empty.Schema.Type().TerraformType(ctx)
	planned := object(typ, map[string]tftypes.Value{"type": str("AGENT_OBSERVABILITY")})
	created := resource.CreateResponse{State: empty}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: empty.Schema, Raw: planned},
		Config: tfsdk.Config{Schema: empty.Schema, Raw: planned},
	}, &created)
	return created
}

func TestCreatingADeploymentThatIsAlreadyReadyReadsNothing(t *testing.T) {
	api := &deploymentAPI{created: deploymentBody("READY", "")}

	created := createDeployment(t, api, time.Millisecond)

	if created.Diagnostics.HasError() {
		t.Fatalf("create: %v", created.Diagnostics)
	}
	if got := api.log(); len(got) != 1 || got[0] != "POST /api/v2/deployments" {
		t.Errorf("requests %v, want the create alone", got)
	}
	if status := stateString(t, created.State, path.Root("status")); status.ValueString() != "READY" {
		t.Errorf("state holds status %s, want READY", status)
	}
}

func TestCreatingADeploymentWaitsUntilItIsReady(t *testing.T) {
	api := &deploymentAPI{
		created: deploymentBody("PROVISIONING", ""),
		reads: []string{
			deploymentBody("PROVISIONING", ""),
			deploymentBody("READY", ""),
		},
	}

	created := createDeployment(t, api, time.Millisecond)

	if created.Diagnostics.HasError() {
		t.Fatalf("create: %v", created.Diagnostics)
	}
	read := "GET /api/v2/deployments/" + waitingDeploymentID
	want := []string{"POST /api/v2/deployments", read, read}
	if got := api.log(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests %v, want %v", got, want)
	}
	// A resource that depends on this one in the same apply reads this state.
	if status := stateString(t, created.State, path.Root("status")); status.ValueString() != "READY" {
		t.Errorf("state holds status %s, want READY", status)
	}
	if id := stateString(t, created.State, path.Root("id")); id.ValueString() != waitingDeploymentID {
		t.Errorf("state holds id %s, want %s", id, waitingDeploymentID)
	}
}

func TestADeploymentThatFailsIsAnErrorAndStaysInState(t *testing.T) {
	const detail = "Setting up the deployment did not finish."
	api := &deploymentAPI{
		created: deploymentBody("PROVISIONING", ""),
		reads: []string{
			deploymentBody("PROVISIONING", ""),
			deploymentBody("FAILED", detail),
		},
	}

	created := createDeployment(t, api, time.Millisecond)

	if !created.Diagnostics.HasError() {
		t.Fatal("create succeeded, want an error for a failed deployment")
	}
	var reported []string
	for _, d := range created.Diagnostics.Errors() {
		reported = append(reported, d.Summary()+": "+d.Detail())
	}
	if msg := strings.Join(reported, "\n"); !strings.Contains(msg, "FAILED") || !strings.Contains(msg, detail) {
		t.Errorf("error reads %q, want the status and its detail", msg)
	}
	// Terraform taints a resource whose create set state and returned an error, so the next
	// apply replaces it. The state says why.
	if id := stateString(t, created.State, path.Root("id")); id.ValueString() != waitingDeploymentID {
		t.Errorf("state holds id %s, want %s", id, waitingDeploymentID)
	}
	if status := stateString(t, created.State, path.Root("status")); status.ValueString() != "FAILED" {
		t.Errorf("state holds status %s, want FAILED", status)
	}
	if got := stateString(t, created.State, path.Root("status_detail")); got.ValueString() != detail {
		t.Errorf("state holds status_detail %s, want %q", got, detail)
	}
}

func TestADeploymentThatNeverSettlesTimesOutAndStaysInState(t *testing.T) {
	api := &deploymentAPI{
		created: deploymentBody("PROVISIONING", ""),
		reads:   []string{deploymentBody("PROVISIONING", "")},
	}

	// The create's timeout is 1800 seconds, so this one lasts 18ms.
	created := createDeployment(t, api, 10*time.Microsecond)

	if !created.Diagnostics.HasError() {
		t.Fatal("create succeeded, want a timeout")
	}
	var reported []string
	for _, d := range created.Diagnostics.Errors() {
		reported = append(reported, d.Summary()+": "+d.Detail())
	}
	if msg := strings.Join(reported, "\n"); !strings.Contains(msg, "timed out after 1800 seconds") {
		t.Errorf("error reads %q, want it to name the timeout", msg)
	}
	if id := stateString(t, created.State, path.Root("id")); id.ValueString() != waitingDeploymentID {
		t.Errorf("state holds id %s, want %s", id, waitingDeploymentID)
	}
	if status := stateString(t, created.State, path.Root("status")); status.ValueString() != "PROVISIONING" {
		t.Errorf("state holds status %s, want PROVISIONING", status)
	}
}

func TestADeploymentWaitThatTimesOutOnUnavailableReadsSaysBoth(t *testing.T) {
	api := &deploymentAPI{created: deploymentBody("PROVISIONING", ""), unavailable: true}

	created := createDeployment(t, api, 10*time.Microsecond)

	if !created.Diagnostics.HasError() {
		t.Fatal("create succeeded, want a timeout")
	}
	var reported []string
	for _, d := range created.Diagnostics.Errors() {
		reported = append(reported, d.Summary()+": "+d.Detail())
	}
	msg := strings.Join(reported, "\n")
	if !strings.Contains(msg, "timed out after 1800 seconds") {
		t.Errorf("error reads %q, want it to name the timeout", msg)
	}
	if !strings.Contains(msg, "Service Unavailable") {
		t.Errorf("error reads %q, want it to name the last read's failure", msg)
	}
	if id := stateString(t, created.State, path.Root("id")); id.ValueString() != waitingDeploymentID {
		t.Errorf("state holds id %s, want %s", id, waitingDeploymentID)
	}
}
