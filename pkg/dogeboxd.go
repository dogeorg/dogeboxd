/*
Dogebox internal architecture:

 Actions are instructions from the user to do something, and come externally
 via the REST API or Websocket etc.  These are submitted to Dogeboxd.AddAction
 and become Jobs in the job queue, returning a Job ID

 Jobs are either processed directly, or if related to the system in some way,
 handed to the SystemUpdater.

 Completed Jobs are submitted to the Changes channel for reporting back to
 the user, along with their Job ID.

                                       ┌──────────────┐
                                       │  Dogeboxd{}  │
                                       │              │
                                       │  ┌────────►  │
                                       │  │Dogebox │  │
 REST API  ─────┐                      │  │Run Loop│  │
                │                      │  ◄──────┬─┘  │
                │                      │     ▲   │    │
                │                      │     │   ▼    │
                │              ======= │  ┌──┴─────►  │ =======   Job ID
                │ Actions      Jobs    │  │ System │  │ Changes
 WebSocket ─────┼───────────►  Channel │  │ Updater│  │ Channel ───► WebSocket
                │ Job ID       ======= │  ◄────────┘  │ =======
                │ ◄────                │              │
                │                      │   ▲      │   │
                │                      │   │      │   │
                │                      └───┼──────┼───┘
 System         │                          │      │
 Events   ──────┘                          │      ▼
                                           Nix CLI
                                           SystemD

*/

package dogeboxd

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golanglibs/gocollections/set/hashset"
)

type syncQueue struct {
	jobQueue            []Job               // pending jobs waiting to be handed to SystemUpdater
	nonQueuedActiveJobs hashset.Set[string] // runtime-active jobs that are not currently in jobQueue
	currentSystemJobID  string              // the single job currently handed to SystemUpdater
	jobQLock            sync.Mutex
	jobInProgress       sync.Mutex
	jobTimer            time.Time
}

type Dogeboxd struct {
	Pups             PupManager
	SystemUpdater    SystemUpdater
	SystemMonitor    SystemMonitor
	JournalReader    JournalReader
	NetworkManager   NetworkManager
	PupUpdateChecker PupUpdateChecker
	sm               StateManager
	sources          SourceManager
	nix              NixManager
	logtailer        LogTailer
	queue            *syncQueue
	jobs             chan Job
	Changes          chan Change
	JobManager       *JobManager
	config           *ServerConfig
}

// Global sequence counter for websocket Changes.
// We use a process-wide counter (rather than a Dogeboxd field) so that sequence ordering
// remains stable even when Dogeboxd methods use value receivers (copies).
var globalChangeSeq uint64

func NewDogeboxd(
	stateManager StateManager,
	pups PupManager,
	updater SystemUpdater,
	monitor SystemMonitor,
	journal JournalReader,
	networkManager NetworkManager,
	sourceManager SourceManager,
	nixManager NixManager,
	logtailer LogTailer,
	pupUpdateChecker PupUpdateChecker,
	config *ServerConfig,
) Dogeboxd {
	q := syncQueue{
		jobQueue:            []Job{},
		nonQueuedActiveJobs: hashset.New[string](),
		jobQLock:            sync.Mutex{},
		jobInProgress:       sync.Mutex{},
	}
	s := Dogeboxd{
		Pups:             pups,
		SystemUpdater:    updater,
		SystemMonitor:    monitor,
		JournalReader:    journal,
		NetworkManager:   networkManager,
		PupUpdateChecker: pupUpdateChecker,
		sm:               stateManager,
		sources:          sourceManager,
		nix:              nixManager,
		logtailer:        logtailer,
		queue:            &q,
		jobs:             make(chan Job, 256),
		Changes:          make(chan Change, 256),
		config:           config,
	}

	return s
	// TODO start monitoring all installed services
	// SUB TO PUP MANAGER monitor.GetMonChannel() <- []string{"dbus.service"}
}

// SetJobManager sets the JobManager reference after Dogeboxd is created
func (t *Dogeboxd) SetJobManager(jm *JobManager) {
	t.JobManager = jm
}

