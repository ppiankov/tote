package detector

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Failure represents a detected image pull failure for a single container.
type Failure struct {
	ContainerName string
	Image         string
	Reason        string
	Message       string
	// CorruptImage is true when the image record exists but content blobs
	// are missing (CreateContainerError with rootfs resolution failure).
	CorruptImage bool
}

var imagePullFailureReasons = map[string]bool{
	"ImagePullBackOff": true,
	"ErrImagePull":     true,
}

// Detect inspects a Pod and returns any image pull failures found across
// both regular and init container statuses.
func Detect(pod *corev1.Pod) []Failure {
	var failures []Failure
	failures = append(failures, detectInStatuses(pod.Status.ContainerStatuses, pod.Spec.Containers)...)
	failures = append(failures, detectInStatuses(pod.Status.InitContainerStatuses, pod.Spec.InitContainers)...)
	return failures
}

func detectInStatuses(statuses []corev1.ContainerStatus, specs []corev1.Container) []Failure {
	specImage := make(map[string]string, len(specs))
	for _, c := range specs {
		specImage[c.Name] = c.Image
	}

	var failures []Failure
	for _, cs := range statuses {
		if cs.State.Waiting == nil {
			continue
		}
		reason := cs.State.Waiting.Reason
		msg := cs.State.Waiting.Message

		if imagePullFailureReasons[reason] {
			failures = append(failures, Failure{
				ContainerName: cs.Name,
				Image:         specImage[cs.Name],
				Reason:        reason,
				Message:       msg,
			})
			continue
		}

		// Detect corrupt images: containerd has the image record but content
		// blobs are missing. kubelet says "already present" but container
		// creation fails with rootfs resolution error.
		if reason == "CreateContainerError" && isCorruptImageMessage(msg) {
			failures = append(failures, Failure{
				ContainerName: cs.Name,
				Image:         specImage[cs.Name],
				Reason:        reason,
				Message:       msg,
				CorruptImage:  true,
			})
		}
	}
	return failures
}

// isCorruptImageMessage returns true if the message indicates missing content
// blobs (image record exists but layers are gone).
func isCorruptImageMessage(msg string) bool {
	return strings.Contains(msg, "failed to resolve rootfs") &&
		strings.Contains(msg, "content digest") &&
		strings.Contains(msg, "not found")
}
