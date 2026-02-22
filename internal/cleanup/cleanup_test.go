package cleanup

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/ppiankov/tote/api/v1alpha1"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func TestReaper_DeletesExpiredRecords(t *testing.T) {
	old := &v1alpha1.SalvageRecord{
		ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "default"},
		Status: v1alpha1.SalvageRecordStatus{
			Phase:       "Completed",
			CompletedAt: time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	recent := &v1alpha1.SalvageRecord{
		ObjectMeta: metav1.ObjectMeta{Name: "recent", Namespace: "default"},
		Status: v1alpha1.SalvageRecordStatus{
			Phase:       "Completed",
			CompletedAt: time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
		},
	}

	cl := fake.NewClientBuilder().WithScheme(newScheme()).
		WithRuntimeObjects(old, recent).Build()

	r := NewReaper(cl, 24*time.Hour, 5*time.Minute)
	r.sweep(context.Background(), &nopLogger{})

	var list v1alpha1.SalvageRecordList
	if err := cl.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 record remaining, got %d", len(list.Items))
	}
	if list.Items[0].Name != "recent" {
		t.Errorf("expected 'recent' to remain, got %q", list.Items[0].Name)
	}
}

func TestReaper_SkipsNoCompletedAt(t *testing.T) {
	rec := &v1alpha1.SalvageRecord{
		ObjectMeta: metav1.ObjectMeta{Name: "no-time", Namespace: "default"},
		Status:     v1alpha1.SalvageRecordStatus{Phase: "Failed"},
	}

	cl := fake.NewClientBuilder().WithScheme(newScheme()).
		WithRuntimeObjects(rec).Build()

	r := NewReaper(cl, 24*time.Hour, 5*time.Minute)
	r.sweep(context.Background(), &nopLogger{})

	var list v1alpha1.SalvageRecordList
	if err := cl.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected record to remain, got %d", len(list.Items))
	}
}

func TestReaper_NeedLeaderElection(t *testing.T) {
	r := &Reaper{}
	if !r.NeedLeaderElection() {
		t.Error("expected NeedLeaderElection to return true")
	}
}

type nopLogger struct{}

func (n *nopLogger) Info(_ string, _ ...interface{}) {}