// Main Dogeboxd goroutine, handles routing messages in
// and out of the system via job and change channels,
// handles messages from subsystems ie: SystemUpdater,
// SystemMonitor etc.
func (t Dogeboxd) Run(started, stopped chan bool, stop chan context.Context) error {
	// Start periodic pup update checking in the background
	updateCheckerStop := make(chan bool)
	t.PupUpdateChecker.StartPeriodicCheck(updateCheckerStop)

	go func() {
		go func() {
			queueTicker := time.NewTicker(100 * time.Millisecond)
			orphanTicker := time.NewTicker(60 * time.Second)
			defer queueTicker.Stop()
			defer orphanTicker.Stop()

			// Create channels once outside the loop
			pupdateChannel := t.Pups.GetUpdateChannel()
			statsChannel := t.Pups.GetStatsChannel()
			eventChannel := t.PupUpdateChecker.GetEventChannel()
			updaterChannel := t.SystemUpdater.GetUpdateChannel()

		mainloop:
			for {
			dance:
				select {

				// Handle shutdown
				case <-stop:
					// Stop the update checker
					updateCheckerStop <- true
					break mainloop

				// Hand incoming jobs to the Job Dispatcher
				case j, ok := <-t.jobs:
					if !ok {
						break dance
					}
					// Queue management: skip a nix cache update if the next queued
					// job is already a nix cache update.
					if t.shouldSkipJob(j) {
						break dance
					}

					j.Start = time.Now() // start the job timer

					// Register tracked jobs in runtime state before persisting them so
					// orphan detection never sees "active in DB, missing from runtime".
					if record, err := t.createTrackedJobRecord(j); err == nil && record != nil {
						t.SendChange(Change{ID: "internal", Type: "job:created", Update: record})
					}

					t.jobDispatcher(j)

				// Handle pupdates from PupManager
				case p, ok := <-pupdateChannel:
					if !ok {
						break dance
					}
					// Don't broadcast a purged pup as a normal pup state update, otherwise clients may resurrect it in their local model.
					if p.Event == PUP_PURGED {
						t.SendChange(Change{ID: "internal", Type: "pup_purged", Update: map[string]string{"pupId": p.State.ID}})
					} else {
						t.SendChange(Change{ID: "internal", Type: "pup", Update: p.State})
					}

				// Handle stats from PupManager
				case stats, ok := <-statsChannel:
					if !ok {
						break dance
					}
					t.SendChange(Change{ID: "internal", Type: "stats", Update: stats})

				// Handle pup update check events
				case event, ok := <-eventChannel:
					if !ok {
						break dance
					}
					// Send event to frontend so it can refresh its cache
					t.SendChange(Change{ID: "internal", Type: "pup-updates-checked", Update: event})

				// Handle completed jobs from SystemUpdater
				case j, ok := <-updaterChannel:
					if !ok {
						break dance
					}
					j.Logger.Step("queue").Progress(100).Log(fmt.Sprintf("finished in %.2fs, queued %.2fs", time.Since(t.queue.jobTimer).Seconds(), time.Since(j.Start).Seconds()))

					// if this job was successful, AND it was a
					// job that results in the stop/start of a pup,
					// tell the PupManager to poll for state changes
					switch j.A.(type) {
					case InstallPup:
						t.Pups.FastPollPup(j.State.ID)
						// Check for updates at the new version (will overwrite stale cache entry)
						if j.State != nil {
							go t.PupUpdateChecker.CheckForUpdates(j.State.ID)
						}
					case EnablePup:
						t.Pups.FastPollPup(j.State.ID)
					case DisablePup:
						t.Pups.FastPollPup(j.State.ID)
					case UpdatePupProviders:
						t.Pups.FastPollPup(j.State.ID)
					case UpgradePup:
						t.Pups.FastPollPup(j.State.ID)
						// Check for updates at the new version (will overwrite stale cache entry)
						if j.Err == "" && j.State != nil {
							go t.PupUpdateChecker.CheckForUpdates(j.State.ID)
						}
					case RollbackPupUpgrade:
						t.Pups.FastPollPup(j.State.ID)
						// Check for updates at the rolled-back version (will overwrite stale cache entry)
						if j.Err == "" && j.State != nil {
							go t.PupUpdateChecker.CheckForUpdates(j.State.ID)
						}
					case UninstallPup:
						t.Pups.FastPollPup(j.State.ID)
						t.PupUpdateChecker.ClearCacheEntry(j.State.ID)
					case PurgePup:
						t.Pups.FastPollPup(j.State.ID)
						t.PupUpdateChecker.ClearCacheEntry(j.State.ID)
					}

					// TODO: explain why we I this
					if j.Err == "" && j.State != nil {
						state, _, err := t.Pups.GetPup(j.State.ID)
						if err == nil {
							j.Success = state
						}
					}

					// Update job record as completed/failed
					if t.JobManager != nil {
						err := t.JobManager.CompleteJob(j.ID, j.Err)
						if err == nil {
							jobRecord, getErr := t.JobManager.GetJob(j.ID)
							if getErr == nil {
								t.SendChange(Change{ID: "internal", Type: "job_completed", Update: jobRecord})
							}
						}
					}

					t.sendFinishedJob("action", j)
					// Only clear this after completion so the orphaned job monitor
					// doesn't mistakenly pick it up as missing from runtime state.
					t.clearCurrentSystemJobID(j.ID)
					t.queue.jobInProgress.Unlock()

				case <-queueTicker.C:
					t.pumpQueue()
				case <-orphanTicker.C:
					if _, err := t.DetectAndMarkOrphanedJobs(); err != nil {
						fmt.Printf("Warning: failed to detect orphaned jobs: %v\n", err)
					}
				}
			}
		}()
		// flag to Conductor we are running
		started <- true
		// Wait on a stop signal
		<-stop
		// do shutdown things and flag we are stopped
		stopped <- true
	}()
	return nil
}

