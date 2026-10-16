package octopus

import (
	"errors"
	"fmt"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/grpcerror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/runner"
)

var ErrUnauthorized = errors.New("gateway is unauthorized to connect to Octopus Server")

func WrapIfUnauthorized(err error) error {
	grpcErr := status.Convert(err)
	if grpcErr.Code() == codes.Unauthenticated {
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	return err
}

type ErrorPolicy struct{}

var DefaultErrorPolicy = &ErrorPolicy{}

// Handle maps the shared classification onto the runner's vocabulary, where Ignore
// is the word for a clean stop.
func (p *ErrorPolicy) Handle(err error) runner.ErrorAction {
	switch grpcerror.Classify(err) {
	case grpcerror.Retry:
		return runner.ActionRetry
	case grpcerror.Stop:
		return runner.ActionIgnore
	default:
		return runner.ActionFatal
	}
}

func (p *ErrorPolicy) Wrap(err error) error {
	return WrapIfUnauthorized(err)
}
