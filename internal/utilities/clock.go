package utilities

//go:generate mockgen -package=mock -destination=../testutilities/mock/clock_mock.go github.com/OctopusDeploy/octopus-argocd-gateway/internal/utilities Clock

import "time"

// Clock Wrapper around time.Time package to provide an interface that can be mocked in tests
type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func NewClock() Clock {
	return &RealClock{}
}

func (RealClock) Now() time.Time {
	return time.Now()
}