// pumpQueue runs every 100ms and attempts to push another job to the SystemUpdater
// which has been queued with enqueue. Only one job can be running at a time.
// jobInProgress is unlocked int he main loop in Run when a job is finished.
func (t *Dogeboxd) pumpQueue() {
	if t.queue.jobInProgress.TryLock() {
		t.queue.jobQLock.Lock()
		if len(t.queue.jobQueue) > 0 {

			job := t.queue.jobQueue[0]
			t.queue.jobQueue = t.queue.jobQueue[1:]
			t.queue.currentSystemJobID = job.ID
			t.queue.jobQLock.Unlock()

			job.Logger.Step("queue").Log(fmt.Sprintf("Queued, position %d\n", len(t.queue.jobQueue)))
			t.SystemUpdater.AddJob(job)
			t.queue.jobTimer = time.Now()
		} else {
			t.queue.jobQLock.Unlock()
			t.queue.jobInProgress.Unlock()
		}
	}
}

// Add the new job to the queue
func (t *Dogeboxd) enqueue(j Job) {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()
	t.queue.nonQueuedActiveJobs.Remove(j.ID)
	t.queue.jobQueue = append(t.queue.jobQueue, j)
}

func (t *Dogeboxd) markNonQueuedActiveJob(jobID string) {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()
	t.queue.nonQueuedActiveJobs.Add(jobID)
}

func (t *Dogeboxd) clearNonQueuedActiveJob(jobID string) {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()
	t.queue.nonQueuedActiveJobs.Remove(jobID)
}

func (t *Dogeboxd) clearCurrentSystemJobID(jobID string) {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()
	if t.queue.currentSystemJobID == jobID {
		t.queue.currentSystemJobID = ""
	}
}

func (t *Dogeboxd) createTrackedJobRecord(j Job) (*JobRecord, error) {
	if t.JobManager == nil || !t.shouldTrackJob(j) {
		return nil, nil
	}

	t.markNonQueuedActiveJob(j.ID)

	record, err := t.JobManager.CreateJobRecord(j)
	if err != nil {
		t.clearNonQueuedActiveJob(j.ID)
		return nil, err
	}

	return record, nil
}

func (t *Dogeboxd) GetRuntimeJobIDs() []string {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()

	ids := make([]string, 0, len(t.queue.jobQueue)+t.queue.nonQueuedActiveJobs.Size()+1)
	if t.queue.currentSystemJobID != "" {
		ids = append(ids, t.queue.currentSystemJobID)
	}
	for _, job := range t.queue.jobQueue {
		ids = append(ids, job.ID)
	}
	t.queue.nonQueuedActiveJobs.ForEach(func(jobID *string) {
		ids = append(ids, *jobID)
	})

	return ids
}

func (t *Dogeboxd) RemoveFromQueue(jobID string) bool {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()

	for i, job := range t.queue.jobQueue {
		if job.ID != jobID {
			continue
		}

		t.queue.jobQueue = append(t.queue.jobQueue[:i], t.queue.jobQueue[i+1:]...)
		t.queue.nonQueuedActiveJobs.Remove(jobID)
		return true
	}

	return false
}

func (t Dogeboxd) shouldSkipJob(j Job) bool {
	if _, ok := j.A.(UpdateNixCache); ok {
		return t.shouldSkipQueuedNixCacheJob()
	}

	return false
}

