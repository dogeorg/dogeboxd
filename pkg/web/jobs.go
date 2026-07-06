package web

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

// Get all jobs
func (t api) getJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := t.dbx.JobManager.GetAllJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

// Get a single job by ID
func (t api) getJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Job ID required")
		return
	}

	job, err := t.dbx.JobManager.GetJob(jobID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Job not found")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"job":     job,
	})
}

// Get active jobs (queued or in progress)
func (t api) getActiveJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := t.dbx.JobManager.GetActiveJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve active jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

// Get recent completed/failed jobs
func (t api) getRecentJobs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	jobs, err := t.dbx.JobManager.GetRecentJobs(limit)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve recent jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

// Clear old completed jobs
func (t api) clearCompletedJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OlderThanDays int `json:"olderThanDays"`
	}

	// Try to decode body, but don't fail if empty
	_ = json.NewDecoder(r.Body).Decode(&req)

	// If olderThanDays is 0 or negative, clear ALL completed jobs
	if req.OlderThanDays <= 0 {
		req.OlderThanDays = 0 // Clear all completed jobs regardless of age
	}

	olderThan := time.Duration(req.OlderThanDays) * 24 * time.Hour
	count, err := t.dbx.JobManager.ClearCompletedJobs(olderThan)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to clear jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"cleared": count,
	})
}

// Get job statistics
func (t api) getJobStats(w http.ResponseWriter, r *http.Request) {
	allJobs, err := t.dbx.JobManager.GetAllJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job stats")
		return
	}

	stats := map[string]int{
		"total":      len(allJobs),
		"queued":     0,
		"inProgress": 0,
		"completed":  0,
		"failed":     0,
		"cancelled":  0,
		"orphaned":   0,
	}

	for _, job := range allJobs {
		switch job.Status {
		case dogeboxd.JobStatusQueued:
			stats["queued"]++
		case dogeboxd.JobStatusInProgress:
			stats["inProgress"]++
		case dogeboxd.JobStatusCompleted:
			stats["completed"]++
		case dogeboxd.JobStatusFailed:
			stats["failed"]++
		case dogeboxd.JobStatusCancelled:
			stats["cancelled"]++
		case dogeboxd.JobStatusOrphaned:
			stats["orphaned"]++
		}
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

func (t api) deleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Job ID required")
		return
	}

	job, err := t.dbx.JobManager.GetJob(jobID)
	if err != nil {
		sendErrorResponse(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status != dogeboxd.JobStatusQueued {
		sendErrorResponse(w, http.StatusConflict, "Only queued jobs can be deleted")
		return
	}

	for _, runtimeJobID := range t.dbx.GetRuntimeJobIDs() {
		if runtimeJobID != jobID {
			continue
		}

		if !t.dbx.RemoveFromQueue(jobID) {
			sendErrorResponse(w, http.StatusConflict, "Job is still being processed")
			return
		}

		break
	}

	if err := t.dbx.JobManager.DeleteJob(jobID); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to delete job")
		return
	}

	t.dbx.SendChange(dogeboxd.Change{ID: "internal", Type: "job:deleted", Update: job})

	sendResponse(w, map[string]interface{}{
		"success": true,
		"deleted": jobID,
	})
}

func (t api) createOrphanCandidateJob(w http.ResponseWriter, r *http.Request) {
	if !t.config.DevMode {
		sendErrorResponse(w, http.StatusForbidden, "This endpoint is only available in dev mode")
		return
	}

	jobID, err := randomJobID()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to create job ID")
		return
	}

	job := dogeboxd.Job{
		ID:    jobID,
		Start: time.Now().Add(-3 * time.Minute),
		A:     dogeboxd.UpdateNixCache{},
	}

	if _, err := t.dbx.JobManager.CreateJobRecord(job); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create orphan candidate job: %v", err))
		return
	}

	if err := t.dbx.JobManager.UpdateJobProgress(dogeboxd.ActionProgress{
		ActionID: jobID,
		Progress: 42,
		Step:     "debug-orphan",
		Msg:      "Debug orphan candidate: waiting for a worker that does not exist",
	}); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update orphan candidate job: %v", err))
		return
	}

	if err := t.writeSyntheticJobLog(jobID); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write orphan candidate logs: %v", err))
		return
	}

	record, err := t.dbx.JobManager.GetJob(jobID)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve orphan candidate job")
		return
	}

	t.dbx.SendChange(dogeboxd.Change{ID: "internal", Type: "job:created", Update: record})

	sendResponse(w, map[string]interface{}{
		"success": true,
		"job":     record,
		"message": "Created orphan candidate job. The backend should mark it orphaned on the next detection pass.",
	})
}

func randomJobID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", b), nil
}

func (t api) writeSyntheticJobLog(jobID string) error {
	if t.config.ContainerLogDir == "" {
		return fmt.Errorf("container log directory is not configured")
	}

	if err := os.MkdirAll(t.config.ContainerLogDir, 0o755); err != nil {
		return err
	}

	now := time.Now()
	lines := []string{
		fmt.Sprintf("[%s] Debug orphan candidate created", now.Add(-2*time.Minute).Format("2006-01-02 15:04:05")),
		fmt.Sprintf("[%s] Step: debug-orphan", now.Add(-90*time.Second).Format("2006-01-02 15:04:05")),
		fmt.Sprintf("[%s] Progress stalled at 42%%", now.Add(-75*time.Second).Format("2006-01-02 15:04:05")),
		fmt.Sprintf("[%s] Waiting for worker process...", now.Add(-60*time.Second).Format("2006-01-02 15:04:05")),
		fmt.Sprintf("[%s] No runtime job registered for this action", now.Add(-45*time.Second).Format("2006-01-02 15:04:05")),
		fmt.Sprintf("[%s] Backend orphan detector should mark this job orphaned", now.Add(-30*time.Second).Format("2006-01-02 15:04:05")),
	}

	logPath := filepath.Join(t.config.ContainerLogDir, "pup-"+jobID)
	return os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// Clear ALL jobs (development/cleanup endpoint)
func (t api) clearAllJobs(w http.ResponseWriter, r *http.Request) {
	count, err := t.dbx.JobManager.ClearAllJobs()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to clear all jobs")
		return
	}

	sendResponse(w, map[string]interface{}{
		"success": true,
		"cleared": count,
		"message": fmt.Sprintf("Cleared all %d jobs", count),
	})
}
