package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Idempotently create the required ConfigMap
// Will return latest error if context has expired, else will keep trying
func (d *Driver) createConfigMap(ctx context.Context, sub progress.SubLogger) error {
	// Create the config map first
	err := fmt.Errorf("timeout before first attempt")
	var latestVerb string
	for err != nil {
		select {
		case <-ctx.Done():
			return err
		default:
		}

		_, err = d.configMapClient.Get(ctx, d.configMap.Name, metav1.GetOptions{})
		if err != nil && kubeerrors.IsNotFound(err) {
			// Doesn't exist, create it
			latestVerb = "create"
			_, err = d.configMapClient.Create(ctx, d.configMap, metav1.CreateOptions{})
		} else if err != nil {
			// Unexpected Get failure...
			logrus.Debugf("unexpected ConfigMap Get failure: %s", err)
			driver.RandSleep(1000)
			continue
		} else if d.userSpecifiedConfig {
			// err was nil, thus it already exists, and user passed a new config, so update it
			latestVerb = "update"
			_, err = d.configMapClient.Update(ctx, d.configMap, metav1.UpdateOptions{})
		}
		if err != nil {
			// Either the Create or the Update failed
			sub.Log(1, []byte(fmt.Sprintf("Warning \tfailed to %s configmap %s - retrying...\n", latestVerb, err)))
			driver.RandSleep(1000)
		}
	}
	return err
}

