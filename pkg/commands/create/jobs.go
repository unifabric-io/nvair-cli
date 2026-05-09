package create

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

var (
	waitForJobsPollInterval            = 2 * time.Second
	waitForJobsMaxWaitTime             = 10 * time.Minute
	waitForSimulationStatePollInterval = 5 * time.Second
	waitForSimulationStateMaxWaitTime  = 15 * time.Minute
)

// WaitForJobs waits for all specified jobs to reach a terminal state (COMPLETE, FAILED, or CANCELLED).
// It polls each job every 2 seconds up to a maximum of 10 minutes.
func (cc *Command) WaitForJobs(apiClient *api.Client, jobIDs []string) error {
	logging.Verbose("WaitForJobs: Starting to monitor %d jobs", len(jobIDs))
	startTime := time.Now()
	jobStates := make(map[string]string)

	for _, jobID := range jobIDs {
		jobStates[jobID] = "PENDING"
	}

	for {
		if time.Since(startTime) > waitForJobsMaxWaitTime {
			logging.Verbose("WaitForJobs: Timeout waiting for jobs after %v", time.Since(startTime))
			incompleteJobs := []string{}
			for id, state := range jobStates {
				if state != "COMPLETE" && state != "FAILED" && state != "CANCELLED" {
					incompleteJobs = append(incompleteJobs, id)
				}
			}
			return fmt.Errorf("timeout waiting for jobs to complete (waited %v). Incomplete jobs: %v", time.Since(startTime), incompleteJobs)
		}

		allComplete := true

		for _, jobID := range jobIDs {
			job, err := apiClient.GetJob(jobID)
			if err != nil {
				logging.Verbose("WaitForJobs: Error fetching job %s: %v", jobID, err)
				allComplete = false
				continue
			}

			jobStates[jobID] = job.State
			logging.Verbose("WaitForJobs: Job %s state: %s", jobID, job.State)

			if job.State != "COMPLETE" && job.State != "FAILED" && job.State != "CANCELLED" {
				allComplete = false
			}

			if job.State == "FAILED" {
				logging.Verbose("WaitForJobs: Job %s failed", jobID)
				return fmt.Errorf("job %s failed", jobID)
			}

			if job.State == "CANCELLED" {
				logging.Verbose("WaitForJobs: Job %s was cancelled", jobID)
				return fmt.Errorf("job %s was cancelled", jobID)
			}
		}

		if allComplete {
			logging.Verbose("WaitForJobs: All jobs completed successfully")
			return nil
		}

		time.Sleep(waitForJobsPollInterval)
	}
}

// WaitForSimulationState polls a simulation until it reaches the desired state.
func (cc *Command) WaitForSimulationState(apiClient *api.Client, simulationID, desiredState string) error {
	desiredState = strings.ToUpper(strings.TrimSpace(desiredState))
	if desiredState == "" {
		return fmt.Errorf("desired simulation state is required")
	}

	logging.Verbose("WaitForSimulationState: Waiting for simulation %s to reach state %s", simulationID, desiredState)
	startTime := time.Now()
	lastState := ""

	for {
		if time.Since(startTime) > waitForSimulationStateMaxWaitTime {
			return fmt.Errorf("timeout waiting for simulation %s to reach state %s (waited %v, last observed state: %s)", simulationID, desiredState, time.Since(startTime), lastState)
		}

		sim, err := apiClient.GetSimulationByID(simulationID)
		if err != nil {
			logging.Verbose("WaitForSimulationState: Error fetching simulation %s: %v", simulationID, err)
			if !isRetryableSimulationStateError(err) {
				return fmt.Errorf("failed to fetch simulation %s while waiting for state %s: %w", simulationID, desiredState, err)
			}
			time.Sleep(waitForSimulationStatePollInterval)
			continue
		}

		lastState = sim.State
		logging.Verbose("WaitForSimulationState: Simulation %s state: %s", simulationID, sim.State)

		if strings.EqualFold(sim.State, desiredState) {
			return nil
		}
		if strings.EqualFold(sim.State, "INVALID") {
			return fmt.Errorf("simulation %s entered INVALID state while waiting for %s", simulationID, desiredState)
		}

		time.Sleep(waitForSimulationStatePollInterval)
	}
}

func isRetryableSimulationStateError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := err.Error()
	return strings.HasPrefix(msg, "request failed after ") || strings.HasPrefix(msg, "transient error after ")
}
