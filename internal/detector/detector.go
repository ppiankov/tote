package detector

import corev1 "k8s.io/api/core/v1"

// Failure represents a detected image pull failure for a single container.
type Failure struct {
	ContainerName string
	Image         string
	Reason        string
	Message       string
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
		if !imagePullFailureReasons[cs.State.Waiting.Reason] {
			continue
		}
		failures = append(failures, Failure{
			ContainerName: cs.Name,
			Image:         specImage[cs.Name],
			Reason:        cs.State.Waiting.Reason,
			Message:       cs.State.Waiting.Message,
		})
	}
	return failures
}
