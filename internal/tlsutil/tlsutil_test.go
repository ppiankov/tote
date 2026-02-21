package tlsutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// testCerts holds paths to generated cert files.
type testCerts struct {
	caFile   string
	certFile string
	keyFile  string
}

// generateCerts creates an ephemeral CA and a leaf certificate signed by it.
// The leaf cert has the SAN "tote" to match ClientCredentials' ServerName.
func generateCerts(t *testing.T, dir string) testCerts {
	t.Helper()

	// Generate CA key and self-signed cert.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	// Generate leaf key and cert signed by CA.
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "tote"},
		DNSNames:     []string{"tote"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	// Write PEM files.
	caFile := filepath.Join(dir, "ca.crt")
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")

	writePEM(t, caFile, "CERTIFICATE", caDER)
	writePEM(t, certFile, "CERTIFICATE", leafDER)

	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, keyFile, "EC PRIVATE KEY", leafKeyDER)

	return testCerts{caFile: caFile, certFile: certFile, keyFile: keyFile}
}

func writePEM(t *testing.T, path, blockType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		t.Fatal(err)
	}
}

func TestServerCredentials(t *testing.T) {
	certs := generateCerts(t, t.TempDir())

	creds, err := ServerCredentials(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("ServerCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
}

func TestClientCredentials(t *testing.T) {
	certs := generateCerts(t, t.TempDir())

	creds, err := ClientCredentials(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
}

func TestServerCredentials_MissingFile(t *testing.T) {
	_, err := ServerCredentials("/nonexistent/cert", "/nonexistent/key", "/nonexistent/ca")
	if err == nil {
		t.Fatal("expected error for missing files")
	}
}

func TestClientCredentials_MissingFile(t *testing.T) {
	_, err := ClientCredentials("/nonexistent/cert", "/nonexistent/key", "/nonexistent/ca")
	if err == nil {
		t.Fatal("expected error for missing files")
	}
}

func TestServerCredentials_BadCA(t *testing.T) {
	dir := t.TempDir()
	certs := generateCerts(t, dir)
	// Overwrite CA with invalid data.
	badCA := filepath.Join(dir, "bad-ca.crt")
	if err := os.WriteFile(badCA, []byte("not a cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ServerCredentials(certs.certFile, certs.keyFile, badCA)
	if err == nil {
		t.Fatal("expected error for bad CA PEM")
	}
}

func TestMTLS_Integration(t *testing.T) {
	dir := t.TempDir()
	certs := generateCerts(t, dir)

	// Set up mTLS server.
	serverCreds, err := ServerCredentials(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("ServerCredentials: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(grpc.Creds(serverCreds))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	// Connect with mTLS client — should succeed.
	clientCreds, err := ClientCredentials(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected status: %v", resp.Status)
	}
}

func TestMTLS_WrongCA(t *testing.T) {
	dir := t.TempDir()
	certs := generateCerts(t, dir)

	// Generate a second CA (different trust root).
	dir2 := t.TempDir()
	otherCerts := generateCerts(t, dir2)

	// Server uses first CA.
	serverCreds, err := ServerCredentials(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("ServerCredentials: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(grpc.Creds(serverCreds))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	// Client uses second CA — should fail.
	clientCreds, err := ClientCredentials(otherCerts.certFile, otherCerts.keyFile, otherCerts.caFile)
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error with wrong CA, got nil")
	}
}

func TestMTLS_InsecureClientRejected(t *testing.T) {
	dir := t.TempDir()
	certs := generateCerts(t, dir)

	serverCreds, err := ServerCredentials(certs.certFile, certs.keyFile, certs.caFile)
	if err != nil {
		t.Fatalf("ServerCredentials: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(grpc.Creds(serverCreds))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	// Client with no TLS — should fail against mTLS server.
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error with insecure client against mTLS server, got nil")
	}
}
