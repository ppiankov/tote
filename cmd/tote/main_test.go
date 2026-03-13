package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestStripPodFields(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc123",
			Namespace: "default",
			Annotations: map[string]string{
				"tote.dev/auto-salvage": "true",
			},
			OwnerReferences: []metav1.OwnerReference{
				{Name: "web", Kind: "ReplicaSet"},
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl"},
			},
		},
		Spec: corev1.PodSpec{
			NodeName:                      "node-1",
			ServiceAccountName:            "default",
			SchedulerName:                 "default-scheduler",
			DNSPolicy:                     corev1.DNSClusterFirst,
			RestartPolicy:                 corev1.RestartPolicyAlways,
			SecurityContext:               &corev1.PodSecurityContext{},
			TerminationGracePeriodSeconds: ptr.To[int64](30),
			Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			InitContainers: []corev1.Container{
				{Name: "init", Image: "busybox"},
			},
			Containers: []corev1.Container{
				{
					Name:    "app",
					Image:   "nginx:1.25",
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", "sleep"},
					Env: []corev1.EnvVar{
						{Name: "FOO", Value: "bar"},
					},
					EnvFrom: []corev1.EnvFromSource{
						{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cm"}}},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "data", MountPath: "/data"},
					},
					Ports: []corev1.ContainerPort{
						{ContainerPort: 80},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
						Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
					},
					LivenessProbe:  &corev1.Probe{InitialDelaySeconds: 10},
					ReadinessProbe: &corev1.Probe{InitialDelaySeconds: 5},
					StartupProbe:   &corev1.Probe{InitialDelaySeconds: 1},
					SecurityContext: &corev1.SecurityContext{
						RunAsUser: ptr.To[int64](1000),
					},
					ImagePullPolicy:          corev1.PullAlways,
					TerminationMessagePath:   "/dev/termination-log",
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
			},
		},
		Status: corev1.PodStatus{
			PodIP:     "10.0.0.1",
			HostIP:    "192.168.1.1",
			PodIPs:    []corev1.PodIP{{IP: "10.0.0.1"}},
			StartTime: &metav1.Time{},
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					Ready:        true,
					RestartCount: 3,
					ContainerID:  "containerd://abc123",
					Started:      ptr.To(true),
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ImagePullBackOff",
							Message: "Back-off pulling image",
						},
						Running:    &corev1.ContainerStateRunning{},
						Terminated: &corev1.ContainerStateTerminated{ExitCode: 1},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{ExitCode: 137},
					},
				},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "init",
					Ready:        true,
					RestartCount: 1,
					ContainerID:  "containerd://def456",
					Started:      ptr.To(true),
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "Init:Error"},
					},
				},
			},
		},
	}

	result, err := stripPodFields(pod)
	if err != nil {
		t.Fatalf("stripPodFields returned error: %v", err)
	}

	got := result.(*corev1.Pod)

	// Retained: metadata.
	if got.Name != "web-abc123" {
		t.Errorf("name stripped: got %q", got.Name)
	}
	if got.Namespace != "default" {
		t.Errorf("namespace stripped: got %q", got.Namespace)
	}
	if got.Annotations["tote.dev/auto-salvage"] != "true" {
		t.Error("annotations stripped")
	}
	if len(got.OwnerReferences) != 1 {
		t.Error("ownerReferences stripped")
	}

	// Retained: spec.nodeName, containers[].name, containers[].image.
	if got.Spec.NodeName != "node-1" {
		t.Errorf("nodeName stripped: got %q", got.Spec.NodeName)
	}
	if len(got.Spec.Containers) != 1 {
		t.Fatal("containers stripped")
	}
	if got.Spec.Containers[0].Name != "app" {
		t.Error("container name stripped")
	}
	if got.Spec.Containers[0].Image != "nginx:1.25" {
		t.Error("container image stripped")
	}

	// Retained: status waiting state.
	if len(got.Status.ContainerStatuses) != 1 {
		t.Fatal("containerStatuses stripped")
	}
	if got.Status.ContainerStatuses[0].State.Waiting == nil {
		t.Error("containerStatuses[0].state.waiting stripped")
	}
	if got.Status.ContainerStatuses[0].State.Waiting.Reason != "ImagePullBackOff" {
		t.Error("waiting reason stripped")
	}
	if len(got.Status.InitContainerStatuses) != 1 {
		t.Fatal("initContainerStatuses stripped")
	}
	if got.Status.InitContainerStatuses[0].State.Waiting == nil {
		t.Error("initContainerStatuses[0].state.waiting stripped")
	}

	// Stripped: managed fields.
	if got.ManagedFields != nil {
		t.Error("managedFields not stripped")
	}

	// Stripped: spec fields.
	if got.Spec.Volumes != nil {
		t.Error("volumes not stripped")
	}
	if got.Spec.InitContainers != nil {
		t.Error("initContainers not stripped")
	}
	if got.Spec.SecurityContext != nil {
		t.Error("securityContext not stripped")
	}
	if got.Spec.ServiceAccountName != "" {
		t.Error("serviceAccountName not stripped")
	}
	if got.Spec.TerminationGracePeriodSeconds != nil {
		t.Error("terminationGracePeriodSeconds not stripped")
	}

	// Stripped: container fields.
	c := got.Spec.Containers[0]
	if c.Command != nil {
		t.Error("command not stripped")
	}
	if c.Args != nil {
		t.Error("args not stripped")
	}
	if c.Env != nil {
		t.Error("env not stripped")
	}
	if c.EnvFrom != nil {
		t.Error("envFrom not stripped")
	}
	if c.VolumeMounts != nil {
		t.Error("volumeMounts not stripped")
	}
	if c.Ports != nil {
		t.Error("ports not stripped")
	}
	if c.LivenessProbe != nil {
		t.Error("livenessProbe not stripped")
	}
	if c.ReadinessProbe != nil {
		t.Error("readinessProbe not stripped")
	}
	if c.StartupProbe != nil {
		t.Error("startupProbe not stripped")
	}
	if c.SecurityContext != nil {
		t.Error("container securityContext not stripped")
	}
	if len(c.Resources.Requests) > 0 || len(c.Resources.Limits) > 0 {
		t.Error("resources not stripped")
	}

	// Stripped: status fields.
	if got.Status.PodIP != "" {
		t.Error("podIP not stripped")
	}
	if got.Status.HostIP != "" {
		t.Error("hostIP not stripped")
	}
	if got.Status.PodIPs != nil {
		t.Error("podIPs not stripped")
	}
	if got.Status.Conditions != nil {
		t.Error("conditions not stripped")
	}
	if got.Status.StartTime != nil {
		t.Error("startTime not stripped")
	}

	// Stripped: container status fields (except waiting).
	cs := got.Status.ContainerStatuses[0]
	if cs.Ready {
		t.Error("ready not stripped")
	}
	if cs.RestartCount != 0 {
		t.Error("restartCount not stripped")
	}
	if cs.ContainerID != "" {
		t.Error("containerID not stripped")
	}
	if cs.Started != nil {
		t.Error("started not stripped")
	}
	if cs.State.Running != nil {
		t.Error("state.running not stripped")
	}
	if cs.State.Terminated != nil {
		t.Error("state.terminated not stripped")
	}
	if cs.LastTerminationState.Terminated != nil {
		t.Error("lastTerminationState not stripped")
	}
}

func TestStripPodFields_NonPod(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	result, err := stripPodFields(svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != svc {
		t.Error("non-pod object was modified")
	}
}