// DetectAndMarkOrphanedJobs reconciles persisted active jobs against runtime state.
// It is used on the periodic orphan scan and during WS bootstrap.
func (t *Dogeboxd) DetectAndMarkOrphanedJobs() ([]string, error) {
	if t.JobManager == nil {
		return nil, nil
	}

	activeJobs, err := t.JobManager.GetActiveJobs()
	if err != nil {
		return nil, err
	}

	runtimeJobIDs := t.GetRuntimeJobIDs()
	runtimeSet := make(map[string]struct{}, len(runtimeJobIDs))
	for _, jobID := range runtimeJobIDs {
		runtimeSet[jobID] = struct{}{}
	}

	orphaned := make([]string, 0)
	for _, job := range activeJobs {
		if _, ok := runtimeSet[job.ID]; ok {
			continue
		}
		if err := t.JobManager.MarkJobOrphaned(job.ID); err != nil {
			return orphaned, err
		}
		orphaned = append(orphaned, job.ID)
	}

	return orphaned, nil
}

func (t Dogeboxd) shouldSkipQueuedNixCacheJob() bool {
	t.queue.jobQLock.Lock()
	defer t.queue.jobQLock.Unlock()

	if len(t.queue.jobQueue) == 0 {
		return false
	}

	lastQueued := t.queue.jobQueue[len(t.queue.jobQueue)-1]
	if _, ok := lastQueued.A.(UpdateNixCache); !ok {
		return false
	}

	// Intentionally ignore the currently running job. Only skip when the
	// next queued job is already a nix cache update.
	return true
}

// Add an Action to the Action queue, returns a unique ID
// which can be used to match the outcome in the Event queue
func (t Dogeboxd) AddAction(a Action) string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println(">> AddAction: Entropic Failure, add more Overminds.")
	}
	id := fmt.Sprintf("%x", b)
	j := Job{A: a, ID: id}
	j.Logger = NewActionLogger(j, "", t)
	t.jobs <- j
	return id
}

/* jobDispatcher handles any incomming Jobs
 * based on their Action type, some to internal
 * helpers and others sent to the system updater
 * for handling.
 */
func (t Dogeboxd) jobDispatcher(j Job) {
	switch a := j.A.(type) {

	// System actions
	case InstallPup:
		t.createPupFromManifest(j, a.PupName, a.PupVersion, a.SourceId, a.Options)
	case InstallPups:
		for i, pup := range a {
			pupJobID := fmt.Sprintf("%s-%d", j.ID, i+1)

			pupJob := Job{
				ID:      pupJobID,
				A:       pup,
				Err:     j.Err,
				Success: j.Success,
				Start:   j.Start,
				Logger:  NewActionLogger(Job{ID: pupJobID}, "", t),
				State:   j.State,
			}
			// Create a separate tracked job for each pup in the batch.
			if record, err := t.createTrackedJobRecord(pupJob); err == nil && record != nil {
				t.SendChange(Change{ID: "internal", Type: "job:created", Update: record})
			}

			t.createPupFromManifest(pupJob, pup.PupName, pup.PupVersion, pup.SourceId, pup.Options)
		}
	case UninstallPup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case PurgePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case EnablePup:
		// Flip Enabled=true immediately (before job executes) so frontend refreshes mid-job show intended state
		if _, err := t.Pups.UpdatePup(a.PupID, PupEnabled(true)); err != nil {
			j.Err = fmt.Sprintf("Failed to set enabled=true: %v", err)
			t.sendFinishedJob("action", j)
			return
		}
		t.sendSystemJobWithPupDetails(j, a.PupID)
	case DisablePup:
		// Flip Enabled=false immediately (before job executes) so frontend refreshes mid-job show intended state
		if _, err := t.Pups.UpdatePup(a.PupID, PupEnabled(false)); err != nil {
			j.Err = fmt.Sprintf("Failed to set enabled=false: %v", err)
			t.sendFinishedJob("action", j)
			return
		}
		t.sendSystemJobWithPupDetails(j, a.PupID)

	// Dogebox actions
	case UpdatePupConfig:
		t.updatePupConfig(j, a)

	case UpdatePupProviders:
		t.updatePupProviders(j, a)

	case UpdatePupHooks:
		t.updatePupHooks(j, a)

	// Pup Update actions
	case CheckPupUpdates:
		t.checkPupUpdates(j, a)

	case UpgradePup:
		t.sendSystemJobWithPupDetails(j, a.PupID)

	case RollbackPupUpgrade:
		t.sendSystemJobWithPupDetails(j, a.PupID)

	case ImportBlockchainData:
		t.enqueue(j)

	// Host Actions
	case UpdatePendingSystemNetwork:
		t.enqueue(j)

	case InitialBootstrap:
		t.enqueue(j)

	case EnableSSH:
		t.enqueue(j)

	case DisableSSH:
		t.enqueue(j)

	case AddSSHKey:
		t.enqueue(j)

	case RemoveSSHKey:
		t.enqueue(j)

	case SaveCustomNix:
		t.enqueue(j)

	case AddBinaryCache:
		t.enqueue(j)

	case RemoveBinaryCache:
		t.enqueue(j)

	case SystemUpdate:
		t.enqueue(j)

	case UpdateTimezone:
		t.enqueue(j)

	case UpdateKeymap:
		t.enqueue(j)

	case UpdateNixCache:
		t.enqueue(j)

	// Pup router actions
	case UpdateMetrics:
		t.Pups.UpdateMetrics(a)

	default:
		fmt.Printf("Unknown action type: %v\n", a)
	}
}

