package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"go.uber.org/mock/gomock"
)

// NewMockApplicationResourceWatchClient Returns a pre-configured mock and a channel to use for mocking out messages sent on a stream
func NewMockApplicationResourceWatchClient(
	ctrl *gomock.Controller,
	ctx context.Context,
) (*MockApplicationService_WatchResourceTreeClient, chan *v1alpha1.ApplicationTree) {
	mockClient := NewMockApplicationService_WatchResourceTreeClient(ctrl)
	recvChan := make(chan *v1alpha1.ApplicationTree)

	mockClient.EXPECT().CloseSend().DoAndReturn(func() error {
		close(recvChan)
		return nil
	}).AnyTimes()

	mockClient.EXPECT().Recv().DoAndReturn(func() (*v1alpha1.ApplicationTree, error) {
		for {
			select {
			case result := <-recvChan:
				return result, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}).AnyTimes()

	return mockClient, recvChan
}

// WaitForMocks blocks the current goroutine until one of the following happens:
//
// 1. Mocks have been satisfied i.e. each expected function has being called - results in success.
//
// 2. Timeout occurs - results in test failure.
//
// 3. Context is canceled - results in test failure.
func WaitForMocks(ctx context.Context, ctrl *gomock.Controller, timeoutDuration time.Duration) error {
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(timeoutDuration)

	for {
		select {
		case <-ticker.C:
			if ctrl.Satisfied() {
				return nil
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for mocks to be satisfied")
		case <-ctx.Done():
			return fmt.Errorf("context cancelled")
		}
	}
}
