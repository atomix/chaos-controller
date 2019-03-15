/*
 * Copyright (c) 2019-present Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"bytes"
	"context"
	"docker.io/go-docker"
	dockertypes "docker.io/go-docker/api/types"
	"docker.io/go-docker/api/types/filters"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"os/exec"
	"strings"
)

// getInterfaces returns the virtual interfaces attached to each container in the pod
// to which the given NetworkPartition refers.
func getInterfaces(name types.NamespacedName) ([]string, error) {
	// Create a new Docker client.
	cli, err := docker.NewEnvClient()
	if err != nil {
		return nil, err
	}

	// Get a list of Docker containers running in the pod.
	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.kubernetes.pod.name", name.Name)),
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.kubernetes.pod.namespace", name.Namespace)),
		),
	})
	if err != nil {
		return nil, err
	}

	// For each container, find the virtual interface connecting the container to the host.
	var ifaces []string
	for _, c := range containers {
		// Get the index of the container's virtual interface.
		cmd := "grep ^ /sys/class/net/vet*/ifindex | grep \":$(docker exec "+c.ID+" cat /sys/class/net/eth0/iflink)\" | cut -d \":\" -f 2"
		ifindex, err := execCommand("bash", "-c", cmd)
		if err != nil {
			return nil, err
		} else if ifindex == "" {
			continue
		}

		// List the host's interfaces and find the name of the virtual interface used by the container.
		cmd = "ip addr | grep \""+strings.TrimSuffix(ifindex, "\n")+":\" | cut -d \":\" -f 2 | cut -d \"@\" -f 1 | tr -d '[:space:]'"
		iface, err := execCommand("bash", "-c", cmd)
		if err != nil {
			return nil, err
		} else if iface == "" {
			continue
		}
		ifaces = append(ifaces, iface)
	}
	return ifaces, nil
}

// execCommand executes a shell command on the host and returns the output.
func execCommand(command string, args ...string) (string, error) {
	stdout := bytes.Buffer{}
	cmd := exec.Command(command, args...)
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return stdout.String(), nil
}
