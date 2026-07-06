package dogeboxd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Dogebox-WG/dogeboxd/pkg/version"
	"golang.org/x/mod/semver"
)

// JobStatus represents the current state of a job
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusInProgress JobStatus = "in_progress"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
	JobStatusOrphaned   JobStatus = "orphaned"
)

// JobRecord represents a persisted job for the frontend activity view
type JobRecord struct {
	ID             string     `json:"id"`
	Started        time.Time  `json:"started"`
	Finished       *time.Time `json:"finished"` // nil if not finished
	DisplayName    string     `json:"displayName"`
	Action         string     `json:"action"` // Action type: install, upgrade, uninstall, etc.
	TargetVersion  string     `json:"targetVersion,omitempty"`
	Progress       int        `json:"progress"` // 0-100
	Status         JobStatus  `json:"status"`
	SummaryMessage string     `json:"summaryMessage"`
	ErrorMessage   string     `json:"errorMessage"`
	PupID          string     `json:"pupID"` // Associated pup if applicable
}

var reconciledInstalledOSFlakePath = "/etc/nixos/flake.nix"
var reconciledOSFlakeVersionPattern = regexp.MustCompile(`(?m)^\s*dbxRelease\s*=\s*"(v[^"]+)"`)
var reconciledReadFile = os.ReadFile
var reconciledCurrentDBXRelease = func() string {
	return version.GetDBXRelease().Release
}

// JobManager handles job persistence and state management
type JobManager struct {
	store      *TypeStore[JobRecord]
	activeJobs map[string]*JobRecord // in-memory cache of active jobs
	jobsMutex  sync.RWMutex
	dbx        *Dogeboxd
}

func NewJobManager(sm *StoreManager, dbx *Dogeboxd) *JobManager {
	return &JobManager{
		store:      GetTypeStore[JobRecord](sm),
		activeJobs: make(map[string]*JobRecord),
		dbx:        dbx,
	}
}

// SetDogeboxd sets the Dogeboxd reference (needed for circular dependency)
func (jm *JobManager) SetDogeboxd(dbx *Dogeboxd) {
	jm.dbx = dbx
}

// CreateJobRecord creates a new job record from a Job
func (jm *JobManager) CreateJobRecord(j Job) (*JobRecord, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	displayName := jm.getDisplayName(j)
	action := jm.getActionName(j)

	// Extract pupID from Action (j.State is not yet set when CreateJobRecord is called)
	pupID := jm.getPupIDFromAction(j)

	record := &JobRecord{
		ID:             j.ID,
		Started:        j.Start,
		Finished:       nil,
		DisplayName:    displayName,
		Action:         action,
		Progress:       0,
		Status:         JobStatusQueued,
		SummaryMessage: "Job queued",
		ErrorMessage:   "",
		PupID:          pupID,
	}
	if action, ok := j.A.(SystemUpdate); ok {
		record.TargetVersion = action.Version
	}

	if j.State != nil {
		record.PupID = j.State.ID
	}

	// Store in database
	if err := jm.store.Set(j.ID, *record); err != nil {
		return nil, fmt.Errorf("failed to store job record: %w", err)
	}

	// Add to active jobs cache
	jm.activeJobs[j.ID] = record

	return record, nil
}

// UpdateJobPupID updates the pupID of a job record (used for install jobs where pupID isn't known at creation)
func (jm *JobManager) UpdateJobPupID(jobID string, pupID string) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	record, ok := jm.activeJobs[jobID]
	if !ok {
		// Try to load from store
		recordValue, err := jm.store.Get(jobID)
		if err != nil {
			return fmt.Errorf("job not found: %s", jobID)
		}
		record = &recordValue
		jm.activeJobs[jobID] = record
	}

	// Update the pupID
	record.PupID = pupID

	// Persist to database
	return jm.store.Set(record.ID, *record)
}

// UpdateJobProgress updates job progress from ActionProgress
func (jm *JobManager) UpdateJobProgress(ap ActionProgress) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	record, ok := jm.activeJobs[ap.ActionID]
	if !ok {
		// Try to load from store
		var err error
		recordValue, err := jm.store.Get(ap.ActionID)
		if err != nil {
			return fmt.Errorf("job not found: %s", ap.ActionID)
		}
		record = &recordValue
		jm.activeJobs[ap.ActionID] = record
	}

	// Update progress
	if ap.Progress > 0 {
		record.Progress = ap.Progress
	}

	// Update status - move to in_progress as soon as job starts sending updates
	if record.Status == JobStatusQueued {
		record.Status = JobStatusInProgress
	}

	// Update pupID if it was empty (e.g., install jobs) and we now have a PupID
	if record.PupID == "" && ap.PupID != "" {
		record.PupID = ap.PupID
	}

	// Update summary message
	record.SummaryMessage = ap.Msg

	// Handle errors
	if ap.Error {
		record.ErrorMessage = ap.Msg
	}

	// Persist to database
	return jm.store.Set(record.ID, *record)
}

