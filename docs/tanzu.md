# Using BuildKit CLI for kubectl on VMware vSphere Tanzu

If your company runs VMware vSphere 7, you have the ability to run Tanzu *guest clusters*, which are kubernetes clusters running directly on your virtual infrastructure.  These can be used as "personal dev/test clusters" or "build farms" to take advantage of the compute power of the underlying servers.

In vSphere Tanzu, there are two layers of kubernetes clusters:

* The *supervisor cluster*
* Zero or more *guest clusters*

A *supervisor cluster* uses Common Resource Definitions (CRDs) and Controllers to model Virtual Machines, Virtual Networks, VMFS partitions, and other vSphere resources, including *guest clusters*. This *supervisor cluster* is typically reserved for vSphere admins.

*Guest clusters* operate inside Virtual Machines running on vSphere. Tanzu abstracts away the details of having to provision these Virtual Machines by allowing you to describe how you want the cluster to be configured. These declarative definitions are used by Controllers in the *supervisor cluster* to automatically create each of the Virtual Machines and resources for your *guest clusters*.

Depending on how your vSphere cluster is configured, as a developer you may be able to self-provision these *guest clusters*, or you may need to ask your vSphere admins (file help-desk tickets, etc.) to get a cluster set up.


## Download the vSphere kubectl CLI plugin

To interact with Tanzu clusters, you'll need the vSphere CLI plugin for kubectl.

At present, VMware does not host a public/internet download page for the latest kubectl CLI plugin for vSphere.  As long as you know the IP address or hostname of your vCenter server, you can download the plugin from that server.  The following example assumes you're running a MacOS laptop:

```sh
curl -L https://${VSPHERE_IP}/wcp/plugin/darwin-amd64/vsphere-plugin.zip -o /tmp/vsphere-plugin.zip
unzip /tmp/vsphere-plugin.zip bin/kubectl-vsphere -d /usr/local
rm /tmp/vsphere-plugin.zip
```

You can confirm it's working with a quick `kubectl vsphere --help`

## Get access to Supervisor Namespace

If your vSphere admins allow self-provisioning of *guest clusters*, you'll typically need to request a Namespace in the *supervisor cluster*.  This Namespace will most likely be locked down, but will allow you to provision *guest clusters*.

```sh
kubectl vsphere login \
    --server ${VSPHERE_IP} \
    --vsphere-username <YOUR_USERNAME> \
    --tanzu-kubernetes-cluster-namespace <YOUR_SUPERVISOR_NAMESPACE>
```

Once you've logged into the *supervisor cluster*, you'll be able to create *guest clusters*

## Creating a Guest Cluster

This step may need to be performed by a vSphere admin if "self provisioning" of *guest clusters* isn't allowed in your environment.

Before you can write up your yaml to create a cluster, you'll need to determine what the `storageClass` is called in your environment.  To determine this, run `kubectl describe namespace <YOUR_SUPERVISOR_NAMESPACE>`  Under the "Resource" list, you should see XXX.storageclass... - the XXX string is what you want.

Create a cluster definition that looks something like this, then `kubectl apply -f mycluster.yaml`

```yaml
apiVersion: run.tanzu.vmware.com/v1alpha1
kind: TanzuKubernetesCluster
metadata:
  name: dev-cluster-1
spec:
  distribution:
    version: v1.18
  topology:
    controlPlane:
      count: 1
      class: best-effort-small
      storageClass: XXX # <-- CHANGE THIS
    workers:
      count: 1
      class: best-effort-medium
      storageClass: XXX # <-- CHANGE THIS
```

Provisioning the cluster can take a little while (depending on how loaded your vSphere cluster is.)  You can watch the progress with something like `watch -n 2 kubectl get tanzukubernetescluster dev-cluster-1`

## Login to the Guest Cluster

Once your Guest Cluster has finished provisioning, you can log in

```sh
kubectl vsphere login \
    --server ${VSPHERE_IP} \
    --vsphere-username <YOUR_USERNAME> \
    --tanzu-kubernetes-cluster-name dev-cluster-1 \
    --tanzu-kubernetes-cluster-namespace <YOUR_SUPERVISOR_NAMESPACE>
```

After you have logged in to vSphere, your active kubectl config will be updated (typically `~/.kube/config` but can be overridden with the env `KUBECONFIG`), and new contexts will be added if this is your first login. You can view all of your contexts using the command:

```sh
kubectl config get-contexts
```

and you can use the newly created context with the command:


```sh
kubectl config use-context dev-cluster-1
```

## Adjusting Default Security Policies

By default, Tanzu *guest clusters* are set up with very restrictive permissions.  To be able to build images and run test workloads, you'll need to adjust those permissions.  The following example opens things up.  **Don't use this on production clusters.**

```sh
kubectl create rolebinding rolebinding-default-privileged-sa-ns_default \
    --namespace=default \
    --clusterrole=psp:vmware-system-privileged \
    --group=system:serviceaccounts
```

At this point you should be able to run `kubectl build ...` on your cluster and run workloads with the images you've built.


# Multinode Hints

If you provisioned a multi-node cluster, remember to scale up your builders so your images will be loaded onto all the nodes.  For example, if you have a 3 node cluster

```
kubectl buildkit create --replicas 3
```