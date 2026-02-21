package registry

import "testing"

func TestBackupRef(t *testing.T) {
	tests := []struct {
		name     string
		original string
		backup   string
		want     string
	}{
		{"full ref with tag", "registry.example.com/team/app:v1", "backup.example.com:5000", "backup.example.com:5000/team/app:v1"},
		{"digest stripped", "registry.example.com/team/app@sha256:abc", "backup.example.com:5000", "backup.example.com:5000/team/app:latest"},
		{"docker hub path", "library/nginx:latest", "backup.example.com:5000", "backup.example.com:5000/library/nginx:latest"},
		{"bare image", "nginx:latest", "backup.example.com:5000", "backup.example.com:5000/nginx:latest"},
		{"port in original", "registry.internal:5000/app:v1", "backup.example.com:5000", "backup.example.com:5000/app:v1"},
		{"nested path", "registry.example.com/org/team/app:v2", "backup.example.com:5000", "backup.example.com:5000/org/team/app:v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BackupRef(tt.original, tt.backup)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBackupRef_EmptyRegistry(t *testing.T) {
	_, err := BackupRef("nginx:latest", "")
	if err == nil {
		t.Fatal("expected error for empty backup registry")
	}
}
