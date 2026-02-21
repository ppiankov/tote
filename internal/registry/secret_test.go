package registry

import "testing"

func TestExtractCredentials_UsernamePassword(t *testing.T) {
	data := []byte(`{"auths":{"backup.example.com:5000":{"username":"admin","password":"s3cret"}}}`)
	u, p, err := ExtractCredentials(data, "backup.example.com:5000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "admin" || p != "s3cret" {
		t.Errorf("got %s:%s, want admin:s3cret", u, p)
	}
}

func TestExtractCredentials_AuthField(t *testing.T) {
	// base64("admin:s3cret") = "YWRtaW46czNjcmV0"
	data := []byte(`{"auths":{"backup.example.com:5000":{"auth":"YWRtaW46czNjcmV0"}}}`)
	u, p, err := ExtractCredentials(data, "backup.example.com:5000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "admin" || p != "s3cret" {
		t.Errorf("got %s:%s, want admin:s3cret", u, p)
	}
}

func TestExtractCredentials_NotFound(t *testing.T) {
	data := []byte(`{"auths":{"other.registry.com":{"username":"x","password":"y"}}}`)
	_, _, err := ExtractCredentials(data, "backup.example.com:5000")
	if err == nil {
		t.Fatal("expected error for missing registry")
	}
}

func TestExtractCredentials_HttpsPrefix(t *testing.T) {
	data := []byte(`{"auths":{"https://backup.example.com:5000":{"username":"admin","password":"s3cret"}}}`)
	u, p, err := ExtractCredentials(data, "backup.example.com:5000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "admin" || p != "s3cret" {
		t.Errorf("got %s:%s, want admin:s3cret", u, p)
	}
}

func TestExtractCredentials_InvalidJSON(t *testing.T) {
	_, _, err := ExtractCredentials([]byte("not json"), "backup.example.com:5000")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
