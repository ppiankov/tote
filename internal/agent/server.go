package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "github.com/ppiankov/tote/api/v1"
	"github.com/ppiankov/tote/internal/registry"
	"github.com/ppiankov/tote/internal/session"
)

const exportChunkSize = 32 * 1024 // 32 KiB

// Server implements the ToteAgent gRPC service.
type Server struct {
	v1.UnimplementedToteAgentServer
	Store    ImageStore
	Sessions *session.Store
	Port     int
}

// NewServer creates a new agent gRPC server.
func NewServer(store ImageStore, sessions *session.Store, port int) *Server {
	return &Server{
		Store:    store,
		Sessions: sessions,
		Port:     port,
	}
}

// Start starts the gRPC server and blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", s.Port, err)
	}

	srv := grpc.NewServer()
	v1.RegisterToteAgentServer(srv, s)

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()

	return srv.Serve(lis)
}

// PrepareExport verifies the digest exists locally and registers the session.
func (s *Server) PrepareExport(ctx context.Context, req *v1.PrepareExportRequest) (*v1.PrepareExportResponse, error) {
	if req.SessionToken == "" || req.Digest == "" {
		return nil, fmt.Errorf("session_token and digest are required")
	}

	has, err := s.Store.Has(ctx, req.Digest)
	if err != nil {
		return nil, fmt.Errorf("checking image: %w", err)
	}
	if !has {
		return nil, fmt.Errorf("image %s not found locally", req.Digest)
	}

	sizeBytes, err := s.Store.Size(ctx, req.Digest)
	if err != nil {
		return nil, fmt.Errorf("getting image size: %w", err)
	}

	// Register the session locally so ExportImage can look up the digest.
	// The token was created by the controller's orchestrator.
	s.Sessions.Register(req.SessionToken, req.Digest, 5*time.Minute)

	return &v1.PrepareExportResponse{SizeBytes: sizeBytes}, nil
}

// ExportImage streams the image tar for the session's digest.
func (s *Server) ExportImage(req *v1.ExportImageRequest, stream v1.ToteAgent_ExportImageServer) error {
	if req.SessionToken == "" {
		return fmt.Errorf("session_token is required")
	}

	sess, ok := s.Sessions.Validate(req.SessionToken)
	if !ok {
		return fmt.Errorf("invalid or expired session token")
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	go func() {
		errCh <- s.Store.Export(stream.Context(), sess.Digest, pw)
		_ = pw.Close()
	}()

	buf := make([]byte, exportChunkSize)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&v1.DataChunk{Data: buf[:n]}); sendErr != nil {
				_ = pr.Close()
				return fmt.Errorf("sending chunk: %w", sendErr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading export: %w", err)
		}
	}

	if err := <-errCh; err != nil {
		return fmt.Errorf("export: %w", err)
	}

	return nil
}

// ImportFrom connects to the source agent, streams the image, and imports it.
func (s *Server) ImportFrom(ctx context.Context, req *v1.ImportFromRequest) (*v1.ImportFromResponse, error) {
	if req.SessionToken == "" || req.Digest == "" || req.SourceEndpoint == "" {
		return &v1.ImportFromResponse{Success: false, Error: "session_token, digest, and source_endpoint are required"}, nil
	}

	conn, err := grpc.NewClient(req.SourceEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return &v1.ImportFromResponse{Success: false, Error: fmt.Sprintf("connecting to source: %v", err)}, nil
	}
	defer func() { _ = conn.Close() }()

	source := v1.NewToteAgentClient(conn)
	stream, err := source.ExportImage(ctx, &v1.ExportImageRequest{SessionToken: req.SessionToken})
	if err != nil {
		return &v1.ImportFromResponse{Success: false, Error: fmt.Sprintf("starting export stream: %v", err)}, nil
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)

	go func() {
		defer func() { _ = pw.Close() }()
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("receiving chunk: %w", err)
				return
			}
			if _, err := pw.Write(chunk.Data); err != nil {
				errCh <- fmt.Errorf("writing to pipe: %w", err)
				return
			}
		}
	}()

	digest, err := s.Store.Import(ctx, pr)
	if err != nil {
		return &v1.ImportFromResponse{Success: false, Error: fmt.Sprintf("importing image: %v", err)}, nil
	}

	// Check for stream errors
	select {
	case streamErr := <-errCh:
		return &v1.ImportFromResponse{Success: false, Error: fmt.Sprintf("stream error: %v", streamErr)}, nil
	default:
	}

	// Verify the imported digest matches
	if digest != req.Digest {
		// Verify by checking containerd directly
		has, err := s.Store.Has(ctx, req.Digest)
		if err != nil || !has {
			return &v1.ImportFromResponse{
				Success: false,
				Error:   fmt.Sprintf("imported digest %s does not match requested %s", digest, req.Digest),
			}, nil
		}
	}

	return &v1.ImportFromResponse{Success: true}, nil
}

// ListImages returns all image digests from the local containerd store.
func (s *Server) ListImages(ctx context.Context, _ *v1.ListImagesRequest) (*v1.ListImagesResponse, error) {
	digests, err := s.Store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	return &v1.ListImagesResponse{Digests: digests}, nil
}

// ResolveTag looks up an image reference in containerd and returns its digest.
func (s *Server) ResolveTag(ctx context.Context, req *v1.ResolveTagRequest) (*v1.ResolveTagResponse, error) {
	if req.ImageRef == "" {
		return nil, fmt.Errorf("image_ref is required")
	}
	digest, err := s.Store.ResolveTag(ctx, req.ImageRef)
	if err != nil {
		return nil, fmt.Errorf("resolving tag: %w", err)
	}
	return &v1.ResolveTagResponse{Digest: digest}, nil
}

// RemoveImage deletes an image record from the local containerd store.
func (s *Server) RemoveImage(ctx context.Context, req *v1.RemoveImageRequest) (*v1.RemoveImageResponse, error) {
	if req.ImageRef == "" {
		return nil, fmt.Errorf("image_ref is required")
	}
	if err := s.Store.Remove(ctx, req.ImageRef); err != nil {
		return nil, fmt.Errorf("removing image: %w", err)
	}
	return &v1.RemoveImageResponse{}, nil
}

// PushImage exports the image from the local containerd store and pushes it
// to a remote backup registry.
func (s *Server) PushImage(ctx context.Context, req *v1.PushImageRequest) (*v1.PushImageResponse, error) {
	if req.Digest == "" || req.TargetRef == "" {
		return &v1.PushImageResponse{Success: false, Error: "digest and target_ref are required"}, nil
	}

	has, err := s.Store.Has(ctx, req.Digest)
	if err != nil {
		return &v1.PushImageResponse{Success: false, Error: fmt.Sprintf("checking image: %v", err)}, nil
	}
	if !has {
		return &v1.PushImageResponse{Success: false, Error: fmt.Sprintf("image %s not found locally", req.Digest)}, nil
	}

	exportFn := func(ctx context.Context, digest string, w io.Writer) error {
		return s.Store.Export(ctx, digest, w)
	}
	if err := registry.Push(ctx, exportFn, req.Digest, req.TargetRef, req.RegistryUsername, req.RegistryPassword, req.Insecure); err != nil {
		return &v1.PushImageResponse{Success: false, Error: fmt.Sprintf("push failed: %v", err)}, nil
	}

	return &v1.PushImageResponse{Success: true}, nil
}
