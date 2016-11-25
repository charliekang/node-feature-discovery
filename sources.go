package main

import (
	"bytes"
	"fmt"
	"github.com/klauspost/cpuid"
	"io/ioutil"
	"os/exec"
	"path"
	"strconv"
)

// FeatureSource represents a source of discovered node features.
type FeatureSource interface {
	// Returns a friendly name for this source of node features.
	Name() string

	// Returns discovered features for this node.
	Discover() ([]string, error)
}

const (
	// RDTBin is the path to RDT detection helpers.
	RDTBin = "/go/src/github.com/kubernetes-incubator/node-feature-discovery/rdt-discovery"
)

////////////////////////////////////////////////////////////////////////////////
// CPUID Source

// Implements main.FeatureSource.
type cpuidSource struct{}

func (s cpuidSource) Name() string { return "cpuid" }

// Returns feature names for all the supported CPU features.
func (s cpuidSource) Discover() ([]string, error) {
	// Get the cpu features as strings
	return cpuid.CPU.Features.Strings(), nil
}

////////////////////////////////////////////////////////////////////////////////
// RDT (Intel Resource Director Technology) Source

// Implements main.FeatureSource.
type rdtSource struct{}

func (s rdtSource) Name() string { return "rdt" }

// Returns feature names for CMT and CAT if suppported.
func (s rdtSource) Discover() ([]string, error) {
	features := []string{}

	cmd := exec.Command("bash", "-c", path.Join(RDTBin, "mon-discovery"))
	if err := cmd.Run(); err != nil {
		stderrLogger.Printf("support for RDT monitoring was not detected: %s", err.Error())
	} else {
		// RDT monitoring detected.
		features = append(features, "RDTMON")
	}

	cmd = exec.Command("bash", "-c", path.Join(RDTBin, "l3-alloc-discovery"))
	if err := cmd.Run(); err != nil {
		stderrLogger.Printf("support for RDT L3 allocation was not detected: %s", err.Error())
	} else {
		// RDT L3 cache allocation detected.
		features = append(features, "RDTL3CA")
	}

	cmd = exec.Command("bash", "-c", path.Join(RDTBin, "l2-alloc-discovery"))
	if err := cmd.Run(); err != nil {
		stderrLogger.Printf("support for RDT L2 allocation was not detected: %s", err.Error())
	} else {
		// RDT L2 cache allocation detected.
		features = append(features, "RDTL2CA")
	}

	return features, nil
}

////////////////////////////////////////////////////////////////////////////////
// PState Source

// Implements main.FeatureSource.
type pstateSource struct{}

func (s pstateSource) Name() string { return "pstate" }

// Returns feature names for p-state related features such as turbo boost.
func (s pstateSource) Discover() ([]string, error) {
	features := []string{}

	// Only looking for turbo boost for now...
	bytes, err := ioutil.ReadFile("/sys/devices/system/cpu/intel_pstate/no_turbo")
	if err != nil {
		return nil, fmt.Errorf("can't detect whether turbo boost is enabled: %s", err.Error())
	}
	if bytes[0] == byte('0') {
		// Turbo boost is enabled.
		features = append(features, "turbo")
	}

	return features, nil
}

////////////////////////////////////////////////////////////////////////////////
// Network Source

// Implements main.FeatureSource.
type networkSource struct{}

func (s networkSource) Name() string { return "netid" }
func (s networkSource) Discover() ([]string, error) {
	features := []string{}
	var total_num_vfs int = 0
	// sysfs of the node mounted as /hostsys inside the pod
	netInterfaces, err := ioutil.ReadDir("/hostsys/class/net") //does not return . and ..
	if err != nil {
		return nil, fmt.Errorf("can't obtain the network interfaces: %s", err.Error())
	}
	for _, netInterface := range netInterfaces {
		//forming the file name : /hostsys/class/net/<network interface>/device/sriov_numvfs
		filename := "/hostsys/class/net/" + netInterface.Name() + "/device/sriov_numvfs"
		bytes_received, err := ioutil.ReadFile(filename)
		if err != nil {
			// interface does not support SRIOV
			continue // proceeding to next iteration for next network interface
		}
		//removing white spaces
		num := bytes.TrimSpace(bytes_received)
		//checking if 0 bytes read
		if l := len(bytes_received); l == 0 {
			// Zero bytes read for this file
			continue // proceeding to next iteration for next network interface
		}
		//converting bytes to integer and checking for error
		n, err := strconv.Atoi(string(num))
		if err != nil {
			// Receiving non-integral value
			continue // proceeding to next iteration for next network interface
		}
		total_num_vfs += n
	}
	if total_num_vfs > 0 {
		features = append(features, "SRIOV")
	}
	return features, nil
}