/* This is where we create a 'PupState' from a ManifestID
* and set it to be installed by the SystemUpdater. After
* this point the Pup has entered a managed state and will
* only be installable again after this one has been purged.
*
* Future: support multiple pup instances per manifest
 */
func (t *Dogeboxd) createPupFromManifest(j Job, pupName, pupVersion, sourceId string, pupOptions AdoptPupOptions) {
	// Fetch the correct manifest from the source manager
	manifest, source, err := t.sources.GetSourceManifest(sourceId, pupName, pupVersion)
	if err != nil {
		j.Err = fmt.Sprintf("Couldn't create pup, no manifest: %s", err)
		t.sendFinishedJob("action", j)
		return
	}

	// create a new pup for the manifest
	pupID, err := t.Pups.AdoptPup(manifest, source, pupOptions)
	if err != nil {
		j.Err = fmt.Sprintf("Couldn't create pup: %s", err)
		t.sendFinishedJob("action", j)
		return
	}

	// send the job off to the SystemUpdater to install
	t.sendSystemJobWithPupDetails(j, pupID)
}

// Handle an UpdatePupConfig action
func (t *Dogeboxd) updatePupConfig(j Job, u UpdatePupConfig) {
	log := j.Logger.Step("config")

	// Get state before update to check if we need to auto-enable
	oldState, _, _ := t.Pups.GetPup(u.PupID)
	wasNeedingConfig := oldState.NeedsConf

	newState, err := t.Pups.UpdatePup(u.PupID, SetPupConfig(u.Payload))
	if err != nil {
		j.Err = fmt.Sprintf("couldn't update config for %s: %v", u.PupID, err)
		t.sendFinishedJob("action", j)
		return
	}

	// Write config to secure storage (inside pup container, not exposed on host)
	if err := WritePupConfigToStorage(t.config.DataDir, u.PupID, newState.Config, log); err != nil {
		j.Err = fmt.Sprintf("failed to write config to storage: %v", err)
		t.sendFinishedJob("action", j)
		return
	}

	// Check if config requirements are now satisfied
	healthReport := t.Pups.GetPupHealthState(&newState)
	configNowSatisfied := wasNeedingConfig && !healthReport.NeedsConf && !healthReport.NeedsDeps

	// If config is now satisfied and pup isn't enabled, enable it
	if configNowSatisfied && !newState.Enabled {
		log.Logf("Config requirements satisfied, enabling pup")
		newState, err = t.Pups.UpdatePup(u.PupID, PupEnabled(true))
		if err != nil {
			j.Err = fmt.Sprintf("failed to enable pup after config: %v", err)
			t.sendFinishedJob("action", j)
			return
		}
	}

	// Rebuild nix configuration and restart the pup
	dbxState := t.sm.Get().Dogebox
	nixPatch := t.nix.NewPatch(log)
	t.nix.WritePupFile(nixPatch, newState, dbxState)

	if err := nixPatch.Apply(); err != nil {
		j.Err = fmt.Sprintf("failed to apply configuration: %v", err)
		t.sendFinishedJob("action", j)
		return
	}

	j.Success = newState
	t.sendFinishedJob("action", j)
}

