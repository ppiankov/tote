package detector

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func podWithContainerStatus(statuses ...corev1.ContainerStatus) *corev1.Pod {
	specs := make([]corev1.Container, len(statuses))
	for i, s := range statuses {
		specs[i] = corev1.Container{Name: s.Name, Image: "registry.example.com/" + s.Name + ":v1"}
	}
	return &corev1.Pod{
		Spec:   corev1.PodSpec{Containers: specs},
		Status: corev1.PodStatus{ContainerStatuses: statuses},
	}
}

func waitingStatus(name, reason, msg string) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: name,
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  reason,
				Message: msg,
			},
		},
	}
}

func runningStatus(name string) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:  name,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
}

func TestDetect_NoFailures(t *testing.T) {
	pod := podWithContainerStatus(runningStatus("app"))
	failures := Detect(pod)
	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(failures))
	}
}

func TestDetect_ImagePullBackOff(t *testing.T) {
	pod := podWithContainerStatus(waitingStatus("app", "ImagePullBackOff", "pull failed"))
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].Reason != "ImagePullBackOff" {
		t.Errorf("expected reason ImagePullBackOff, got %q", failures[0].Reason)
	}
	if failures[0].ContainerName != "app" {
		t.Errorf("expected container name 'app', got %q", failures[0].ContainerName)
	}
}

func TestDetect_ErrImagePull(t *testing.T) {
	pod := podWithContainerStatus(waitingStatus("app", "ErrImagePull", "image not found"))
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].Reason != "ErrImagePull" {
		t.Errorf("expected reason ErrImagePull, got %q", failures[0].Reason)
	}
}

func TestDetect_MultipleFailures(t *testing.T) {
	pod := podWithContainerStatus(
		waitingStatus("app", "ImagePullBackOff", ""),
		waitingStatus("sidecar", "ErrImagePull", ""),
	)
	failures := Detect(pod)
	if len(failures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(failures))
	}
}

func TestDetect_InitContainerFailure(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init", Image: "registry.example.com/init:v1"}},
		},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{
				waitingStatus("init", "ImagePullBackOff", ""),
			},
		},
	}
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].ContainerName != "init" {
		t.Errorf("expected container name 'init', got %q", failures[0].ContainerName)
	}
}

func TestDetect_MixedStates(t *testing.T) {
	pod := podWithContainerStatus(
		runningStatus("healthy"),
		waitingStatus("broken", "ImagePullBackOff", ""),
	)
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].ContainerName != "broken" {
		t.Errorf("expected container name 'broken', got %q", failures[0].ContainerName)
	}
}

func TestDetect_WaitingOtherReason(t *testing.T) {
	pod := podWithContainerStatus(waitingStatus("app", "CrashLoopBackOff", ""))
	failures := Detect(pod)
	if len(failures) != 0 {
		t.Errorf("expected 0 failures for CrashLoopBackOff, got %d", len(failures))
	}
}

func TestDetect_ImageFromSpec(t *testing.T) {
	pod := podWithContainerStatus(waitingStatus("app", "ImagePullBackOff", ""))
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].Image != "registry.example.com/app:v1" {
		t.Errorf("expected image from spec, got %q", failures[0].Image)
	}
}

func TestDetect_CreateContainerError_CorruptImage(t *testing.T) {
	msg := "failed to create containerd container: failed to resolve rootfs: content digest sha256:b50153e8abcd1234: not found"
	pod := podWithContainerStatus(waitingStatus("app", "CreateContainerError", msg))
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].Reason != "CreateContainerError" {
		t.Errorf("expected reason CreateContainerError, got %q", failures[0].Reason)
	}
	if !failures[0].CorruptImage {
		t.Error("expected CorruptImage=true")
	}
}

func TestDetect_CreateContainerError_NotCorrupt(t *testing.T) {
	msg := "some other container creation error"
	pod := podWithContainerStatus(waitingStatus("app", "CreateContainerError", msg))
	failures := Detect(pod)
	if len(failures) != 0 {
		t.Errorf("expected 0 failures for non-corrupt CreateContainerError, got %d", len(failures))
	}
}

func TestDetect_ImagePullBackOff_NotCorrupt(t *testing.T) {
	pod := podWithContainerStatus(waitingStatus("app", "ImagePullBackOff", "pull failed"))
	failures := Detect(pod)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].CorruptImage {
		t.Error("expected CorruptImage=false for ImagePullBackOff")
	}
}
