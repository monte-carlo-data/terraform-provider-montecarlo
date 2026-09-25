package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	sdk "github.com/monte-carlo-data/mc-sdk-go/montecarlo"
)

// Terraform plans a write-only value as null and hands it to the provider only in the
// configuration. These call Create and Update the way the framework does, with the secret in
// the config and not the plan, and check what reaches the API and what lands in state.

// apiRecorder answers every request with one canned body and keeps the last request body.
type apiRecorder struct {
	response string
	body     map[string]any
}

func (a *apiRecorder) client(t *testing.T) *clients {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		a.body = nil
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &a.body); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, a.response)
	}))
	t.Cleanup(srv.Close)
	cfg := sdk.NewConfiguration()
	cfg.Servers = sdk.ServerConfigurations{{URL: srv.URL}}
	cfg.HTTPClient = srv.Client()
	return &clients{api: sdk.NewAPIClient(cfg)}
}

// configured is a resource with its schema, wired to the recorder.
func configured(t *testing.T, newResource func() resource.Resource, c *clients) (resource.Resource, tfsdk.State) {
	t.Helper()
	ctx := context.Background()
	r := newResource()
	var cr resource.ConfigureResponse
	r.(resource.ResourceWithConfigure).Configure(ctx, resource.ConfigureRequest{ProviderData: c}, &cr)
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() || cr.Diagnostics.HasError() {
		t.Fatalf("setup: %v %v", sr.Diagnostics, cr.Diagnostics)
	}
	s := sr.Schema
	return r, tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

// object builds a value of typ with the given attributes set and every other one null.
func object(typ tftypes.Type, set map[string]tftypes.Value) tftypes.Value {
	obj := typ.(tftypes.Object)
	vals := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, at := range obj.AttributeTypes {
		if v, ok := set[name]; ok {
			vals[name] = v
		} else {
			vals[name] = tftypes.NewValue(at, nil)
		}
	}
	return tftypes.NewValue(typ, vals)
}

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
func num(n int) tftypes.Value    { return tftypes.NewValue(tftypes.Number, n) }

func nullString() tftypes.Value { return tftypes.NewValue(tftypes.String, nil) }

func stateString(t *testing.T, state tfsdk.State, p path.Path) types.String {
	t.Helper()
	var v types.String
	if d := state.GetAttribute(context.Background(), p, &v); d.HasError() {
		t.Fatalf("reading %s: %v", p, d)
	}
	return v
}

func stateInt(t *testing.T, state tfsdk.State, p path.Path) types.Int64 {
	t.Helper()
	var v types.Int64
	if d := state.GetAttribute(context.Background(), p, &v); d.HasError() {
		t.Fatalf("reading %s: %v", p, d)
	}
	return v
}

const snowflakeOut = `{"id": "8b1c9a52-3e4f-4c2a-9d1e-2f6a7b8c9d0e", "connection_type": "snowflake",
	"storage_type": "mc_managed", "created_time": "2026-09-25T00:00:00Z", "account": "xy12345",
	"user": "MONTE_CARLO", "warehouse": null}`

func TestASnowflakeKeyIsSentFromTheConfigAndNotKeptInState(t *testing.T) {
	ctx := context.Background()
	api := &apiRecorder{response: snowflakeOut}
	r, empty := configured(t, NewSnowflakeCredentialsResource, api.client(t))
	typ := empty.Schema.Type().TerraformType(ctx)
	inputs := map[string]tftypes.Value{
		"account":                str("xy12345"),
		"user":                   str("MONTE_CARLO"),
		"private_key_wo_version": num(1),
	}
	withKey := func(key tftypes.Value) map[string]tftypes.Value {
		m := map[string]tftypes.Value{"private_key_wo": key}
		for k, v := range inputs {
			m[k] = v
		}
		return m
	}

	created := resource.CreateResponse{State: empty}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: empty.Schema, Raw: object(typ, withKey(nullString()))},
		Config: tfsdk.Config{Schema: empty.Schema, Raw: object(typ, withKey(str("first key")))},
	}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("create: %v", created.Diagnostics)
	}
	if got := api.body["private_key"]; got != "first key" {
		t.Errorf("create sent private_key %v, want the config's", got)
	}
	if key := stateString(t, created.State, path.Root("private_key_wo")); !key.IsNull() {
		t.Errorf("state holds private_key_wo %s, want null", key)
	}
	if v := stateInt(t, created.State, path.Root("private_key_wo_version")); v.ValueInt64() != 1 {
		t.Errorf("state holds private_key_wo_version %s, want 1", v)
	}

	// A bump plans an update; the new key again arrives only in the config.
	inputs["private_key_wo_version"] = num(2)
	updated := resource.UpdateResponse{State: created.State}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: empty.Schema, Raw: object(typ, withKey(nullString()))},
		Config: tfsdk.Config{Schema: empty.Schema, Raw: object(typ, withKey(str("second key")))},
		State:  created.State,
	}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatalf("update: %v", updated.Diagnostics)
	}
	if got := api.body["private_key"]; got != "second key" {
		t.Errorf("update sent private_key %v, want the config's", got)
	}
	if key := stateString(t, updated.State, path.Root("private_key_wo")); !key.IsNull() {
		t.Errorf("state holds private_key_wo %s, want null", key)
	}
	if v := stateInt(t, updated.State, path.Root("private_key_wo_version")); v.ValueInt64() != 2 {
		t.Errorf("state holds private_key_wo_version %s, want 2", v)
	}
}

