package argocd

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func TestHTTPStatusToGRPCCode(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		expected   codes.Code
	}{
		{"200 OK", http.StatusOK, codes.Internal},
		{"400 Bad Request", http.StatusBadRequest, codes.Internal},
		{"401 Unauthorized", http.StatusUnauthorized, codes.Unauthenticated},
		{"403 Forbidden", http.StatusForbidden, codes.PermissionDenied},
		{"404 Not Found", http.StatusNotFound, codes.NotFound},
		{"429 Too Many Requests", http.StatusTooManyRequests, codes.Internal},
		{"500 Internal Server Error", http.StatusInternalServerError, codes.Unavailable},
		{"502 Bad Gateway", http.StatusBadGateway, codes.Unavailable},
		{"503 Service Unavailable", http.StatusServiceUnavailable, codes.Unavailable},
		{"504 Gateway Timeout", http.StatusGatewayTimeout, codes.Unavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, httpStatusToGRPCCode(tt.httpStatus))
		})
	}
}

func TestSanitizeServerAddr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"https scheme", "https://example.com:443", "example.com:443"},
		{"grpc scheme", "grpc://example.com:443", "example.com:443"},
		{"http scheme", "http://example.com:80", "example.com:80"},
		{"no scheme", "example.com:443", "example.com:443"},
		{"trailing slash", "https://example.com:443/", "example.com:443"},
		{"multiple trailing slashes", "https://example.com:443///", "example.com:443"},
		{"scheme and root path", "https://example.com:443/argocd/", "example.com:443/argocd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeServerAddr(tt.input))
		})
	}
}

func endOfStreamFrame() []byte {
	return []byte{endOfStreamFlag, 0, 0, 0, 0}
}

func newProxyClient(serverAddr string, plainText bool, rootPath string, httpClient *http.Client) *client {
	return &client{
		serverAddr:      serverAddr,
		plainText:       plainText,
		grpcWebRootPath: rootPath,
		httpClient:      httpClient,
		logger:          slog.Default(),
	}
}

func TestExecuteRequest_SuccessReturnsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(endOfStreamFrame())
	}))
	defer srv.Close()

	c := newProxyClient(srv.Listener.Addr().String(), true, "", srv.Client())
	resp, err := c.executeRequest(context.Background(), "/pkg.Service/Method", []byte{}, metadata.MD{})
	require.NoError(t, err)
	resp.Body.Close()
}

func TestExecuteRequest_SetsGRPCWebContentType(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("content-type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(endOfStreamFrame())
	}))
	defer srv.Close()

	c := newProxyClient(srv.Listener.Addr().String(), true, "", srv.Client())
	resp, err := c.executeRequest(context.Background(), "/pkg.Service/Method", []byte{}, metadata.MD{})
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, "application/grpc-web+proto", got)
}

func TestExecuteRequest_FramesRequestBody(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(endOfStreamFrame())
	}))
	defer srv.Close()

	msg := []byte{10, 20, 30}
	c := newProxyClient(srv.Listener.Addr().String(), true, "", srv.Client())
	resp, err := c.executeRequest(context.Background(), "/pkg.Service/Method", msg, metadata.MD{})
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, toFrame(msg), capturedBody)
}

func TestExecuteRequest_GRPCStatusOKIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Grpc-Status", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(endOfStreamFrame())
	}))
	defer srv.Close()

	c := newProxyClient(srv.Listener.Addr().String(), true, "", srv.Client())
	resp, err := c.executeRequest(context.Background(), "/pkg.Service/Method", []byte{}, metadata.MD{})
	require.NoError(t, err)
	resp.Body.Close()
}
