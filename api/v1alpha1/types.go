package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SalvageRecordSpec describes a salvage operation.
type SalvageRecordSpec struct {
	// PodName is the name of the pod that triggered the salvage.
	PodName string `json:"podName"`

	// Digest is the image content digest (sha256:...).
	Digest string `json:"digest"`

	// ImageRef is the original image reference from the pod spec.
	ImageRef string `json:"imageRef"`

	// SourceNode is the node the image was exported from.
	SourceNode string `json:"sourceNode"`

	// TargetNode is the node the image was imported to.
	TargetNode string `json:"targetNode"`
}

// SalvageRecordStatus describes the outcome of a salvage operation.
type SalvageRecordStatus struct {
	// Phase is the current state: Completed or Failed.
	Phase string `json:"phase"`

	// CompletedAt is when the salvage finished (RFC3339).
	// +optional
	CompletedAt string `json:"completedAt,omitempty"`

	// Error is the failure reason (empty on success).
	// +optional
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Digest",type=string,JSONPath=`.spec.digest`,priority=0
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.sourceNode`,priority=0
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetNode`,priority=0
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0

// SalvageRecord tracks a single image salvage operation.
type SalvageRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SalvageRecordSpec   `json:"spec,omitempty"`
	Status SalvageRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SalvageRecordList contains a list of SalvageRecord.
type SalvageRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SalvageRecord `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SalvageRecord{}, &SalvageRecordList{})
}