const azureDataStoreOut = `{"id": "5d2e8f14-7a3b-4c6d-8e9f-0a1b2c3d4e5f",
	"deployment_id": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "storage_type": "AZURE_BLOB",
	"authentication_type": "AZURE_STORAGE_SERVICE_PRINCIPAL", "enabled": true,
	"container_name": "mcd-store"}`

// A nested block is sent whole: its secret from the config, its other fields from the plan.
func TestANestedClientSecretIsSentFromTheConfigBesideItsSiblingsFromThePlan(t *testing.T) {
	ctx := context.Background()
	api := &apiRecorder{response: azureDataStoreOut}
	r, empty := configured(t, NewAzureCollectionDataStoreResource, api.client(t))
	typ := empty.Schema.Type().TerraformType(ctx)
	blockType := typ.(tftypes.Object).AttributeTypes["service_principal"]
	resourceWith := func(clientID string, secret tftypes.Value) tftypes.Value {
		return object(typ, map[string]tftypes.Value{
			"deployment_id":       str("1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"),
			"authentication_type": str("AZURE_STORAGE_SERVICE_PRINCIPAL"),
			"container_name":      str("mcd-store"),
			"service_principal": object(blockType, map[string]tftypes.Value{
				"account_url":              str("https://store.blob.core.windows.net"),
				"client_id":                str(clientID),
				"client_secret_wo":         secret,
				"client_secret_wo_version": num(1),
				"tenant_id":                str("tenant"),
			}),
		})
	}

	created := resource.CreateResponse{State: empty}
	r.Create(ctx, resource.CreateRequest{
		// Different client ids tell a field read from the plan from one read from the config.
		Plan:   tfsdk.Plan{Schema: empty.Schema, Raw: resourceWith("from-plan", nullString())},
		Config: tfsdk.Config{Schema: empty.Schema, Raw: resourceWith("from-config", str("the secret"))},
	}, &created)
	if created.Diagnostics.HasError() {
		t.Fatalf("create: %v", created.Diagnostics)
	}
	sent, _ := api.body["service_principal"].(map[string]any)
	if sent["client_secret"] != "the secret" {
		t.Errorf("sent client_secret %v, want the config's", sent["client_secret"])
	}
	if sent["client_id"] != "from-plan" {
		t.Errorf("sent client_id %v, want the plan's", sent["client_id"])
	}
	block := path.Root("service_principal")
	if s := stateString(t, created.State, block.AtName("client_secret_wo")); !s.IsNull() {
		t.Errorf("state holds client_secret_wo %s, want null", s)
	}
	if v := stateInt(t, created.State, block.AtName("client_secret_wo_version")); v.ValueInt64() != 1 {
		t.Errorf("state holds client_secret_wo_version %s, want 1", v)
	}
}
