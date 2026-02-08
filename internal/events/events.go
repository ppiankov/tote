package events

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
)

const (
	// ReasonSalvageable indicates the image exists on cluster nodes.
	ReasonSalvageable = "ImageSalvageable"

	// ReasonNotActionable indicates the image uses a tag, not a digest.
	ReasonNotActionable = "ImageNotActionable"

	// ReasonSalvaged indicates the image was transferred between nodes.
	ReasonSalvaged = "ImageSalvaged"

	// ReasonSalvageFailed indicates the salvage attempt failed.
	ReasonSalvageFailed = "ImageSalvageFailed"

	actionDetected  = "Detected"
	actionSalvaged  = "Salvaged"
	actionSalvaging = "Salvaging"
)

// Emitter emits Kubernetes events for tote detections.
type Emitter struct {
	Recorder events.EventRecorder
}

// NewEmitter creates an Emitter with the given recorder.
func NewEmitter(recorder events.EventRecorder) *Emitter {
	return &Emitter{Recorder: recorder}
}

// EmitSalvageable emits a Warning event indicating the image digest exists on
// specific nodes.
func (e *Emitter) EmitSalvageable(pod *corev1.Pod, image string, nodes []string) {
	e.Recorder.Eventf(
		pod, nil, corev1.EventTypeWarning, ReasonSalvageable, actionDetected,
		"Registry pull failed for %s; image digest exists on nodes: [%s]. This is technical debt — rebuild and push the image properly.",
		image, strings.Join(nodes, ", "),
	)
}

// EmitNotActionable emits a Warning event indicating the image uses a tag,
// not a digest, so tote cannot determine cache locality.
func (e *Emitter) EmitNotActionable(pod *corev1.Pod, image string) {
	e.Recorder.Eventf(
		pod, nil, corev1.EventTypeWarning, ReasonNotActionable, actionDetected,
		"Not actionable: image %s uses tag, not digest. Pin images by digest for tote to help.",
		image,
	)
}

// EmitSalvaged emits a Warning event indicating the image was transferred
// between nodes via containerd.
func (e *Emitter) EmitSalvaged(pod *corev1.Pod, image, sourceNode, targetNode string) {
	e.Recorder.Eventf(
		pod, nil, corev1.EventTypeWarning, ReasonSalvaged, actionSalvaged,
		"Image %s salvaged from node %s to node %s via containerd. This is emergency — rebuild properly.",
		image, sourceNode, targetNode,
	)
}

// EmitSalvageFailed emits a Warning event indicating the salvage attempt failed.
func (e *Emitter) EmitSalvageFailed(pod *corev1.Pod, image, reason string) {
	e.Recorder.Eventf(
		pod, nil, corev1.EventTypeWarning, ReasonSalvageFailed, actionSalvaging,
		"Image salvage failed for %s: %s",
		image, reason,
	)
}