// CompleteJob marks a job as completed
func (jm *JobManager) CompleteJob(jobID string, err string) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	record, ok := jm.activeJobs[jobID]
	if !ok {
		// Try to load from store
		recordValue, loadErr := jm.store.Get(jobID)
		if loadErr != nil {
			return fmt.Errorf("job not found: %s", jobID)
		}
		record = &recordValue
	}

	now := time.Now()
	record.Finished = &now

	if err != "" {
		record.Status = JobStatusFailed
		record.ErrorMessage = err
		// Progress stays at current value
		record.SummaryMessage = "Job failed"
	} else {
		record.Status = JobStatusCompleted
		record.Progress = 100
		record.SummaryMessage = "Job completed successfully"
	}

	// Remove from active jobs
	delete(jm.activeJobs, jobID)

	// Persist to database
	storeErr := jm.store.Set(record.ID, *record)
	if storeErr != nil {
		return storeErr
	}

	// Emit WebSocket event for job completion
	if jm.dbx != nil {
		eventType := "job:completed"
		if err != "" {
			eventType = "job:failed"
		}
		jm.dbx.SendChange(Change{ID: "internal", Type: eventType, Update: record})
	}

	return nil
}

// MarkJobOrphaned marks a job as orphaned when it no longer has runtime processing state.
func (jm *JobManager) MarkJobOrphaned(jobID string) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	record, ok := jm.activeJobs[jobID]
	if !ok {
		recordValue, loadErr := jm.store.Get(jobID)
		if loadErr != nil {
			return fmt.Errorf("job not found: %s", jobID)
		}
		record = &recordValue
	}

	if record.Status != JobStatusQueued && record.Status != JobStatusInProgress {
		return nil
	}

	now := time.Now()
	record.Finished = &now
	record.Status = JobStatusOrphaned
	record.SummaryMessage = "Job marked as orphaned"
	record.ErrorMessage = "Job is no longer being processed"

	delete(jm.activeJobs, jobID)

	if err := jm.store.Set(record.ID, *record); err != nil {
		return err
	}

	if jm.dbx != nil {
		jm.dbx.SendChange(Change{ID: "internal", Type: "job:orphaned", Update: record})
	}

	return nil
}

// GetJob retrieves a job record by ID
func (jm *JobManager) GetJob(jobID string) (*JobRecord, error) {
	jm.jobsMutex.RLock()
	defer jm.jobsMutex.RUnlock()

	// Check active jobs first
	if record, ok := jm.activeJobs[jobID]; ok {
		return record, nil
	}

	// Load from store
	record, err := jm.store.Get(jobID)
	if err != nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	return &record, nil
}

// IsJobActive returns true if the job is in the active jobs cache (not yet completed)
// Used to avoid duplicate CompleteJob calls
func (jm *JobManager) IsJobActive(jobID string) bool {
	jm.jobsMutex.RLock()
	defer jm.jobsMutex.RUnlock()
	_, ok := jm.activeJobs[jobID]
	return ok
}

// GetAllJobs retrieves all job records
func (jm *JobManager) GetAllJobs() ([]JobRecord, error) {
	query := fmt.Sprintf("SELECT value FROM %s ORDER BY json_extract(value, '$.started') DESC", jm.store.Table)
	return jm.store.Exec(query)
}

// GetActiveJobs retrieves all jobs that are queued or in progress
func (jm *JobManager) GetActiveJobs() ([]JobRecord, error) {
	query := fmt.Sprintf("SELECT value FROM %s WHERE json_extract(value, '$.status') IN ('queued', 'in_progress') ORDER BY json_extract(value, '$.started') ASC", jm.store.Table)
	return jm.store.Exec(query)
}

// GetRecentJobs retrieves recent completed/failed jobs
func (jm *JobManager) GetRecentJobs(limit int) ([]JobRecord, error) {
	query := fmt.Sprintf("SELECT value FROM %s WHERE json_extract(value, '$.status') IN ('completed', 'failed', 'cancelled', 'orphaned') ORDER BY json_extract(value, '$.finished') DESC LIMIT %d", jm.store.Table, limit)
	return jm.store.Exec(query)
}

