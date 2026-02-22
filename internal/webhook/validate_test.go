package webhook

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func makeRequest(annotations map[string]string) admission.Request {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": annotations,
		},
	}
	raw, _ := json.Marshal(obj)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw},
		},
	}
}

func TestAnnotationValidator_AllowsValid(t *testing.T) {
	v := &AnnotationValidator{}
	resp := v.Handle(context.Background(), makeRequest(map[string]string{
		"tote.dev/auto-salvage": "true",
		"tote.dev/allow":        "false",
	}))
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestAnnotationValidator_AllowsNonToteAnnotations(t *testing.T) {
	v := &AnnotationValidator{}
	resp := v.Handle(context.Background(), makeRequest(map[string]string{
		"app.kubernetes.io/name": "myapp",
	}))
	if !resp.Allowed {
		t.Error("expected allowed for non-tote annotations")
	}
}

func TestAnnotationValidator_DeniesUnknownAnnotation(t *testing.T) {
	v := &AnnotationValidator{}
	resp := v.Handle(context.Background(), makeRequest(map[string]string{
		"tote.dev/auto-slavage": "true", // typo
	}))
	if resp.Allowed {
		t.Error("expected denied for unknown tote.dev annotation")
	}
}

func TestAnnotationValidator_DeniesInvalidValue(t *testing.T) {
	v := &AnnotationValidator{}
	resp := v.Handle(context.Background(), makeRequest(map[string]string{
		"tote.dev/allow": "yes",
	}))
	if resp.Allowed {
		t.Error("expected denied for invalid value 'yes'")
	}
}

func TestAnnotationValidator_AllowsNoAnnotations(t *testing.T) {
	v := &AnnotationValidator{}
	resp := v.Handle(context.Background(), makeRequest(nil))
	if !resp.Allowed {
		t.Error("expected allowed with no annotations")
	}
}

func TestAnnotationValidator_FailsOpenOnBadJSON(t *testing.T) {
	v := &AnnotationValidator{}
	resp := v.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: []byte("not json")},
		},
	})
	if !resp.Allowed {
		t.Error("expected fail-open on bad JSON")
	}
}
