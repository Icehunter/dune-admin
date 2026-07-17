package main

import (
	"fmt"
	"sync"
)

// db_restore_job.go runs full-database restores as background jobs with real,
// per-step progress the frontend polls — the restore-progress dialog's
// checkmarks reflect what the backend actually did, not client-side timers.
// A restore spans stopping game shards (up to the container stop timeout) and
// pg_restore (minutes on a real database), far beyond what a single blocking
// HTTP request should hold open.

// restoreStepState is one step's key + status in a dbRestoreStatus, in
// display order. Statuses are the restoreStatus* constants plus "pending".
type restoreStepState struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

// dbRestoreStatus is the polled progress snapshot for one server scope's
// restore job.
type dbRestoreStatus struct {
	Running        bool               `json:"running"`
	Done           bool               `json:"done"`
	Failed         bool               `json:"failed"`
	File           string             `json:"file"`
	Steps          []restoreStepState `json:"steps"`
	Error          string             `json:"error"`
	Output         string             `json:"output"`
	IgnoredErrors  int                `json:"ignored_errors"`
	ServersStopped bool               `json:"servers_stopped"`
}

// restoreStepOrder is the fixed display order of restore steps.
var restoreStepOrder = []string{restoreStepCheck, restoreStepStop, restoreStepRestore, restoreStepFinalize}

// pendingRestoreSteps returns a fresh all-pending step list.
func pendingRestoreSteps() []restoreStepState {
	steps := make([]restoreStepState, 0, len(restoreStepOrder))
	for _, k := range restoreStepOrder {
		steps = append(steps, restoreStepState{Key: k, Status: "pending"})
	}
	return steps
}

// dbRestoreJobs tracks at most one restore job per server scope.
type dbRestoreJobs struct {
	mu   sync.Mutex
	jobs map[string]*dbRestoreJob
}

type dbRestoreJob struct {
	status dbRestoreStatus
	done   chan struct{}
}

func newDBRestoreJobs() *dbRestoreJobs {
	return &dbRestoreJobs{jobs: map[string]*dbRestoreJob{}}
}

// globalRestoreJobs is the process-wide job registry, shared by the Database
// and Battlegroup restore handlers and the status endpoint.
var globalRestoreJobs = newDBRestoreJobs()

// Status returns a copy of the scope's job status; an idle all-pending status
// when no job has run yet.
func (j *dbRestoreJobs) Status(scope string) dbRestoreStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	job, ok := j.jobs[scope]
	if !ok {
		return dbRestoreStatus{Steps: pendingRestoreSteps()}
	}
	st := job.status
	st.Steps = append([]restoreStepState(nil), job.status.Steps...)
	return st
}

// Start launches run in a background goroutine for scope, wiring its report
// callback into the tracked status. Returns an error (no goroutine spawned)
// when a job is already running for that scope.
func (j *dbRestoreJobs) Start(scope, file string, run func(report func(step, status string)) (dbRestoreResult, error)) error {
	j.mu.Lock()
	if existing, ok := j.jobs[scope]; ok && existing.status.Running {
		j.mu.Unlock()
		return fmt.Errorf("a restore is already running for this server")
	}
	job := &dbRestoreJob{
		status: dbRestoreStatus{Running: true, File: file, Steps: pendingRestoreSteps()},
		done:   make(chan struct{}),
	}
	j.jobs[scope] = job
	j.mu.Unlock()

	report := func(step, status string) {
		j.mu.Lock()
		defer j.mu.Unlock()
		for i := range job.status.Steps {
			if job.status.Steps[i].Key == step {
				job.status.Steps[i].Status = status
				return
			}
		}
	}

	go func() {
		defer close(job.done)
		res, err := run(report)
		j.mu.Lock()
		defer j.mu.Unlock()
		job.status.Running = false
		job.status.Done = true
		job.status.Output = res.Output
		job.status.IgnoredErrors = res.IgnoredErrors
		job.status.ServersStopped = res.ServersStopped
		if err != nil {
			job.status.Failed = true
			job.status.Error = err.Error()
		}
	}()
	return nil
}

// wait blocks until the scope's current job goroutine finishes. Test helper —
// production callers poll Status instead.
func (j *dbRestoreJobs) wait(scope string) {
	j.mu.Lock()
	job, ok := j.jobs[scope]
	j.mu.Unlock()
	if ok {
		<-job.done
	}
}