// ClearCompletedJobs removes completed/failed jobs older than the specified duration
func (jm *JobManager) ClearCompletedJobs(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339Nano)
	query := fmt.Sprintf(`DELETE FROM %s 
		WHERE json_extract(value, '$.status') IN ('completed', 'failed', 'cancelled', 'orphaned')
		  AND json_extract(value, '$.finished') IS NOT NULL
		  AND json_extract(value, '$.finished') < ?`, jm.store.Table)

	count, err := jm.store.ExecWrite(query, cutoff)
	return int(count), err
}

// ClearAllJobs removes ALL jobs (for development/cleanup)
func (jm *JobManager) ClearAllJobs() (int, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	query := fmt.Sprintf("DELETE FROM %s", jm.store.Table)
	count, err := jm.store.ExecWrite(query)
	if err != nil {
		return 0, err
	}

	// Clear active jobs cache
	jm.activeJobs = make(map[string]*JobRecord)

	return int(count), nil
}

// DeleteJob removes a single job record from storage and the active cache.
func (jm *JobManager) DeleteJob(jobID string) error {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	delete(jm.activeJobs, jobID)
	return jm.store.Del(jobID)
}

// ClearOrphanedJobs marks jobs stuck in queued/in_progress state as failed
// Jobs are considered orphaned if they've been queued for longer than the threshold
func (jm *JobManager) ClearOrphanedJobs(olderThan time.Duration) (int, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	cutoff := time.Now().Add(-olderThan)
	now := time.Now()

	count, err := jm.markOrphanedJobsAsFailed(now, cutoff)
	if err != nil {
		return 0, err
	}

	// Clear active jobs cache for these orphaned jobs
	for id, job := range jm.activeJobs {
		if job.Status == JobStatusQueued || job.Status == JobStatusInProgress {
			if job.Started.Before(cutoff) {
				delete(jm.activeJobs, id)
			}
		}
	}

	return count, nil
}

// ReconcileCompletedSystemUpdateJobs marks interrupted system update jobs as
// completed when the current runtime state already matches their target version.
func (jm *JobManager) ReconcileCompletedSystemUpdateJobs() (int, error) {
	currentDBXRelease := strings.TrimSpace(reconciledCurrentDBXRelease())
	installedFlakeVersion, err := getReconciledInstalledOSFlakeVersion()
	if err != nil {
		return 0, err
	}

	activeJobs, err := jm.GetActiveJobs()
	if err != nil {
		return 0, err
	}

	reconciled := 0
	for _, job := range activeJobs {
		if !isInterruptedSystemJob(&job) {
			continue
		}

		targetVersion := strings.TrimSpace(jm.resolveSystemUpdateTargetVersion(job))
		if targetVersion == "" {
			continue
		}
		if currentDBXRelease != targetVersion || installedFlakeVersion != targetVersion {
			continue
		}

		if err := jm.CompleteJob(job.ID, ""); err != nil {
			return reconciled, err
		}
		jm.appendJobRecoveryLog(job.ID, fmt.Sprintf("System update to %s completed after dogeboxd restart", targetVersion))
		reconciled++
	}

	return reconciled, nil
}

// ClearInterruptedSystemJobs marks system-level jobs from a previous dogeboxd
// process as failed. These jobs cannot resume after dogeboxd restarts, and if
// they remain active they keep the updates UI locked.
func (jm *JobManager) ClearInterruptedSystemJobs() (int, error) {
	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	now := time.Now()
	count := 0
	activeJobs, err := jm.getActiveSystemJobsLocked()
	if err != nil {
		return 0, err
	}
	for _, job := range activeJobs {
		if err := jm.markInterruptedSystemJobAsFailedLocked(job, now); err != nil {
			return count, err
		}
		count++
	}

	for id, job := range jm.activeJobs {
		if job.Status != JobStatusQueued && job.Status != JobStatusInProgress {
			continue
		}
		if isInterruptedSystemJob(job) {
			delete(jm.activeJobs, id)
		}
	}

	return count, nil
}

