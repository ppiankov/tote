package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var knownAnnotations = map[string]bool{
	"tote.dev/allow":        true,
	"tote.dev/auto-salvage": true,
}

// AnnotationValidator rejects Pods and Namespaces with unknown tote.dev/*
// annotations or invalid annotation values.
type AnnotationValidator struct{}

// Handle validates tote.dev/* annotations on any admitted object.
func (v *AnnotationValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	var meta struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(req.Object.Raw, &meta); err != nil {
		return admission.Allowed("") // fail open on decode error
	}

	for key, value := range meta.Metadata.Annotations {
		if !strings.HasPrefix(key, "tote.dev/") {
			continue
		}
		if !knownAnnotations[key] {
			return admission.Denied(fmt.Sprintf(
				"unknown tote.dev annotation %q; valid annotations: tote.dev/allow, tote.dev/auto-salvage", key))
		}
		if value != "true" && value != "false" {
			return admission.Denied(fmt.Sprintf(
				"annotation %q must be \"true\" or \"false\", got %q", key, value))
		}
	}
	return admission.Allowed("")
}