// Idempotently create the required ConfigMap
// Will return latest error if context has expired, else will keep trying
func (d *Driver) createBuilder(ctx context.Context, sub progress.SubLogger, userSpecifiedRuntime bool) error {
	var (
		err        error
		depl       *appsv1.Deployment
		replicaUID string
		refUID     *string
		refKind    *string
	)
	reportedEvents := map[string]interface{}{}

	logEvents := func(events []v1.Event, resource string, originUID types.UID, callback func(v1.Event) error) error {
		for _, event := range events {
			if event.InvolvedObject.UID != originUID {
				continue
			}
			// TODO - better formatting alignment...
			msg := fmt.Sprintf("%s \t%s \t%s \t%s\n",
				event.Type,
				resource,
				event.Reason,
				event.Message,
			)
			if _, alreadyProcessed := reportedEvents[msg]; alreadyProcessed {
				continue
			}
			reportedEvents[msg] = struct{}{}
			sub.Log(1, []byte(msg))
			// TODO handle additional known failure modes here...
			if event.Type == "Warning" && event.Reason == "Failed" && strings.Contains(event.Message, "Failed to pull image") {
				// While some image pull failures may be transient, it may take a very long time to converge, so fail fast
				return fmt.Errorf("%s", event.Message)
			}
			if event.Type == "Warning" && event.Reason == "Failed" && strings.Contains(event.Message, "Error: ErrImagePull") {
				// While some image pull failures may be transient, it may take a very long time to converge, so fail fast
				return fmt.Errorf("%s", event.Message)
			}

			if event.Type == "Normal" {
				continue
			}
			if err := callback(event); err != nil {
				return err
			}
		}
		return nil
	}
	var zero64 int64

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(err, "timed out waiting for builder to become ready")
		default:
		}

		depl, err = d.deploymentClient.Get(ctx, d.deployment.Name, metav1.GetOptions{})
		if err != nil && kubeerrors.IsNotFound(err) {
			depl, err = d.deploymentClient.Create(ctx, d.deployment, metav1.CreateOptions{})
			if err != nil {
				driver.RandSleep(1000)
				continue
			}
		} else if err != nil {
			// TODO - are there additional failure modes worth hardening for to fail fast?
			driver.RandSleep(1000)
			continue
		}

		// Exit once we're all healthy
		if depl.Status.ReadyReplicas >= int32(d.minReplicas) {
			sub.Log(1, []byte(fmt.Sprintf("All %d replicas for %s online\n", d.minReplicas, d.deployment.Name)))
			return nil
		}

		// Not all pods are ready, so inspect why, and take corrective action if necessary
		// Check to see if we have any replicaset errors so we can log them
		replicas, err2 := d.getReplicaSets(ctx, depl)
		if err2 != nil {
			err = errors.Wrapf(err2, "failed to retrieve replicaset for deployment %s", depl.Name)
			driver.RandSleep(1000)
			continue
		}
		for _, replica := range replicas {
			events, err2 := d.getReplicaEvents(ctx, depl, replica)
			if err2 != nil {
				err = errors.Wrapf(err2, "failed to retrieve replicaset for deployment %s", depl.Name)
				break
			}
			_ = logEvents(events, replica.Name, replica.ObjectMeta.UID, func(v1.Event) error { return nil })

			// TODO - is there a better/tighter listOpts to get just the pods for the specific replicaset?
			labelSelector := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": d.deployment.ObjectMeta.Name,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			if err != nil {
				// Shouldn't happen
				return err
			}
			listOpts := metav1.ListOptions{
				LabelSelector: selector.String(),
			}

			podList, err2 := d.podClient.List(ctx, listOpts)
			if err2 != nil {
				err = errors.Wrapf(err2, "failed to list pods for replicaset %s", replica.Name)
				driver.RandSleep(1000)
				continue
			}
			replicaUID = string(replica.ObjectMeta.GetUID())

			for i := range podList.Items {
				pod := &podList.Items[i]
				if !isChildOf(pod.ObjectMeta, replicaUID) {
					continue
				}

				stringRefUID := string(pod.GetUID())
				if len(stringRefUID) > 0 {
					refUID = &stringRefUID
				}
				stringRefKind := pod.Kind
				if len(stringRefKind) > 0 {
					refKind = &stringRefKind
				}
				selector := d.eventClient.GetFieldSelector(&pod.Name, &pod.Namespace, refKind, refUID)
				options := metav1.ListOptions{FieldSelector: selector.String()}
				events, err2 := d.eventClient.List(ctx, options)
				if err2 != nil {
					err = errors.Wrapf(err2, "failed to list pods events %s", pod.Name)
					driver.RandSleep(1000)
					break
				}

				err = logEvents(events.Items, pod.Name, pod.ObjectMeta.UID, func(event v1.Event) error {
					if event.Reason == "FailedMount" && strings.Contains(event.Message, "is not a socket file") {
						if d.userSpecifiedRuntime {
							return fmt.Errorf("pod failed to initialize - did you pick the correct runtime? - %s", event.Message)
						}
						// Flip the logic for what the default runtime is, and re-init, then re-bootstrap and try once more.
						attemptedRuntime := DefaultContainerRuntime
						runtime := ""
						switch attemptedRuntime {
						case "containerd":
							runtime = "docker"
						case "docker":
							runtime = "containerd"
						default:
							return fmt.Errorf("unexpected runtime: %s", attemptedRuntime) // not reached
						}

						sub.Log(1, []byte(fmt.Sprintf("Warning \tinitial attempt to deploy configured for the %s runtime failed, retrying with %s\n", attemptedRuntime, runtime)))
						d.InitConfig.DriverOpts["runtime"] = runtime
						d.InitConfig.DriverOpts["worker"] = "auto"
						err = d.initDriverFromConfig() // This will toggle userSpecifiedRuntime to true to prevent cycles
						if err != nil {
							return err
						}

						// Set the resource version for atomic update to detect possible races with other CLIs
						d.deployment.ObjectMeta.ResourceVersion = depl.ObjectMeta.ResourceVersion
						depl2, err := d.deploymentClient.Update(ctx, d.deployment, metav1.UpdateOptions{})
						if err != nil {
							// TODO - may need to explore the failure modes here further to see if additional hardening/retry logic is called for
							return errors.Wrapf(err, "error while calling deploymentClient.Update for %q - resourceVersion: %v", d.deployment.Name, &d.deployment.ObjectMeta.ResourceVersion)
						}
						depl = depl2

						// Delete the now stale replica set to accelerate convergence
						if err := d.replicaSetClient.Delete(ctx, replica.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero64}); err != nil && !kubeerrors.IsNotFound(err) {
							sub.Log(1, []byte(fmt.Sprintf("Warning \treplicaset deletion failed %s\n", err)))
						}

						// Note: on more recent versions of k8s/containerd the Deployment delete leaves the pod in a Terminating state for a
						// long time waiting for a mount timeout.  We work around this by forcing a quick delete of the pod(s) that hit the mount failure
						// Error: MountVolume.SetUp failed for volume "docker-sock" ... hostPath type check failed: /var/run/docker.sock is not a socket file
						// <60+ seconds>
						// Unable to attach or mount volumes for pod; skipping pod ... timed out waiting for the condition ...
						for j := range podList.Items {
							pod := &podList.Items[j]
							if !isChildOf(pod.ObjectMeta, replicaUID) {
								continue
							}
							if err := d.podClient.Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero64}); err != nil && !kubeerrors.IsNotFound(err) {
								// Just log, but proceed anyway and the Deployment Delete should eventually clean things up...
								sub.Log(1, []byte(fmt.Sprintf("Warning \tpod deletion failed %s\n", err)))
							}
						}
						driver.RandSleep(2000)
					}
					return nil
				})
				if err != nil {
					// If logEvents returns an error, fail fast
					return err
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (d *Driver) getReplicaSets(ctx context.Context, depl *appsv1.Deployment) ([]*appsv1.ReplicaSet, error) {
	resp := []*appsv1.ReplicaSet{}

	deploymentUID := string(depl.ObjectMeta.GetUID())
	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": d.deployment.ObjectMeta.Name,
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		// Shouldn't happen
		return nil, err
	}
	listOpts := metav1.ListOptions{
		LabelSelector: selector.String(),
	}

	// TODO - consider a get?
	replicas, err := d.replicaSetClient.List(ctx, listOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "list on replicaset %v failed", listOpts)
	}

	for i := range replicas.Items {
		replica := &replicas.Items[i]
		if !isChildOf(replica.ObjectMeta, deploymentUID) {
			continue
		}
		resp = append(resp, replica)
	}
	return resp, nil
}

func (d *Driver) getReplicaEvents(ctx context.Context, depl *appsv1.Deployment, replica *appsv1.ReplicaSet) ([]v1.Event, error) {
	var (
		refUID  *string
		refKind *string
	)

	stringRefUID := string(replica.GetUID())
	if len(stringRefUID) > 0 {
		refUID = &stringRefUID
	}
	stringRefKind := replica.Kind
	if len(stringRefKind) > 0 {
		refKind = &stringRefKind
	}

	selector := d.eventClient.GetFieldSelector(&replica.Name, &replica.Namespace, refKind, refUID)
	options := metav1.ListOptions{FieldSelector: selector.String()}
	events, err := d.eventClient.List(ctx, options)
	if err != nil {
		return nil, err
	}
	return events.Items, err

}