// Handle an UpdatePupProviders action
func (t *Dogeboxd) updatePupProviders(j Job, u UpdatePupProviders) {
	log := j.Logger.Step("update providers")
	_, err := t.Pups.UpdatePup(u.PupID, SetPupProviders(u.Payload))
	if err != nil {
		j.Err = fmt.Sprintf("Couldnt update: %s", u.PupID)
		t.sendFinishedJob("action", j)
		return
	}

	pupState, _, err := t.Pups.GetPup(u.PupID)
	j.Success = pupState
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	canPupStart, err := t.Pups.CanPupStart(u.PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	// If the pup may now start, update all of our nix files and rebuild.
	if canPupStart {
		dbxState := t.sm.Get().Dogebox

		nixPatch := t.nix.NewPatch(log)
		t.nix.UpdateSystemContainerConfiguration(nixPatch)
		t.nix.WritePupFile(nixPatch, pupState, dbxState)

		if err := nixPatch.Apply(); err != nil {
			j.Err = fmt.Sprintf("Failed to apply nix patch: %v", err)
			t.sendFinishedJob("action", j)
			return
		}
	}

	t.sendFinishedJob("action", j)
}

// Handle an UpdatePupHooks action
func (t *Dogeboxd) updatePupHooks(j Job, u UpdatePupHooks) {
	_, err := t.Pups.UpdatePup(u.PupID, SetPupHooks(u.Payload))
	if err != nil {
		j.Err = fmt.Sprintf("Couldnt update: %s", u.PupID)
		t.sendFinishedJob("action", j)
		return
	}

	j.Success, _, err = t.Pups.GetPup(u.PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}
	t.sendFinishedJob("action", j)
}

// Handle a CheckPupUpdates action
func (t *Dogeboxd) checkPupUpdates(j Job, c CheckPupUpdates) {
	log := j.Logger.Step("check-pup-updates")

	// Handle errors and send result (deferred to avoid duplication)
	defer func() { t.sendFinishedJob("action", j) }()

	if c.PupID == "" {
		// Check all pups
		log.Logf("Starting update check for all installed pups")
		allPups := t.Pups.GetStateMap()
		pupCount := len(allPups)
		log.Logf("Found %d installed pups to check", pupCount)

		if pupCount == 0 {
			log.Logf("No pups installed to check")
			j.Success = map[string]interface{}{
				"message":      "No pups installed",
				"pupsChecked":  0,
				"updatesFound": 0,
				"updateInfo":   map[string]interface{}{},
			}
			return
		}

		results := t.PupUpdateChecker.CheckAllPupUpdates()

		// Check if any pups were successfully checked
		if len(results) == 0 {
			log.Errf("Failed to check any of %d installed pup(s)", pupCount)
			j.Err = fmt.Sprintf("Failed to check any of %d installed pup(s)", pupCount)
			return
		}

		updatesFound := 0
		for _, info := range results {
			if info.UpdateAvailable {
				updatesFound++
			}
		}

		failedCount := pupCount - len(results)
		if failedCount > 0 {
			log.Logf("Update check complete: %d pup(s) with updates available out of %d checked (%d failed)",
				updatesFound, len(results), failedCount)
		} else {
			log.Logf("Update check complete: %d pup(s) with updates available out of %d checked",
				updatesFound, len(results))
		}

		j.Success = map[string]interface{}{
			"message":      "Update check completed",
			"pupsChecked":  len(results),
			"pupsFailed":   failedCount,
			"updatesFound": updatesFound,
			"updateInfo":   results,
		}
	} else {
		// Check specific pup
		log.Logf("Starting update check for pup: %s", c.PupID)

		info, err := t.PupUpdateChecker.CheckForUpdates(c.PupID)
		if err != nil {
			log.Errf("Failed to check updates for pup %s: %v", c.PupID, err)
			j.Err = fmt.Sprintf("Update check failed: %v", err)
			return
		}

		if info.UpdateAvailable {
			log.Logf("Update available for %s: %s → %s", c.PupID, info.CurrentVersion, info.LatestVersion)
		} else {
			log.Logf("No updates available for %s (current: %s)", c.PupID, info.CurrentVersion)
		}

		j.Success = map[string]interface{}{
			"message":    "Update check completed",
			"pupId":      c.PupID,
			"updateInfo": info,
		}
	}
}

// SendChange sends a change to the websocket relay without blocking if the channel is full.
func (t Dogeboxd) SendChange(c Change) {
	// Attach ordering metadata for client-side staleness protection.
	c.Seq = atomic.AddUint64(&globalChangeSeq, 1)
	c.TS = time.Now().UnixMilli()

	timer := time.After(200 * time.Millisecond)
	select {
	case t.Changes <- c:
	case <-timer:
		fmt.Println("Can't sent change, no receiver", c)
	}
}

// helper to report a completed job back to the client
func (t Dogeboxd) sendFinishedJob(changeType string, j Job) {
	if j.Err != "" {
		j.Logger.Step("queue").Err(j.Err)
	}

	// Update job record as completed/failed for immediate jobs (those that don't go through SystemUpdater)
	// This ensures jobs like UpdatePupProviders get properly marked as completed
	// Only call CompleteJob if the job is still active (not already completed by SystemUpdater path)
	jobWasActive := false
	if t.JobManager != nil && t.shouldTrackJob(j) && t.JobManager.IsJobActive(j.ID) {
		jobWasActive = true
		err := t.JobManager.CompleteJob(j.ID, j.Err)
		if err == nil {
			jobRecord, getErr := t.JobManager.GetJob(j.ID)
			if getErr == nil {
				t.SendChange(Change{ID: "internal", Type: "job:completed", Update: jobRecord})
			}
		}
	}
	// Keep direct-completion jobs runtime-visible until their DB row has left the
	// active states, so concurrent orphan scans cannot observe a false orphan.
	t.clearNonQueuedActiveJob(j.ID)

	// Only send "action" event for jobs that were NOT already completed by JobManager
	// Jobs completed by SystemUpdater (like upgrade) already send job:completed events
	// and don't need a redundant "action" event
	if t.JobManager == nil || !t.shouldTrackJob(j) || jobWasActive {
		t.SendChange(Change{ID: j.ID, Error: j.Err, Type: changeType, Update: j.Success})
	}
}

// shouldTrackJob determines if a job should create a visible job record
// Excludes routine background operations that users don't need to see
func (t Dogeboxd) shouldTrackJob(j Job) bool {
	switch j.A.(type) {
	case UpdateMetrics:
		return false // Metrics updates happen every 10s, don't track
	case UpdatePupConfig:
		return false // Config updates are instantaneous, don't need tracking
	case UpdatePupHooks:
		return false // Hook updates are instantaneous
	case InstallPups:
		return false // Individual sub-jobs are tracked separately in jobDispatcher
	default:
		return true // Track everything else
	}
}

// updates the client on the progress of any inflight actions
func (t Dogeboxd) sendProgress(p ActionProgress) {
	// Update job record with progress
	if t.JobManager != nil {
		err := t.JobManager.UpdateJobProgress(p)
		if err == nil {
			jobRecord, getErr := t.JobManager.GetJob(p.ActionID)
			if getErr == nil {
				t.SendChange(Change{ID: "internal", Type: "job:updated", Update: jobRecord})
			}
		}
	}

	t.SendChange(Change{ID: p.ActionID, Type: "progress", Update: p})
}

// helper to attach PupState to a job and send it to the SystemUpdater
func (t Dogeboxd) sendSystemJobWithPupDetails(j Job, PupID string) {
	p, _, err := t.Pups.GetPup(PupID)
	if err != nil {
		j.Err = err.Error()
		t.sendFinishedJob("action", j)
		return
	}

	j.State = &p
	j.Logger.PupID = PupID

	// Update the job record with the pupID (important for install jobs where pupID wasn't known at creation)
	if err := t.JobManager.UpdateJobPupID(j.ID, PupID); err != nil {
		// Log but don't fail - this is a non-critical update
		fmt.Printf("Warning: failed to update job record pupID: %v\n", err)
	}

	// Send job to the system updater for handling
	t.enqueue(j)
}

// WritePupConfigToStorage writes the pup's user configuration to a secure file
// in the pup's storage directory. This file is loaded by systemd via EnvironmentFile
// directive, keeping sensitive config values (like passwords) out of the nix files.
func WritePupConfigToStorage(dataDir string, pupID string, config map[string]string, log SubLogger) error {
	// Convert config map to JSON
	configJSON, err := configToJSON(config)
	if err != nil {
		if log != nil {
			log.Errf("Failed to serialize config to JSON: %v", err)
		}
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	cmd := exec.Command("sudo", "_dbxroot", "pup", "write-config",
		"--data-dir", dataDir,
		"--pupId", pupID,
		"--config", configJSON,
	)

	if log != nil {
		log.Logf("Writing pup config to storage")
		log.LogCmd(cmd)
	}

	if err := cmd.Run(); err != nil {
		if log != nil {
			log.Errf("Failed to write pup config: %v", err)
		}
		return fmt.Errorf("failed to write pup config: %w", err)
	}

	return nil
}

// configToJSON converts a config map to JSON string for passing to _dbxroot
func configToJSON(config map[string]string) (string, error) {
	if config == nil {
		return "{}", nil
	}

	// Use encoding/json to properly escape values
	bytes, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

var allowedJournalServices = map[string]string{
	"dbx": "dogeboxd.service",
	"dkm": "dkm.service",
}

type logSource struct {
	journalService string
	filePath       string
}

func (s logSource) usesJournal() bool {
	return s.journalService != ""
}

func (t Dogeboxd) resolvePupLogSource(PupID string) (logSource, error) {
	// We read dogeboxd and dkm from the host systemd journal,
	// and read everything else (pups) from the container logs we export.
	service, ok := allowedJournalServices[PupID]
	if ok {
		return logSource{journalService: service}, nil
	}

	// Check that we've actually got a valid pup id.
	_, _, err := t.Pups.GetPup(PupID)
	if err != nil {
		return logSource{}, err
	}

	return logSource{filePath: t.config.PupLogPath(PupID)}, nil
}

func (t Dogeboxd) resolveJobLogSource(JobID string) (logSource, error) {
	_, err := t.JobManager.GetJob(JobID)
	if err != nil {
		return logSource{}, fmt.Errorf("job not found: %s", JobID)
	}

	return logSource{filePath: t.config.JobLogPath(JobID)}, nil
}

func (t Dogeboxd) getLogChannel(source logSource, resumeToken *string) (context.CancelFunc, chan string, error) {
	if source.usesJournal() {
		if resumeToken != nil {
			return t.JournalReader.GetJournalChannelFromCursor(source.journalService, *resumeToken)
		}
		return t.JournalReader.GetJournalChannel(source.journalService)
	}

	if resumeToken != nil {
		offset, err := parseLogOffsetResumeToken(*resumeToken)
		if err != nil {
			return nil, nil, err
		}
		return t.logtailer.GetChannelFromOffset(source.filePath, offset)
	}

	return t.logtailer.GetChannel(source.filePath)
}

func (t Dogeboxd) getLogPage(source logSource, before *string, limit int) (LogPage, error) {
	if limit <= 0 {
		return LogPage{}, fmt.Errorf("Log tail limit must be greater than zero")
	}

	if source.usesJournal() {
		return t.JournalReader.GetJournalPage(source.journalService, before, limit)
	}

	if before != nil {
		offset, err := parseLogOffsetResumeToken(*before)
		if err != nil {
			return LogPage{}, err
		}
		return t.logtailer.GetPage(source.filePath, &offset, limit)
	}

	return t.logtailer.GetPage(source.filePath, nil, limit)
}

func (t Dogeboxd) GetLogChannel(PupID string, resumeToken *string) (context.CancelFunc, chan string, error) {
	source, err := t.resolvePupLogSource(PupID)
	if err != nil {
		return nil, nil, err
	}

	return t.getLogChannel(source, resumeToken)
}

func (t Dogeboxd) GetLogTail(PupID string, limit int) ([]string, *string, error) {
	page, err := t.GetLogPage(PupID, nil, limit)
	if err != nil {
		return nil, nil, err
	}

	return page.Lines, page.ResumeToken, nil
}

func (t Dogeboxd) GetLogPage(PupID string, before *string, limit int) (LogPage, error) {
	source, err := t.resolvePupLogSource(PupID)
	if err != nil {
		return LogPage{}, err
	}

	return t.getLogPage(source, before, limit)
}

// GetJobLogChannel returns a log channel for a specific job
// Streams logs from the job's ActionLogger in real-time (same system as pup logs)
func (t Dogeboxd) GetJobLogChannel(JobID string, resumeToken *string) (context.CancelFunc, chan string, error) {
	source, err := t.resolveJobLogSource(JobID)
	if err != nil {
		return nil, nil, err
	}

	return t.getLogChannel(source, resumeToken)
}

func (t Dogeboxd) GetJobLogTail(JobID string, limit int) ([]string, *string, error) {
	page, err := t.GetJobLogPage(JobID, nil, limit)
	if err != nil {
		return nil, nil, err
	}

	return page.Lines, page.ResumeToken, nil
}

func (t Dogeboxd) GetJobLogPage(JobID string, before *string, limit int) (LogPage, error) {
	source, err := t.resolveJobLogSource(JobID)
	if err != nil {
		return LogPage{}, err
	}

	return t.getLogPage(source, before, limit)
}

func parseLogOffsetResumeToken(resumeToken string) (int64, error) {
	offset, err := strconv.ParseInt(resumeToken, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid log resume token")
	}
	if offset < 0 {
		offset = 0
	}

	return offset, nil
}

func logOffsetResumeToken(offset int64) *string {
	resumeToken := strconv.FormatInt(offset, 10)
	return &resumeToken
}
