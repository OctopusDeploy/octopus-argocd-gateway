// This implementation is based on code from Argo CD:
// https://github.com/argoproj/argo-cd/blob/c32286a9a4760bee528f84cce9e92f210e89bedd/pkg/apiclient/grpcproxy.go
//
// Original Copyright (c) The Argo CD Authors.
// Licensed under the Apache License 2.0.
//
// The code was modified and refactored to support gRPC-Web proxy usage
// in the octopus-argocd-gateway project.
package argocd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	frameHeaderLength = 5
	endOfStreamFlag   = 128
)

type noopCodec struct{}

func (noopCodec) Marshal(v any) ([]byte, error) {
	return v.([]byte), nil
}

func (noopCodec) Unmarshal(data []byte, v any) error {
	pointer := v.(*[]byte)
	*pointer = data
	return nil
}

func (noopCodec) Name() string {
	return "proto"
}

func toFrame(msg []byte) []byte {
	frame := append([]byte{0, 0, 0, 0}, msg...)
	binary.BigEndian.PutUint32(frame, uint32(len(msg)))
	frame = append([]byte{0}, frame...)
	return frame
}

func httpStatusToGRPCCode(httpStatus int) codes.Code {
	switch httpStatus {
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	default:
		if httpStatus >= 500 {
			return codes.Unavailable
		}
		return codes.Internal
	}
}

func sanitizeServerAddr(url string) string {
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	return strings.TrimRight(url, "/")
}

// executeRequest sends a grpc-web HTTP request to the upstream Argo CD server
// and returns the response.
func (c *client) executeRequest(
	ctx context.Context, fullMethodName string, msg []byte, md metadata.MD,
) (*http.Response, error) {
	schema := "https"
	if c.plainText {
		schema = "http"
	}
	rootPath := strings.TrimRight(strings.TrimLeft(c.grpcWebRootPath, "/"), "/")

	requestURL := fmt.Sprintf("%s://%s%s", schema, c.serverAddr, fullMethodName)
	if rootPath != "" {
		requestURL = fmt.Sprintf("%s://%s/%s%s", schema, c.serverAddr, rootPath, fullMethodName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(toFrame(msg)))
	if err != nil {
		return nil, err
	}
	for k, v := range md {
		if strings.HasPrefix(k, ":") {
			continue
		}
		for i := range v {
			req.Header.Set(k, v[i])
		}
	}
	req.Header.Set("content-type", "application/grpc-web+proto")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.Body != nil {
			resp.Body.Close()
		}
		return nil, status.Errorf(
			httpStatusToGRPCCode(resp.StatusCode), "%s %s: HTTP %d", req.Method, req.URL.Path, resp.StatusCode,
		)
	}
	if statusStr := resp.Header.Get("Grpc-Status"); statusStr != "" {
		statusInt, err := strconv.ParseUint(statusStr, 10, 32)
		var code codes.Code
		if err != nil {
			code = codes.Unknown
		} else {
			code = codes.Code(statusInt)
		}
		if code != codes.OK {
			if resp.Body != nil {
				resp.Body.Close()
			}
			return nil, status.Error(code, resp.Header.Get("Grpc-Message"))
		}
	}
	return resp, nil
}

// startGRPCProxy starts a local gRPC server on a Unix socket.
// It forwards all incoming gRPC calls to the upstream Argo CD server using grpc-web.
func (c *client) startGRPCProxy(ctx context.Context) (*grpc.Server, net.Listener, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random socket filename: %w", err)
	}
	socketAddr := fmt.Sprintf("%s/argocd-%s.sock", os.TempDir(), hex.EncodeToString(b))

	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "unix", socketAddr)
	if err != nil {
		return nil, nil, err
	}

	proxySrv := grpc.NewServer(
		grpc.ForceServerCodec(&noopCodec{}),
		grpc.KeepaliveEnforcementPolicy(
			keepalive.EnforcementPolicy{
				MinTime: 10 * time.Second,
			},
		),
		grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
			fullMethodName, ok := grpc.MethodFromServerStream(stream)
			if !ok {
				return errors.New("unable to get method name from stream context")
			}

			msg := make([]byte, 0)
			if err := stream.RecvMsg(&msg); err != nil {
				return err
			}

			md, _ := metadata.FromIncomingContext(stream.Context())

			resp, err := c.executeRequest(stream.Context(), fullMethodName, msg, md)
			if err != nil {
				if _, ok := status.FromError(err); !ok {
					return status.Errorf(codes.Unavailable, "ArgoCD upstream connection failed: %v", err)
				}
				return err
			}

			go func() {
				<-stream.Context().Done()
				resp.Body.Close()
			}()
			defer resp.Body.Close()
			c.httpClient.CloseIdleConnections()

			for {
				header := make([]byte, frameHeaderLength)
				if _, err := io.ReadAtLeast(resp.Body, header, frameHeaderLength); err != nil {
					return status.Errorf(codes.Unavailable, "ArgoCD upstream closed the stream: %v", err)
				}

				if header[0] == endOfStreamFlag {
					return nil
				}

				length := int(binary.BigEndian.Uint32(header[1:frameHeaderLength]))
				data := make([]byte, length)
				if _, err := io.ReadAtLeast(resp.Body, data, length); err != nil {
					return status.Errorf(codes.Unavailable, "ArgoCD upstream closed the stream: %v", err)
				}

				if err := stream.SendMsg(data); err != nil {
					return err
				}
			}
		}),
	)

	go func() {
		if err := proxySrv.Serve(ln); err != nil {
			c.logger.Error("GRPCWeb proxy server stopped with error", slog.Any("error", err))
		}
	}()

	return proxySrv, ln, nil
}

// useGRPCProxy makes sure the gRPC proxy is running and returns a closer
// that stops it when it is no longer in use.
func (c *client) useGRPCProxy(ctx context.Context) (net.Addr, io.Closer, error) {
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()

	if c.proxyListener == nil {
		var err error
		c.proxyServer, c.proxyListener, err = c.startGRPCProxy(ctx)
		if err != nil {
			return nil, nil, err
		}
	}
	c.proxyUsersCount++

	return c.proxyListener.Addr(), closerFunc(func() error {
		c.proxyMutex.Lock()
		defer c.proxyMutex.Unlock()
		c.proxyUsersCount--
		if c.proxyUsersCount == 0 {
			c.proxyServer.Stop()
			c.proxyListener = nil
			c.proxyServer = nil
		}
		return nil
	}), nil
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}
