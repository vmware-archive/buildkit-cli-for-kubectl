// Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetKubeClientset retrieves the clientset and namespace
func GetKubeClientset() (*kubernetes.Clientset, string, error) {
	configFlags := genericclioptions.NewConfigFlags(true)
	clientConfig := configFlags.ToRawKubeConfigLoader()
	ns, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, "", err
	}
	restClientConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, "", err
	}
	clientset, err := kubernetes.NewForConfig(restClientConfig)
	if err != nil {
		return nil, "", err
	}

	// Verify the cluster is accessible so we can fail fast
	content, err := clientset.Discovery().RESTClient().Get().AbsPath("/livez").DoRaw(context.Background())
	if err != nil {
		return nil, "", errors.Wrap(err, "kubernetes cluster is unhealthy or inaccessible")
	}
	if !strings.Contains(string(content), "ok") {
		return nil, "", fmt.Errorf("kubernetes cluster is unhealthy or inaccessible: %s", string(content))
	}
	return clientset, ns, err
}

func RunSimpleBuildImageAsPod(ctx context.Context, name, imageName, namespace, nodeName string, clientset *kubernetes.Clientset) error {
	podClient := clientset.CoreV1().Pods(namespace)
	eventClient := clientset.CoreV1().Events(namespace)
	logrus.Infof("starting pod %s for image %s on node '%s'", name, imageName, nodeName)
	// Start the pod
	var zero64 int64
	pod, err := podClient.Create(ctx,
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},

			Spec: corev1.PodSpec{
				NodeName: nodeName,
				Containers: []corev1.Container{
					{
						Name:            name,
						Image:           imageName,
						Command:         []string{"sleep", "60"},
						ImagePullPolicy: v1.PullNever,
					},
				},
				TerminationGracePeriodSeconds: &zero64,
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}

	defer func() {
		err := podClient.Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil {
			logrus.Warnf("failed to clean up pod %s: %s", pod.Name, err)
		}
	}()

	logrus.Infof("waiting for pod to start...")
	// Wait for it to get started, and make sure it isn't complaining about image not being found
	// TODO - multi-node test clusters will need some refinement here if we wind up not scaling the builder up in some scenarios
	var refUID *string
	var refKind *string
	reportedEvents := map[string]interface{}{}

	// TODO - DRY this out with pkg/driver/kubernetes/driver.go:wait(...)
	for try := 0; try < 100; try++ {

		stringRefUID := string(pod.GetUID())
		if len(stringRefUID) > 0 {
			refUID = &stringRefUID
		}
		stringRefKind := pod.Kind
		if len(stringRefKind) > 0 {
			refKind = &stringRefKind
		}
		selector := eventClient.GetFieldSelector(&pod.Name, &pod.Namespace, refKind, refUID)
		options := metav1.ListOptions{FieldSelector: selector.String()}
		events, err2 := eventClient.List(ctx, options)
		if err2 != nil {
			return err2
		}

		for _, event := range events.Items {
			if event.InvolvedObject.UID != pod.ObjectMeta.UID {
				continue
			}
			msg := fmt.Sprintf("%s:%s:%s:%s\n",
				event.Type,
				pod.Name,
				event.Reason,
				event.Message,
			)
			if _, alreadyProcessed := reportedEvents[msg]; alreadyProcessed {
				continue
			}
			reportedEvents[msg] = struct{}{}
			logrus.Info(msg)

			if event.Reason == "ErrImageNeverPull" {
				// Fail fast, it will never converge
				return fmt.Errorf(msg)
			}
		}

		<-time.After(time.Duration(100+try*20) * time.Millisecond)
		pod, err = podClient.Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		logrus.Infof("Pod Phase: %s", pod.Status.Phase)
		if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
			return nil
		}
	}
	return fmt.Errorf("pod never started")
}

// GetRuntime will return the runtime detected in the cluster
// Assumes a common runtime (first node found is returned)
func GetRuntime(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	nodes, err := GetNodes(ctx, clientset)
	if err != nil {
		return "", err
	}
	if len(nodes) > 0 {
		return nodes[0].Status.NodeInfo.ContainerRuntimeVersion, nil
	}
	return "", fmt.Errorf("unable to retrieve node runtimes")
}

func GetNodes(ctx context.Context, clientset *kubernetes.Clientset) ([]v1.Node, error) {
	nodeClient := clientset.CoreV1().Nodes()
	nodes, err := nodeClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Remove any nodes that aren't ready
	res := []v1.Node{}
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
				res = append(res, node)
				logrus.Debugf("Node %s is ready", node.Name)
				continue
			}
		}
	}
	return res, nil
}

func GetBuilderNodes(ctx context.Context, name, namespace string, clientset *kubernetes.Clientset) ([]string, error) {
	nodeNames := []string{}
	pods, err := getBuilderPods(ctx, name, namespace, clientset)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		nodeNames = append(nodeNames, pod.Spec.NodeName)
	}
	return nodeNames, nil
}

func getBuilderPods(ctx context.Context, name, namespace string, clientset *kubernetes.Clientset) ([]v1.Pod, error) {
	podClient := clientset.CoreV1().Pods(namespace)
	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": name,
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		logrus.Errorf("should not happen: %s", err)
		return nil, err
	}
	listOpts := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	podList, err := podClient.List(ctx, listOpts)
	if err != nil {
		logrus.Warnf("failed to get builder pods: %s", err)
		return nil, err
	}
	return podList.Items, nil
}

// LogBuilderLogs attempts to replay the log messages from the builder(s)
func LogBuilderLogs(ctx context.Context, name, namespace string, clientset *kubernetes.Clientset) {
	pods, err := getBuilderPods(ctx, name, namespace, clientset)
	if err != nil {
		return
	}
	logrus.Infof("Detected %d pods for builder %s - gathering logs", len(pods), name)
	logrus.Infof("--- BEGIN BUILDER LOGS ---")
	LogPodLogs(ctx, pods, namespace, clientset)
	logrus.Infof("--- END BUILDER LOGS ---")
}

func LogPodLogs(ctx context.Context, pods []v1.Pod, namespace string, clientset *kubernetes.Clientset) {
	podClient := clientset.CoreV1().Pods(namespace)
	for _, pod := range pods {
		for _, ctr := range pod.Spec.Containers {
			logrus.Infof("%s labels %#v", pod.Name, pod.Labels)
			req := podClient.GetLogs(pod.Name, &v1.PodLogOptions{Container: ctr.Name})
			buf, err := req.DoRaw(ctx)
			if err != nil {
				logrus.Errorf("failed to get logs for %s: %s", pod.Name, err)
			}
			for _, line := range strings.Split(string(buf), "\n") {
				// Don't use logrus since that results in double levels and conflicting timestamps
				fmt.Printf("pod=\"%s\" container=\"%s\" %s\n", pod.Name, ctr.Name, line)
			}
		}
	}

}