func (jm *JobManager) markOrphanedJobsAsFailed(finished time.Time, startedBefore time.Time) (int, error) {
	query := fmt.Sprintf(`UPDATE %s SET value = json_set(json_set(json_set(value, '$.status', 'failed'), '$.errorMessage', 'Job was orphaned (stuck in queue)'), '$.finished', ?) WHERE json_extract(value, '$.status') IN ('queued', 'in_progress') AND json_extract(value, '$.started') < ?`, jm.store.Table)
	count, err := jm.store.ExecWrite(query, finished.Format(time.RFC3339Nano), startedBefore.Format(time.RFC3339Nano))
	return int(count), err
}

func (jm *JobManager) getActiveSystemJobsLocked() ([]JobRecord, error) {
	activeJobs, err := jm.GetActiveJobs()
	if err != nil {
		return nil, err
	}

	filtered := make([]JobRecord, 0, len(activeJobs))
	for _, job := range activeJobs {
		if isInterruptedSystemJob(&job) {
			filtered = append(filtered, job)
		}
	}

	return filtered, nil
}

func (jm *JobManager) markInterruptedSystemJobAsFailedLocked(job JobRecord, finished time.Time) error {
	targetVersion := strings.TrimSpace(jm.resolveSystemUpdateTargetVersion(job))
	job.Status = JobStatusFailed
	job.Finished = &finished
	job.ErrorMessage = interruptedSystemJobMessage(job)
	job.SummaryMessage = "Job failed"
	if err := jm.store.Set(job.ID, job); err != nil {
		return err
	}

	note := interruptedSystemJobLogNote(job, targetVersion)
	if note != "" {
		jm.appendJobRecoveryLog(job.ID, note)
	}

	return nil
}

func isInterruptedSystemJob(job *JobRecord) bool {
	if job == nil {
		return false
	}
	if job.Action == (SystemUpdate{}).ActionName() {
		return true
	}

	displayName := strings.ToLower(job.DisplayName)
	return displayName == "system update"
}

func interruptedSystemJobMessage(job JobRecord) string {
	return "Job was interrupted by dogeboxd restart"
}

func interruptedSystemJobLogNote(job JobRecord, targetVersion string) string {
	if isExpectedManualRestartMigrationUpdate(targetVersion) {
		return "System update was interrupted by dogeboxd restart. This is expected when performing a 0.8.x to 0.9.x update; restart dogeboxd to continue phase 2."
	}

	return ""
}

func isExpectedManualRestartMigrationUpdate(targetVersion string) bool {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return false
	}

	return semver.IsValid(targetVersion) && semver.Compare(targetVersion, "v0.9.0-rc.1") >= 0
}

func (jm *JobManager) resolveSystemUpdateTargetVersion(job JobRecord) string {
	targetVersion := strings.TrimSpace(job.TargetVersion)
	if targetVersion != "" {
		return targetVersion
	}

	return strings.TrimSpace(jm.readSystemUpdateTargetVersionFromLog(job.ID))
}

func getReconciledInstalledOSFlakeVersion() (string, error) {
	contents, err := reconciledReadFile(reconciledInstalledOSFlakePath)
	if err != nil {
		return "", err
	}

	match := reconciledOSFlakeVersionPattern.FindStringSubmatch(string(contents))
	if len(match) != 2 {
		return "", fmt.Errorf("installed os flake version not found in %s", reconciledInstalledOSFlakePath)
	}

	return strings.TrimSpace(match[1]), nil
}

func (jm *JobManager) readSystemUpdateTargetVersionFromLog(jobID string) string {
	if jm.dbx == nil || jm.dbx.config == nil || jm.dbx.config.ContainerLogDir == "" {
		return ""
	}

	logPath := filepath.Join(jm.dbx.config.ContainerLogDir, "pup-"+jobID)
	contents, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}

	matches := regexp.MustCompile(`Starting system update to (v[^\s]+)`).FindAllStringSubmatch(string(contents), -1)
	if len(matches) == 0 {
		return ""
	}

	return matches[len(matches)-1][1]
}

func (jm *JobManager) appendJobRecoveryLog(jobID string, msg string) {
	if jm.dbx == nil || jm.dbx.config == nil || jm.dbx.config.ContainerLogDir == "" {
		return
	}

	logPath := filepath.Join(jm.dbx.config.ContainerLogDir, "pup-"+jobID)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, _ = fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
}

