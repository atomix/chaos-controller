# Chaos Controller

The Chaos Controller provides a controller for chaos testing in [Kubernetes][Kubernetes].

## Setup

Before running the controller, register the ChaosMonkey CRD:

```
$ kubectl create -f deploy/chaosmonkey.yaml
```

Setup RBAC and deploy the controller:

```
$ kubectl create -f deploy/service_account.yaml
$ kubectl create -f deploy/role.yaml
$ kubectl create -f deploy/role_binding.yaml
$ kubectl create -f deploy/controller.yaml
```

Example resources can be found in the `example` directory:
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

Each monkey configuration supports both a _rate_ and _period_ for which the crash occurs:
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

[Kubernetes]: https://kubernetes.io/
