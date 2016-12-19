package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/klauspost/cpuid"
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

func (s networkSource) Name() string { return "network" }

// reading the network card details from sysfs and determining if SR-IOV is enabled for each of the network interfaces
func (s networkSource) Discover() ([]string, error) {
	features := []string{}
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("can't obtain the network interfaces details: %s", err.Error())
	}
	// iterating through network interfaces to obtain their respective number of virtual functions
	for _, netInterface := range netInterfaces {
		if strings.Contains(netInterface.Flags.String(), "up") && !strings.Contains(netInterface.Flags.String(), "loopback") {
			totalVfsPath := "/sys/class/net/" + netInterface.Name + "/device/sriov_totalvfs"
			totalBytes, err := ioutil.ReadFile(totalVfsPath)
			if err != nil {
				stderrLogger.Printf("SR-IOV not supported for network interface: %s: %s", netInterface.Name, err.Error())
				continue
			}
			total := bytes.TrimSpace(totalBytes)
			t, err := strconv.Atoi(string(total))
			if err != nil {
				stderrLogger.Printf("Error in obtaining maximum supported number of virtual functions for network interface: %s: %s", netInterface.Name, err.Error())
				continue
			}
			if t > 0 {
				stdoutLogger.Printf("SR-IOV capability is detected on the network interface: %s", netInterface.Name)
				stdoutLogger.Printf("%d maximum supported number of virtual functions on network interface: %s", t, netInterface.Name)
				features = append(features, "sriov")
				numVfsPath := "/sys/class/net/" + netInterface.Name + "/device/sriov_numvfs"
				numBytes, err := ioutil.ReadFile(numVfsPath)
				if err != nil {
					stderrLogger.Printf("SR-IOV not configured for network interface: %s: %s", netInterface.Name, err.Error())
					continue
				}
				num := bytes.TrimSpace(numBytes)
				n, err := strconv.Atoi(string(num))
				if err != nil {
					stderrLogger.Printf("Error in obtaining the configured number of virtual functions for network interface: %s: %s", netInterface.Name, err.Error())
					continue
				}
				if n > 0 {
					stderrLogger.Printf("%d virtual functions configured on network interface: %s", n, netInterface.Name)
					features = append(features, "sriov-configured")
					break
				} else if n == 0 {
					stderrLogger.Printf("SR-IOV not configured on network interface: %s", netInterface.Name)
				}
			}
		}
	}
	return features, nil
}
