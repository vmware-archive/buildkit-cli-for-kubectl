# Architecture

The following diagram shows the major components for the system.

![Architecture](./architecture.svg)

When you run `kubectl build` or `kubectl buildkit` the kubectl CLI detects this is a CLI plugin and fork/exec's the `kubectl-buildkit` or `kubectl-build` CLI binary.

The CLI plugin then operates just like kubectl does.  It loads up your kubeconfig, uses the currently active context, and interacts with the kubernetes API on your cluster.

In order to build a container image, `kubectl build` needs one or more BuildKit builders. These builders can be created explicitly with the `kubectl buildkit create` command, or one will be created automatically the first time you run `kubectl build`. The builders are modeled as a kubernetes Deployment, but we'll probably add DaemonSet support too.  The Deployment uses anti-affinity to distribute pods across the nodes in the cluster.

The pod spec by default tries to mount the socket for the container runtime on the host so it can communicate directly to the runtime. This works for both `containerd` and `dockerd` runtimes. This allows the builder to build and load the images you build directly into the container runtime so kubernetes can run other pods with those images. You can opt-out of this model if you prefer a de-privileged approach, but then it can't load the images directly. For those cases you will have to push any built images to a registry and then rely on the kubelet to pull them.

When you run a command like `build` the CLI finds one running/healthy pod, and does the equivalent of a `kubectl exec` to connect to the pod.  Within the pod we use 2 containers.  One container has the BuildKit daemon, and the other contains a proxy component used by this CLI to communicate with `buildkitd`. The CLI uses this proxy to be able to route gRPC API calls over the stdin/stdout pipe from the exec to the `buildkitd`.  This proxy helps offload the work of loading the final image from `buildkitd` into the local runtime, unless you've opted out or have requested to push directly to a registry. 

To make your freshly built image available in a multi-node cluster, you must scale the Deployment definition for the builder up, and the CLI detects multiple builder pods are running.  When detected, the proxy container inside the pods facilitate transferring the image between the nodes across the internal kubernetes cluster network.  This avoids transferring the image back-and-forth between the builder and CLI, which may be running remotely.  If you specify `--push` during the build, it skips this transfer step during the build.


## Code/Repo Layout

The following lists the major components that make up the repo

* [cmd/](../cmd/) Main routines for the CLI plugin
* [pkg/cmd/](../pkg/cmd/) Implementations of the CLI verbs dealing with argument parsing (e.g. `build`, `create`, etc.)
* [pkg/build/](../pkg/build/) The "heavy lifting" of the build logic
* [pkg/driver/kubernetes/](../pkg/driver/kubernetes/) The logic to manage and interact with the kubernetes builders
* [pkg/driver/kubernetes/execconn/](../pkg/driver/kubernetes/execconn/) The exec tunnel to send gRPC commands to the builder pod
* [pkg/driver/kubernetes/manifest/](../pkg/driver/kubernetes/manifest/) Handles creation of the kubernetes resource definitions for the builder
* [pkg/driver/kubernetes/podchooser/](../pkg/driver/kubernetes/podchooser/) Selects which pod to send build requests to
* [pkg/proxy/](../pkg/proxy/) Runs within the builder pod to offload CLI operations related to image loading and transfer between nodes
* [integration/](../integration/) Integration tests which exercise the CLI via go test running on a live kubernetes cluster
