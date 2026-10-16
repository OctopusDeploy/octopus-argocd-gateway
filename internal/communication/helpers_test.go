package communication

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPSTLS_SucceedsWithValidTLSServer(t *testing.T) {
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()
	err := TestHTTPSTLS(t.Context(), srv.Listener.Addr().String(), 5*time.Second, true)
	require.NoError(t, err)
}

func TestHTTPSTLS_FailsForUnreachableAddress(t *testing.T) {
	err := TestHTTPSTLS(t.Context(), "127.0.0.1:1", 5*time.Second, false)
	assert.Error(t, err)
}

func TestSanitizeGrpcUrl_ShouldStripGrpcPrefix(t *testing.T) {
	// Arrange
	test := "GrPc://my-url-gRPC.com"
	expected := "my-url-grpc.com"

	// Act
	result := SanitizeGrpcUrl(test)

	// Assert
	cmp.Equal(expected, result)
}