// getDisplayName returns a human-readable name for the job
func (jm *JobManager) getDisplayName(j Job) string {
	switch a := j.A.(type) {
	case InstallPup:
		return fmt.Sprintf("Install %s", a.PupName)
	case InstallPups:
		if len(a) == 1 {
			return fmt.Sprintf("Install %s", a[0].PupName)
		}
		return fmt.Sprintf("Install %d Pups", len(a))
	case UninstallPup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Uninstall %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Uninstall %s", pup.Manifest.Meta.Name)
			}
		}
		return "Uninstall Pup"
	case PurgePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Purge %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Purge %s", pup.Manifest.Meta.Name)
			}
		}
		return "Purge Pup"
	case EnablePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Enable %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Enable %s", pup.Manifest.Meta.Name)
			}
		}
		return "Enable Pup"
	case DisablePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Disable %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Disable %s", pup.Manifest.Meta.Name)
			}
		}
		return "Disable Pup"
	case UpdatePupConfig:
		return "Update Pup Configuration"
	case UpdatePupProviders:
		return "Update Pup Providers"
	case ImportBlockchainData:
		return "Import Blockchain Data"
	case UpdatePendingSystemNetwork:
		return "Update Network Configuration"
	case InitialBootstrap:
		return "Initial Setup"
	case EnableSSH:
		return "Enable SSH"
	case DisableSSH:
		return "Disable SSH"
	case AddSSHKey:
		return "Add SSH Key"
	case RemoveSSHKey:
		return "Remove SSH Key"
	case SaveCustomNix:
		return "Save Custom OS Configuration"
	case AddBinaryCache:
		return "Add Binary Cache"
	case RemoveBinaryCache:
		return "Remove Binary Cache"
	case SystemUpdate:
		return "System Update"
	case UpdateMetrics:
		return "Update Metrics"
	case UpdateTimezone:
		return "Update Timezone"
	case UpdateKeymap:
		return "Update Keyboard Layout"
	case UpdateNixCache:
		return "Update Nix Cache"
	case CheckPupUpdates:
		if a.PupID != "" {
			// Checking specific pup
			if jm.dbx != nil {
				if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
					return fmt.Sprintf("Check Updates for %s", pup.Manifest.Meta.Name)
				}
			}
			return "Check Pup Updates"
		}
		return "Check All Pup Updates"
	case UpgradePup:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Upgrade %s to %s", j.State.Manifest.Meta.Name, a.TargetVersion)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Upgrade %s to %s", pup.Manifest.Meta.Name, a.TargetVersion)
			}
		}
		return "Upgrade Pup"
	case RollbackPupUpgrade:
		// Try to get pup name from state first
		if j.State != nil && j.State.Manifest.Meta.Name != "" {
			return fmt.Sprintf("Rollback %s", j.State.Manifest.Meta.Name)
		}
		// Fallback: look up pup by ID if we have access to dbx
		if jm.dbx != nil {
			if pup, _, err := jm.dbx.Pups.GetPup(a.PupID); err == nil {
				return fmt.Sprintf("Rollback %s", pup.Manifest.Meta.Name)
			}
		}
		return "Rollback Pup"
	default:
		return "System Operation"
	}
}

// getActionName returns the action type identifier for the job
func (jm *JobManager) getActionName(j Job) string {
	return j.A.ActionName()
}

// getPupIDFromAction extracts the pupID from various Action types
func (jm *JobManager) getPupIDFromAction(j Job) string {
	// Special case: InstallPup doesn't have a pupID yet
	if _, ok := j.A.(InstallPup); ok {
		return ""
	}

	// Use reflection to extract PupID field if it exists
	v := reflect.ValueOf(j.A)
	if v.Kind() == reflect.Struct {
		pupIDField := v.FieldByName("PupID")
		if pupIDField.IsValid() && pupIDField.Kind() == reflect.String {
			return pupIDField.String()
		}
	}

	return ""
}

// SyncWithActiveJobs ensures all jobs in the queue are tracked
func (jm *JobManager) SyncWithActiveJobs() error {
	activeJobs, err := jm.GetActiveJobs()
	if err != nil {
		return err
	}

	jm.jobsMutex.Lock()
	defer jm.jobsMutex.Unlock()

	// Refresh active jobs cache
	jm.activeJobs = make(map[string]*JobRecord, len(activeJobs))
	for i := range activeJobs {
		jm.activeJobs[activeJobs[i].ID] = &activeJobs[i]
	}

	return nil
}

// JobsUpdate represents a change to the jobs list for WebSocket updates
type JobsUpdate struct {
	Jobs []JobRecord `json:"jobs"`
}

// MarshalJSON custom marshaler to handle the Update interface
func (ju JobsUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Jobs []JobRecord `json:"jobs"`
	}{
		Jobs: ju.Jobs,
	})
}
