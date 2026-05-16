package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geanlabs/gean/node"
)

// TestAggregatorStatusHandler covers GET — returns the controller's current
// value with the spec-exact JSON key "is_aggregator".
func TestAggregatorStatusHandler(t *testing.T) {
	for _, init := range []bool{false, true} {
		ctl := node.NewAggregatorController(init)
		rec := httptest.NewRecorder()
		AggregatorStatusHandler(ctl)(rec, httptest.NewRequest("GET", "/lean/v0/admin/aggregator", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("init=%v: status=%d, want 200", init, rec.Code)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("init=%v: decode body: %v", init, err)
		}
		if got := body["is_aggregator"]; got != init {
			t.Errorf("init=%v: is_aggregator=%v, want %v", init, got, init)
		}
	}
}

// TestAggregatorToggleHandler covers POST happy paths: activate, deactivate,
// and idempotent flips. Response shape matches leanSpec PR #636.
func TestAggregatorToggleHandler(t *testing.T) {
	tests := []struct {
		name     string
		initial  bool
		body     string
		wantNow  bool
		wantPrev bool
	}{
		{"activate", false, `{"enabled": true}`, true, false},
		{"deactivate", true, `{"enabled": false}`, false, true},
		{"idempotent_enable", true, `{"enabled": true}`, true, true},
		{"idempotent_disable", false, `{"enabled": false}`, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctl := node.NewAggregatorController(tc.initial)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/lean/v0/admin/aggregator", strings.NewReader(tc.body))
			AggregatorToggleHandler(ctl)(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d, want 200", rec.Code)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if got := body["is_aggregator"]; got != tc.wantNow {
				t.Errorf("is_aggregator=%v, want %v", got, tc.wantNow)
			}
			if got := body["previous"]; got != tc.wantPrev {
				t.Errorf("previous=%v, want %v", got, tc.wantPrev)
			}
			if ctl.Get() != tc.wantNow {
				t.Errorf("controller not persisted: Get()=%v, want %v", ctl.Get(), tc.wantNow)
			}
		})
	}
}

// TestAggregatorToggleHandler_BadRequest covers the 400 matrix: empty body,
// malformed JSON, missing `enabled`, wrong type. All must leave the
// controller's state untouched.
func TestAggregatorToggleHandler_BadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty_body", ""},
		{"malformed_json", `{"enabled": tru`},
		{"missing_enabled", `{"other": true}`},
		{"non_bool_enabled", `{"enabled": "yes"}`},
		{"unknown_field", `{"enabled": true, "extra": 1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctl := node.NewAggregatorController(true)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/lean/v0/admin/aggregator", strings.NewReader(tc.body))
			AggregatorToggleHandler(ctl)(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", rec.Code)
			}
			if !ctl.Get() {
				t.Errorf("controller mutated on bad request; want unchanged (true)")
			}
		})
	}
}
