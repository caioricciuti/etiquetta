package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIngestReplayRejectsPathIdentifiersBeforeStorage(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/r", strings.NewReader(
		`{"session_id":"../../outside","domain":"example.com","events":[{"type":1}]}`,
	))
	response := httptest.NewRecorder()

	(&Handlers{}).IngestReplay(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}
