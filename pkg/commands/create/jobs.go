package create

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	nodeutil "github.com/unifabric-io/nvair-cli/pkg/node"
)

var (
	waitForJobsPollInterval              = 2 * time.Second
	waitForJobsMaxWaitTime               = 10 * time.Minute
	waitForSimulationStatePollInterval   = 5 * time.Second
	waitForSimulationStateMaxWaitTime    = 15 * time.Minute
	waitForNodeManagementIPsPollInterval = 5 * time.Second
	waitForNodeManagementIPsMaxWaitTime  = 10 * time.Minute
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

// WaitForNodeManagementIPs polls GetNodes until every node (other than
// out-of-band management nodes, i.e. oob-mgmt-server, which is reached via
// its own outbound interface rather than a management IP, and
// oob-mgmt-switch* nodes, which are not expected to have a management IP
// assigned) reports a management IP. The API can take a short while after a
// simulation becomes ACTIVE before management IPs are actually assigned, so
// callers must not assume the node list returned right after ACTIVE is
// final. It returns the freshest node list once all required nodes have an
// IP.
func (cc *Command) WaitForNodeManagementIPs(apiClient *api.Client, simulationID string, nodes []api.Node) ([]api.Node, error) {
	logging.Verbose("WaitForNodeManagementIPs: Waiting for management IPs on nodes for simulation %s", simulationID)
	startTime := time.Now()
	current := nodes

	for {
		missing := nodesMissingManagementIP(current)
		if len(missing) == 0 {
			return current, nil
		}

		if time.Since(startTime) > waitForNodeManagementIPsMaxWaitTime {
			return nil, fmt.Errorf("timeout waiting for management IPs to be assigned (waited %v). Missing on: %s", time.Since(startTime), strings.Join(missing, ", "))
		}

		logging.Verbose("WaitForNodeManagementIPs: Still missing management IPs on: %s", strings.Join(missing, ", "))
		time.Sleep(waitForNodeManagementIPsPollInterval)

		refreshed, err := apiClient.GetNodes(simulationID)
		if err != nil {
			logging.Verbose("WaitForNodeManagementIPs: Error refetching nodes: %v", err)
			continue
		}
		current = refreshed
	}
}

func nodesMissingManagementIP(nodes []api.Node) []string {
	var missing []string
	for _, n := range nodes {
		if isOOBMgmtNode(n.Name) {
			continue
		}
		mgmtIP, err := nodeutil.ResolveMgmtIP(n)
		if err != nil || mgmtIP == "" {
			missing = append(missing, n.Name)
		}
	}
	return missing
}

// isOOBMgmtNode reports whether the node is an out-of-band management node
// (the mgmt server, reached via its outbound interface, or an mgmt switch,
// e.g. "oob-mgmt-switch-leaf-1") that is not expected to have a management
// IP assigned like other switches/nodes.
func isOOBMgmtNode(name string) bool {
	return name == constant.OOBMgmtServerName || strings.HasPrefix(name, constant.OOBMgmtSwitchName)
}

func isRetryableSimulationStateError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := err.Error()
	return strings.HasPrefix(msg, "request failed after ") || strings.HasPrefix(msg, "transient error after ")
}
