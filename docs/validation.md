# Validating BuildKit CLI for kubectl

While this CLI can be used for CI usecases where the images are always pushed to a registry, the primary
usecase is for developer scenarios where the image is immediately available within the Kubernetes cluster.
The example listed here can be used to validate everything is set up properly and you're able to build images
and run pods with those images.

## Remove any prior default builder
```
kubectl buildkit rm buildkit
```

## Build a simple image
We'll first build a simple image which prints "Success!" after it runs. We won't tag this with latest since that would cause Kubernetes to try to pull it from a registry.
```
cat << EOF | kubectl build -t acme.com/some/image:1.0 -f - .
FROM busybox
ENTRYPOINT ["echo", "Success!"]
EOF
``` 

## Run a Job with the image we just built
```
cat << EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: testjob1
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: jobc
        image: acme.com/some/image:1.0
        imagePullPolicy: Never
EOF
```

## Confirm Success
```
kubectl get job testjob1
```
You want to see `COMPLETIONS` showing `1/1`

If not, then troubleshoot below...

## Troubleshooting

You can try to look for the expected "Success!" message with the following:
```
kubectl logs -l job-name=testjob1
```

If no logs are available, you can inspect the status and events related to the pod with the following:
```
kubectl describe pod -l job-name=testjob1
```

If the images are not getting loaded properly into the container runtime, you'll see an Event that looks like this:

```
  Warning  ErrImageNeverPull  4s (x11 over 99s)  kubelet            Container image "acme.com/some/image:1.0" is not present with pull policy of Never
```

In that case, take a closer look at the output from the build command and/or get logs for the builder pod(s)
to see if there are any error messages.  You may be able to use the `kubectl buildkit create` options to
solve or workaround the problem.  If you aren't able to find a working configuration, please file a bug
and include the details of your k8s environment and relevant logs so we can assist.

## Cleanup
```
kubectl delete job testjob1
```