package manager

import (
	"github.com/Sirupsen/logrus"
	"github.com/contiv/cluster/management/src/inventory"
	"github.com/contiv/errored"
)

func nodeNotExistsError(nameOrAddr string) error {
	return errored.Errorf("node with name or address %q doesn't exists", nameOrAddr)
}

func nodeConfigNotExistsError(name string) error {
	return errored.Errorf("the configuration info for node %q doesn't exist", name)
}

func nodeInventoryNotExistsError(name string) error {
	return errored.Errorf("the inventory info for node %q doesn't exist", name)
}

func (m *Manager) findNode(name string) (*node, error) {
	n, ok := m.nodes[name]
	if !ok {
		return nil, nodeNotExistsError(name)
	}
	return n, nil
}

func (m *Manager) findNodeByMgmtAddr(addr string) (*node, error) {
	for _, node := range m.nodes {
		if node.Mon.GetMgmtAddress() == addr {
			return node, nil
		}
	}
	return nil, nodeNotExistsError(addr)
}

func (m *Manager) isMasterNode(name string) (bool, error) {
	n, err := m.findNode(name)
	if err != nil {
		return false, err
	}
	if n.Cfg == nil {
		return false, nodeConfigNotExistsError(name)
	}
	return n.Cfg.GetGroup() == ansibleMasterGroupName, nil
}

func (m *Manager) isWorkerNode(name string) (bool, error) {
	n, err := m.findNode(name)
	if err != nil {
		return false, err
	}
	if n.Cfg == nil {
		return false, nodeConfigNotExistsError(name)
	}
	return n.Cfg.GetGroup() == ansibleWorkerGroupName, nil
}

func (m *Manager) isDiscoveredNode(name string) (bool, error) {
	n, err := m.findNode(name)
	if err != nil {
		return false, err
	}
	if n.Inv == nil {
		return false, nodeInventoryNotExistsError(name)
	}
	_, state := n.Inv.GetStatus()
	return state == inventory.Discovered, nil
}

// areDiscovered checks if all nodes are in discovered state.
// Returns nil error if all nodes are discovered, else returns appropriate error
func (m *Manager) areDiscoveredNodes(names []string) error {
	disappearedNodes := []string{}
	for _, name := range names {
		discovered, err := m.isDiscoveredNode(name)
		if err != nil {
			return err
		}
		if !discovered {
			disappearedNodes = append(disappearedNodes, name)
		}
	}
	if len(disappearedNodes) > 0 {
		return errored.Errorf("one or more nodes are not in discovered state, please check their network reachability. Non-discovered nodes: %v", disappearedNodes)
	}
	return nil
}

func (m *Manager) isDiscoveredAndAllocatedNode(name string) (bool, error) {
	n, err := m.findNode(name)
	if err != nil {
		return false, err
	}
	if n.Inv == nil {
		return false, nodeInventoryNotExistsError(name)
	}
	status, state := n.Inv.GetStatus()
	return state == inventory.Discovered && status == inventory.Allocated, nil
}

type setInvStateCallback func(name string) error

// tries to set the newStatus as state of all assets, it continues on failures
func (m *Manager) setAssetsStatusBestEffort(names []string, newStatusCb setInvStateCallback) {
	for _, name := range names {
		if err := newStatusCb(name); err != nil {
			logrus.Errorf("failed to update %s's state in inventory, Error: %v", name, err)
			continue
		}
	}
}

// try to atomically set the newStatus as state of all assets or revert to revertStatus in case of failure
func (m *Manager) setAssetsStatusAtomic(names []string, newStatusCb setInvStateCallback, revertStatusCb setInvStateCallback) error {
	for i, name := range names {
		if err := newStatusCb(name); err != nil {
			// try to revert back to original state in case of failure
			m.setAssetsStatusBestEffort(names[0:i+1], revertStatusCb)
			return errored.Errorf("failed to update %s's state in inventory, Error: %v", name, err)
		}
	}
	return nil
}

// checkAndGetNewJob() is a wrapper to check that there are no active jobs before a job is run
func (m *Manager) checkAndSetActiveJob(jobDesc string, runner JobRunner, doneCb DoneCallback) error {
	if m.activeJob != nil {
		return errActiveJob(m.activeJob.String())
	}
	m.activeJob = NewJob(jobDesc, runner, doneCb)
	return nil
}

// resetActiveJob() is a helper to reset active jobs if any
func (m *Manager) resetActiveJob() {
	if m.activeJob != nil {
		m.lastJob = m.activeJob
	}
	m.activeJob = nil
}

// runActiveJob() is a wrapper to run the job and reset the active job once the actual job is done
func (m *Manager) runActiveJob() {
	if m.activeJob == nil {
		logrus.Errorf("run called without an active job")
		return
	}
	m.activeJob.Run()
	// reset the active job once done
	m.resetActiveJob()
}

// IsValidHostGroup checks if the passed hostGroup is valid
func IsValidHostGroup(hostGroup string) bool {
	switch hostGroup {
	case
		ansibleMasterGroupName,
		ansibleWorkerGroupName:
		return true
	}
	return false
}
