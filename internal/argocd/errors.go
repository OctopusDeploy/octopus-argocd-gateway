package argocd

import (
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

var (
	ErrConnectionFailed = errors.New("failed to connect to ArgoCD server")
	ErrUnauthenticated  = errors.New("ArgoCD authentication failed")
	ErrNotFound         = errors.New("ArgoCD resource not found")
	ErrPermissionDenied = errors.New("ArgoCD permission denied")
)

type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeConnection
	ErrorTypeStream
	ErrorTypeStreamIgnore
	ErrorTypeRequest
	ErrorTypeAuth
	ErrorTypeNotFound
	ErrorTypePermission
)

func ClassifyError(err error) ErrorType {
	if err == io.EOF {
		return ErrorTypeStream
	}

	grpcErr := status.Convert(err)
	switch grpcErr.Code() {
	case codes.Unavailable, codes.Aborted:
		return ErrorTypeConnection
	case codes.Canceled:
		return ErrorTypeStreamIgnore
	case codes.Unauthenticated:
		return ErrorTypeAuth
	case codes.NotFound:
		return ErrorTypeNotFound
	case codes.PermissionDenied:
		return ErrorTypePermission
	default:
		return ErrorTypeRequest
	}
}

func IsRecoverable(err error) bool {
	errType := ClassifyError(err)
	return errType == ErrorTypeConnection || errType == ErrorTypeStream
}

func IsArgoError(err error) bool {
	return errors.Is(err, ErrConnectionFailed) ||
		errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrPermissionDenied)
}

func WrapRequestError(err error, operation string) error {
	if err == nil {
		return nil
	}

	errType := ClassifyError(err)
	switch errType {
	case ErrorTypeAuth:
		return fmt.Errorf("%w: %s: %v", ErrUnauthenticated, operation, err)
	case ErrorTypeNotFound:
		return fmt.Errorf("%w: %s: %v", ErrNotFound, operation, err)
	case ErrorTypePermission:
		return fmt.Errorf("%w: %s: %v", ErrPermissionDenied, operation, err)
	case ErrorTypeConnection:
		return fmt.Errorf("%w: %s: %v", ErrConnectionFailed, operation, err)
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}

func WrapIfUnauthenticated(err error) error {
	grpcErr := status.Convert(err)
	if grpcErr.Code() == codes.Unauthenticated {
		return fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	return err
}

type ErrorPolicy struct{}

var DefaultErrorPolicy = &ErrorPolicy{}

func (p *ErrorPolicy) Handle(err error) runner.ErrorAction {
	errType := ClassifyError(err)
	switch errType {
	case ErrorTypeConnection, ErrorTypeStream:
		return runner.ActionRetry
	case ErrorTypeAuth:
		return runner.ActionFatal
	default:
		return runner.ActionIgnore
	}
}

func (p *ErrorPolicy) Wrap(err error) error {
	return WrapIfUnauthenticated(err)
}
