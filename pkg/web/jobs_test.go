package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteJobRejectsNonQueuedJobs(t *testing.T) {
	sm, err := dogeboxd.NewStoreManager(":memory:")
	require.NoError(t, err)

	dbx := dogeboxd.NewDogeboxd(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &dogeboxd.ServerConfig{
		ContainerLogDir: "",
	})
	jm := dogeboxd.NewJobManager(sm, &dbx)
	dbx.SetJobManager(jm)

	testCases := []struct {
		name   string
		jobID  string
		status dogeboxd.JobStatus
		setup  func(t *testing.T, sm *dogeboxd.StoreManager, jm *dogeboxd.JobManager, job dogeboxd.Job) error
	}{
		{
			name:   "failed",
			jobID:  "failed-job",
			status: dogeboxd.JobStatusFailed,
			setup: func(t *testing.T, sm *dogeboxd.StoreManager, jm *dogeboxd.JobManager, job dogeboxd.Job) error {
				_, err := jm.CreateJobRecord(job)
				require.NoError(t, err)
				return jm.CompleteJob(job.ID, "install failed")
			},
		},
		{
			name:   "cancelled",
			jobID:  "cancelled-job",
			status: dogeboxd.JobStatusCancelled,
			setup: func(t *testing.T, sm *dogeboxd.StoreManager, jm *dogeboxd.JobManager, job dogeboxd.Job) error {
				_, err := jm.CreateJobRecord(job)
				require.NoError(t, err)
				record, err := jm.GetJob(job.ID)
				require.NoError(t, err)
				now := time.Now()
				record.Status = dogeboxd.JobStatusCancelled
				record.Finished = &now
				record.SummaryMessage = "Job cancelled"
				store := dogeboxd.GetTypeStore[dogeboxd.JobRecord](sm)
				return store.Set(record.ID, *record)
			},
		},
		{
			name:   "orphaned",
			jobID:  "orphaned-job",
			status: dogeboxd.JobStatusOrphaned,
			setup: func(t *testing.T, sm *dogeboxd.StoreManager, jm *dogeboxd.JobManager, job dogeboxd.Job) error {
				_, err := jm.CreateJobRecord(job)
				require.NoError(t, err)
				return jm.MarkJobOrphaned(job.ID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			job := dogeboxd.Job{
				ID:    tc.jobID,
				Start: time.Now(),
				A:     dogeboxd.InstallPup{PupName: "test-app"},
			}

			err := tc.setup(t, sm, jm, job)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodDelete, "/jobs/"+job.ID, nil)
			req.SetPathValue("jobID", job.ID)
			rec := httptest.NewRecorder()

			api{dbx: dbx}.deleteJob(rec, req)

			assert.Equal(t, http.StatusConflict, rec.Code)

			stillThere, err := jm.GetJob(job.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.status, stillThere.Status)
		})
	}
}

func TestDeleteJobAllowsQueuedJobs(t *testing.T) {
	sm, err := dogeboxd.NewStoreManager(":memory:")
	require.NoError(t, err)

	dbx := dogeboxd.NewDogeboxd(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &dogeboxd.ServerConfig{
		ContainerLogDir: "",
	})
	jm := dogeboxd.NewJobManager(sm, &dbx)
	dbx.SetJobManager(jm)

	job := dogeboxd.Job{
		ID:    "queued-job",
		Start: time.Now(),
		A:     dogeboxd.InstallPup{PupName: "test-app"},
	}

	_, err = jm.CreateJobRecord(job)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/jobs/"+job.ID, nil)
	req.SetPathValue("jobID", job.ID)
	rec := httptest.NewRecorder()

	api{dbx: dbx}.deleteJob(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	_, err = jm.GetJob(job.ID)
	assert.Error(t, err)
}
