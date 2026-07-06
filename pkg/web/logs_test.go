package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogTailReturnsPagingMetadata(t *testing.T) {
	api := api{}

	req := httptest.NewRequest(http.MethodGet, "/log/pup/test-pup/tail?limit=25&before=cursor-1", nil)
	req.SetPathValue("PupID", "test-pup")
	recorder := httptest.NewRecorder()

	resumeToken := "resume-1"
	olderCursor := "cursor-2"

	api.getLogTail(recorder, req, "PupID", func(logID string, before *string, limit int) (dogeboxd.LogPage, error) {
		require.Equal(t, "test-pup", logID)
		require.NotNil(t, before)
		require.Equal(t, "cursor-1", *before)
		require.Equal(t, 25, limit)

		return dogeboxd.LogPage{
			Lines:        []string{"line-1", "line-2"},
			ResumeToken:  &resumeToken,
			OlderCursor:  &olderCursor,
			HasMoreOlder: true,
		}, nil
	})

	require.Equal(t, http.StatusOK, recorder.Code)

	var response logTailResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, []string{"line-1", "line-2"}, response.Lines)
	require.NotNil(t, response.ResumeToken)
	assert.Equal(t, "resume-1", *response.ResumeToken)
	require.NotNil(t, response.OlderCursor)
	assert.Equal(t, "cursor-2", *response.OlderCursor)
	assert.True(t, response.HasMoreOlder)
}

func TestGetLogTailRejectsInvalidLimit(t *testing.T) {
	api := api{}

	req := httptest.NewRequest(http.MethodGet, "/log/pup/test-pup/tail?limit=0", nil)
	req.SetPathValue("PupID", "test-pup")
	recorder := httptest.NewRecorder()

	api.getLogTail(recorder, req, "PupID", func(string, *string, int) (dogeboxd.LogPage, error) {
		t.Fatal("fetch function should not be called for invalid limits")
		return dogeboxd.LogPage{}, nil
	})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Log tail limit must be greater than zero")
}
