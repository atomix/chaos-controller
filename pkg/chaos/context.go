/*
 * Copyright 2019 Open Networking Foundation
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

package chaos

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	client  client.Client
	scheme  *runtime.Scheme
	kubecli kubernetes.Interface
	config  *rest.Config
	log     logr.Logger
}

// new returns a new Context with the given logger
func (c Context) new(log logr.Logger) Context {
	return Context{c.client, c.scheme, c.kubecli, c.config, log}
}

// exec executes a command in the given pod/container
func (c *Context) exec(pod v1.Pod, container *v1.Container, command ...string) (string, error) {
	log := c.log.WithValues("pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)

	req := c.kubecli.CoreV1().RESTClient().Post()
	req = req.Resource("pods").Name(pod.Name).Namespace(pod.Namespace).SubResource("exec")

	req.VersionedParams(&v1.PodExecOptions{
		Container: container.Name,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
		Stdin:     false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())

	if err != nil {
		log.Error(err, "Creating remote command executor failed")
		return "", err
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	log.Info("Executing command", "command", command)
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: bufio.NewWriter(&stdout),
		Stderr: bufio.NewWriter(&stderr),
		Tty:    false,
	})

	log.Info(stderr.String())
	log.Info(stdout.String())

	if err != nil {
		log.Error(err, "Executing command failed")
		return "", err
	}

	log.Info("Command succeeded")
	if stderr.Len() > 0 {
		return "", fmt.Errorf("stderr: %v", stderr.String())
	}

	return stdout.String(), nil
}
