package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/ppiankov/tote/api/v1"
	"github.com/ppiankov/tote/internal/session"
)

func startTestServer(t *testing.T, store ImageStore, sessions *session.Store) (v1.ToteAgentClient, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	agent := &Server{Store: store, Sessions: sessions}
	v1.RegisterToteAgentServer(srv, agent)

	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		srv.Stop()
		t.Fatalf("dial: %v", err)
	}

	client := v1.NewToteAgentClient(conn)
	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
	}
	return client, cleanup
}

func TestListImages(t *testing.T) {
	store := NewFakeImageStore()
	store.AddImage("sha256:aaa", []byte("data-a"))
	store.AddImage("sha256:bbb", []byte("data-b"))
	sessions := session.NewStore()

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	resp, err := client.ListImages(context.Background(), &v1.ListImagesRequest{})
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(resp.Digests) != 2 {
		t.Errorf("expected 2 digests, got %d", len(resp.Digests))
	}
}

func TestListImages_Empty(t *testing.T) {
	store := NewFakeImageStore()
	sessions := session.NewStore()

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	resp, err := client.ListImages(context.Background(), &v1.ListImagesRequest{})
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(resp.Digests) != 0 {
		t.Errorf("expected 0 digests, got %d", len(resp.Digests))
	}
}

func TestPrepareExport_Success(t *testing.T) {
	store := NewFakeImageStore()
	store.AddImage("sha256:aaa", []byte("data"))
	sessions := session.NewStore()
	sess := sessions.Create("sha256:aaa", "node-a", "node-b", 5*time.Minute)

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	_, err := client.PrepareExport(context.Background(), &v1.PrepareExportRequest{
		SessionToken: sess.Token,
		Digest:       "sha256:aaa",
	})
	if err != nil {
		t.Fatalf("PrepareExport: %v", err)
	}
}

func TestPrepareExport_ImageNotFound(t *testing.T) {
	store := NewFakeImageStore()
	sessions := session.NewStore()
	sess := sessions.Create("sha256:missing", "node-a", "node-b", 5*time.Minute)

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	_, err := client.PrepareExport(context.Background(), &v1.PrepareExportRequest{
		SessionToken: sess.Token,
		Digest:       "sha256:missing",
	})
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestPrepareExport_InvalidSession(t *testing.T) {
	store := NewFakeImageStore()
	store.AddImage("sha256:aaa", []byte("data"))
	sessions := session.NewStore()

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	_, err := client.PrepareExport(context.Background(), &v1.PrepareExportRequest{
		SessionToken: "bad-token",
		Digest:       "sha256:aaa",
	})
	if err == nil {
		t.Fatal("expected error for invalid session")
	}
}

func TestExportImage_Success(t *testing.T) {
	store := NewFakeImageStore()
	store.AddImage("sha256:aaa", []byte("image-tar-data"))
	sessions := session.NewStore()
	sess := sessions.Create("sha256:aaa", "node-a", "node-b", 5*time.Minute)

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	stream, err := client.ExportImage(context.Background(), &v1.ExportImageRequest{
		SessionToken: sess.Token,
	})
	if err != nil {
		t.Fatalf("ExportImage: %v", err)
	}

	var received []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		received = append(received, chunk.Data...)
	}

	if string(received) != "image-tar-data" {
		t.Errorf("expected 'image-tar-data', got %q", string(received))
	}
}

func TestExportImage_InvalidSession(t *testing.T) {
	store := NewFakeImageStore()
	sessions := session.NewStore()

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	stream, err := client.ExportImage(context.Background(), &v1.ExportImageRequest{
		SessionToken: "bad-token",
	})
	if err != nil {
		t.Fatalf("ExportImage: %v", err)
	}
	// Error should come on first Recv
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("expected error for invalid session")
	}
}

func TestListImages_StoreError(t *testing.T) {
	store := &FailingImageStore{Err: fmt.Errorf("containerd down")}
	sessions := session.NewStore()

	client, cleanup := startTestServer(t, store, sessions)
	defer cleanup()

	_, err := client.ListImages(context.Background(), &v1.ListImagesRequest{})
	if err == nil {
		t.Fatal("expected error when store fails")
	}
}
