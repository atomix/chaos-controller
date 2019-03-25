# Chaos Controller

The Chaos Controller provides a controller for chaos testing in [Kubernetes][Kubernetes]
and supports a rich set of supported failure scenarios. It relies in Linux and Docker
functionality to inject network partitions and stress nodes.

* [Setup](#setup)
  * [Helm](#helm)
  * [Manual setup](#manual-setup)
* [Usage](#usage)
  * [Scheduling](#scheduling)
  * [Selectors](#selectors)
  * [Crash monkey](#crash-monkey)
  * [Partition monkey](#partition-monkey)
  * [Stress monkey](#stress-monkey)
* [Architecture](#architecture)
  * [Controller](#controller)
    * [Crash controller](#crash-controller)
    * [Partition controller](#partition-controller)
    * [Stress controller](#stress-controller)
  * [Workers](#workers)
    * [Crash workers](#crash-workers)
    * [Partition workers](#partition-workers)
    * [Stress workers](#stress-workers)

## Setup

### Helm

A [Helm][Helm] chart is provided for setting up the controller. To deploy the
controller use `helm install helm` from the project root:

```
helm install helm
```

When the chart is installed, following custom resources will be added to
the cluster:
* `ChaosMonkey`
* `Crash`
* `NetworkPartition`
* `Stress`

The `ChaosMonkey` resource is the primary resource provided by the controller.
The remaining custom resources are used by the controller to inject specific
failures into pods.

The chart supports overrides for both the [controller](#controller) and
[workers](#workers). The controller is deployed as a `Deployment`, and
the workers as a `DaemonSet`.

### Manual Setup

Before running the controller, register the custom resources:

```
$ kubectl create -f deploy/chaosmonkey.yaml
$ kubectl create -f deploy/crash.yaml
$ kubectl create -f deploy/networkpartition.yaml
$ kubectl create -f deploy/stress.yaml
```

Setup RBAC and deploy the controller:

```
$ kubectl create -f deploy/service_account.yaml
$ kubectl create -f deploy/role.yaml
$ kubectl create -f deploy/role_binding.yaml
$ kubectl create -f deploy/controller.yaml
```

Deploy the workers:

```
$ kubectl create -f deploy/workers.yaml
```

Example `ChaosMonkey` resources can be found in the `example` directory:
```
$ kubectl create -f example/crash_monkey.yaml
$ kubectl create -f example/partition_monkey.yaml
$ kubectl create -f example/stress_monkey.yaml
```

## Usage

The chaos controller provides a full suite of tools for chaos testing, injecting
a variety of failures into the nodes and in the k8s pods and networks. Each monkey
plays a specific role in injecting failures into the cluster:

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: crash-monkey
spec:
  rateSeconds: 60
  jitter: .5
  crash:
    crashStrategy:
      type: Container
```

### Scheduling

The scheduling of periodic `ChaosMonkey` executions can be managed by providing a
_rate_ and _period_ for which the fault occurs:
* `rateSeconds` - the number of seconds to wait between monkey runs
* `periodSeconds` - the number of seconds for which to run a monkey, e.g. the amount of
time for which to partition the network or stress a node
* `jitter` - the amount of jitter to apply to the rate

### Selectors

Specific sets of pods can be selected using pod names, labels, or match expressions
specified in the configured `selector`:
* `matchPods` - a list of pod names on which to match
* `matchLabels` - a map of label names and values on which to match pods
* `matchExpressions` - label match expressions on which to match pods

Selector options can be added on a per-monkey basis:

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: crash-monkey
spec:
  crash:
    crashStrategy:
      type: Pod
  selector:
    matchPods:
    - pod-1
    - pod-2
    - pod-3
    matchLabels:
      group: raft
    matchExpressions:
    - key: group
      operator: In
      values:
      - raft
      - data
```

Each monkey type has a custom configuration provided by a named field for the
monkey type:
* [`crash`](#crash-monkey)
* [`partition`](#partition-monkey)
* [`stress`](#stress-monkey)

### Crash monkey

The crash monkey can be used to inject node crashes into the cluster. To configure a
crash monkey, use the `crash` configuration:

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: crash-monkey
spec:
  rateSeconds: 60
  jitter: .5
  crash:
    crashStrategy:
      type: Container
```

The `crash` configuration supports a `crashStrategy` with the following options:
* `Container` - kills the process running inside the container
* `Pod` - deletes the `Pod` using the Kubernetes API

### Partition monkey

The partition monkey can be used to cut off network communication between a set of
pods. To use the partition monkey, selected pods must have `iptables` installed.
To configure a partition monkey, use the `partition` configuration:

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: partition-isolate-monkey
spec:
  rateSeconds: 600
  periodSeconds: 120
  partition:
    partitionStrategy:
      type: Isolate
```

The `partition` configuration supports a `partitionStrategy` with the following options:
* `Isolate` - isolates a single random node in the cluster from all other nodes
* `Halves` - splits the cluster into two halves
* `Bridge` - splits the cluster into two halves with a single bridge node able to
communicate with each half (for testing consensus)

### Stress monkey

The stress monkey uses a variety of tools to simulate stress on nodes and on the 
network. To configure a stress monkey, use the `stress` configuration:

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: stress-cpu-monkey
spec:
  rateSeconds: 300
  periodSeconds: 300
  stress:
    stressStrategy:
      type: All
    cpu:
      workers: 2
```

The `stress` configuration supports a `stressStrategy` with the following options:
* `Random` - applies stress options to a random pod
* `All` - applies stress options to all pods in the cluster

The stress monkey supports a variety of types of stress using the
[stress](https://linux.die.net/man/1/stress) tool:
* `cpu` - spawns `cpu.workers` workers spinning on `sqrt()`
* `io` - spawns `io.workers` workers spinning on `sync()`
* `memory` - spawns `memory.workers` workers spinning on `malloc()`/`free()`
* `hdd` - spawns `hdd.workers` workers spinning on `write()`/`unlink()`

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: stress-all-monkey
spec:
  rateSeconds: 300
  periodSeconds: 300
  stress:
    stressStrategy:
      type: Random
    cpu:
      workers: 2
    io:
      workers: 2
    memory:
      workers: 4
    hdd:
      workers: 1
```

Additionally, network latency can be injected using the stress monkey via
[traffic control](http://man7.org/linux/man-pages/man8/tc-netem.8.html) by providing
a `network` stress configuration:
* `latencyMilliseconds` - the amount of latency to inject in milliseconds
* `jitter` - the jitter to apply to the latency
* `correlation` - the correlation to apply to the latency
* `distribution` - the delay distribution, either `normal`, `pareto`, or `paretonormal`

```yaml
apiVersion: chaos.atomix.io/v1alpha1
kind: ChaosMonkey
metadata:
  name: stress-network-monkey
spec:
  rateSeconds: 300
  periodSeconds: 60
  stress:
    stressStrategy:
      type: All
    network:
      latencyMilliseconds: 500
      jitter: .5
      correlation: .25
```

## Architecture

The controller consists of two independent components which run as containers in
a k8s cluster.

### Controller

The controller is the component responsible for monitoring the creation/deletion of
`ChaosMonkey` resources, scheduling executions, and distributing tasks to [workers](#workers).
The controller typically runs as a `Deployment`. When multiple replicas are run, only a
single replica will control the cluster at any given time.

When `ChaosMonkey` resources are created in the k8s cluster, the controller receives a
notification and, in response, schedules a periodic background task to execute the monkey
handler. The periodic task is configured based on the monkey configuration. When a monkey
handler is executed, the controller filters pods using the monkey's configured selectors
and passes the pods to the handler for execution. Monkey handlers then assign tasks to
specific [workers](#workers) to carry out the specified chaos function.

![Controller](https://i.ibb.co/d76BDZW/Controller-Worker.png)

#### Crash controller

The crash controller assigns tasks to workers via `Crash` resources. The `Crash` resource
will indicate the `podName` of the pod to crash and the `crashStrategy` with which to crash
the pod.

#### Partition controller

The partition controller assigns tasks to workers according to the configured
`partitionStrategy` and uses the `NetworkPartition` resource to communicate details of the
network partition to the workers. After determining the set of routes to cut off between pods,
the controller creates a `NetworkPartition` for each source/destination pair.

![Network Partition](https://i.ibb.co/J7DX1v5/Network-Partition.png)

#### Stress controller

The stress controller assigns tasks to workers via `Stress` resources. The `Stress` resource
will indicate the `podName` of the pod to stress and the mechanisms with which to stress the
pod.

### Workers

Workers are the components responsible for injecting failures on specific k8s nodes.
Like the [controller](#controller), workers are a type of resource controller, but rather
than managing the high level `ChaosMonkey` resources used to randomly inject failures,
workers provide for the injection of pod-level failures in response to the creation of
resources like `Crash`, `NetworkPartition`, or `Stress`.

In order to ensure a worker is assigned to each node and can inject failures into the OS,
workers must be run in a `DaemonSet` and granted `privileged` access to k8s nodes.

#### Crash workers

Crash workers monitor k8s for the creation of `Crash` resources. When a `Crash` resource is
detected, if the `podName` contained in the `Crash` belongs to the node on which the worker
is running, the worker executes the crash. This ensures only one node attempts to execute
a crash regardless of the method by which the crash is performed.

The method of execution of the crash depends on the configured `crashStrategy`. If the `Pod`
strategy is used, the worker simply deletes the pod via the Kubernetes API. If the `Container`
strategy is indicates, the worker locates the pod's container(s) via the Docker API and
kills the containers directly.

#### Partition workers

Partition workers monitor k8s for the creation of `NetworkPartition` resources. Each
`NetworkPartition` represents a link between two pods to be cut off while the resource is
running. The worker configures the pod indicated by `podName` to drop packets from the
configured `sourceName`. `NetworkPartition` resources may be in one of four _phases_:
* `started` indicates the resource has been created but the pods are not yet partitioned
* `running` indicates the pod has been partitioned
* `stopped` indicates the partition has been stopped but the physical communication has not
yet been restored
* `complete` indicates communication between the pod and the source has been restored

When a worker receives notification of a `NetworkPartition` in the `started` phase, if the
pod is running on the worker's node, the worker cuts off communication between the pod and
the source by locating the virtual network interface for the pod and adding firewall rules
to the host to drop packets received on the pod's virtual interface from the source IP.
To restore communication with the source, the worker simply deletes the added firewall rules.

#### Stress workers

Stress workers monitor k8s for the creation of `Stress` resources. When a `Stress` resource
is detected, the worker on the node to which the stressed pod is assigned may perform several
tasks to stress the desired pod.

For I/O, CPU, memory, and HDD stress, the worker will create a container in the pod's
namespace to execute the [stress tool](https://linux.die.net/man/1/stress). For each
configured stress option, a separate container will be created to stress the pod.
When the `Stress` resource is `stopped`, all stress containers will be stopped.

For network stress, the host is configured using the [traffic control](https://linux.die.net/man/8/tc)
utility to control the pod's virtual interfaces. When the `Stress` resource is `stopped`,
the traffic control rule is deleted.

[Kubernetes]: https://kubernetes.io/
[Helm]: https://helm.sh
