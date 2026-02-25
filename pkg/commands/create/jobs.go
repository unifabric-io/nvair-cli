package create

import (
	"fmt"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

// WaitForJobs waits for all specified jobs to reach a terminal state (COMPLETE, FAILED, or CANCELLED).
// It polls each job every 2 seconds up to a maximum of 10 minutes.
func (cc *Command) WaitForJobs(apiClient *api.Client, jobIDs []string) error {
	const (
		pollInterval = 2 * time.Second
		maxWaitTime  = 10 * time.Minute
	)

	logging.Verbose("WaitForJobs: Starting to monitor %d jobs", len(jobIDs))
	startTime := time.Now()
	jobStates := make(map[string]string)

	for _, jobID := range jobIDs {
		jobStates[jobID] = "PENDING"
	}

	for {
		if time.Since(startTime) > maxWaitTime {
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

		time.Sleep(pollInterval)
	}
}
