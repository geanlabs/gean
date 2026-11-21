package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTestDriverHandlerServesOnlyTheDriver(t *testing.T) {
	handler := TestDriverHandler()
	for _, c := range []struct {
		method, path, body string
		wantStatus         int
	}{
		{http.MethodGet, "/lean/v0/health", "", http.StatusOK},
		{http.MethodPost, "/lean/v0/test_driver/state_transition/run", "not json", http.StatusBadRequest},
		{http.MethodGet, "/lean/v0/checkpoints/justified", "", http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		if recorder.Code != c.wantStatus {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, recorder.Code, c.wantStatus)
		}
	}
}
