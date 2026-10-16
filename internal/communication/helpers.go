package communication

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/OctopusDeploy/octopus-grpc/go/pkg/certificate"
	"github.com/OctopusDeploy/octopus-grpc/go/pkg/connection"
	"go.uber.org/multierr"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	certpool "github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/configuration"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/logger"
)

var connectionValidationDialTimeout = 5 * time.Second

func SanitizeGrpcUrl(url string) string {
	// remove any leading grpc:// as it's not required
	sanitizedUrl := strings.TrimPrefix(strings.ToLower(url), "grpc://")

	// If port is unspecified, assume the most likely port
	if parts := strings.Split(sanitizedUrl, ":"); len(parts) == 1 {
		sanitizedUrl += ":443"
	}

	return sanitizedUrl
}

func logConnectionConfiguration(logger *slog.Logger, config *configuration.Configuration) {
	octopusConfig := map[string]string{
		"OctopusUrl":            config.Gateway.OctopusGrpcUrl,
		"OctopusThumbprint":     config.Gateway.OctopusGrpcThumbprint,
		"OctopusPinCertificate": fmt.Sprintf("%t", config.Gateway.OctopusGrpcPinCertificate),
		"OctopusPlaintext":      fmt.Sprintf("%t", config.Gateway.OctopusGrpcTlsPlaintext),
	}

	argoCDConfig := map[string]string{
		"ArgoCDUrl":       config.Gateway.ArgoCdServerUrl,
		"ArgoCDInsecure":  fmt.Sprintf("%t", config.Gateway.ArgoCdServerTlsInsecure),
		"ArgoCDPlaintext": fmt.Sprintf("%t", config.Gateway.ArgoCdServerTlsPlainText),
	}

	logger.Info(
		"Validating connection to Octopus and Argo CD using the following config:",
		slog.Any("octopusConfig", octopusConfig),
		slog.Any("argoCDConfig", argoCDConfig),
	)
}

// EnsureValidTlsConfiguration checks if the gRPC connection is valid by attempting to perform a TLS handshake
// this method will only return errors if the connection to Octopus fails, to allow services that don't rely on Argo CD at all to continue to function
func EnsureValidTlsConfiguration(ctx context.Context, log *slog.Logger, config *configuration.Configuration) error {
	logConnectionConfiguration(log, config)
	// Octopus
	octopusError := ensureValidOctopusTlsConfiguration(ctx, log, config)

	// Argo
	argoError := ensureValidArgoCdTlsConfiguration(ctx, log, config)

	if octopusError != nil {
		logger.LogMultiError(
			log,
			"Gateway failed to connect to Octopus, this could be due to the Octopus instance being unavailable or a TLS configuration issue, for help please see https://oc.to/argo-cd-gateway-troubleshooting",
			octopusError,
		)
	}

	if argoError != nil {
		logger.LogMultiError(
			log,
			"Gateway failed to connect to Argo CD, this could be due to the Argo CD instance being unavailable or a TLS configuration issue, for help please see https://oc.to/argo-cd-gateway-troubleshooting",
			argoError,
		)
	}

	if octopusError != nil && argoError != nil {
		return fmt.Errorf(
			"failed to connect to both Octopus %s and Argo CD %s",
			config.Gateway.OctopusGrpcUrl,
			config.Gateway.ArgoCdServerUrl,
		)
	} else if octopusError != nil {
		return fmt.Errorf("failed to connect to Octopus %s", config.Gateway.OctopusGrpcUrl)
	}

	return nil
}

func ensureValidOctopusTlsConfiguration(
	ctx context.Context,
	log *slog.Logger,
	config *configuration.Configuration,
) error {
	sanitizedOctopusGrpcUrl := SanitizeGrpcUrl(config.Gateway.OctopusGrpcUrl)
	octopusErrors := make([]error, 0)

	if !config.Gateway.OctopusGrpcTlsPlaintext {
		// IF certificate isn't pinned we should test both secure and insecure, only failing if both tests fail
		if !config.Gateway.OctopusGrpcPinCertificate {
			log.Info("Validating connection to Octopus using a secure TLS connection...")
			err := TestSecureTLS(ctx, sanitizedOctopusGrpcUrl, connectionValidationDialTimeout)
			if err != nil {
				secureErr := fmt.Errorf(
					"error validating connection to Octopus %s using a secure TLS connection: %w",
					sanitizedOctopusGrpcUrl,
					err,
				)
				log.Info(
					"Failed validating connection using a secure TLS connection, attempting connection to Octopus using a TLS connection without certificate verification...",
				)
				err := TestInsecureTLS(
					ctx,
					sanitizedOctopusGrpcUrl,
					connectionValidationDialTimeout,
					true,
					config.Gateway.OctopusGrpcThumbprint,
				)
				if err != nil {
					insecureErr := fmt.Errorf(
						"error validating connection to Octopus %s using a TLS connection without certificate verification: %w",
						sanitizedOctopusGrpcUrl,
						err,
					)
					octopusErrors = append(octopusErrors, secureErr)
					octopusErrors = append(octopusErrors, insecureErr)
				}
			}
		} else {
			// if the cert is pinned we only need to validate insecure
			log.Info("Validating connection to Octopus using a TLS connection without certificate verification...")
			err := TestInsecureTLS(
				ctx,
				sanitizedOctopusGrpcUrl,
				connectionValidationDialTimeout,
				true,
				config.Gateway.OctopusGrpcThumbprint,
			)
			if err != nil {
				insecureErr := fmt.Errorf(
					"error validating connection to Octopus %s using a TLS connection without certificate verification: %w",
					sanitizedOctopusGrpcUrl,
					err,
				)
				octopusErrors = append(octopusErrors, insecureErr)
			}
		}
	} else {
		log.Info("Validating connection to Octopus using a plain text connection...")
		err := TestPlaintextTLS(ctx, sanitizedOctopusGrpcUrl, connectionValidationDialTimeout)
		if err != nil {
			plainTextErr := fmt.Errorf(
				"error validating connection to Octopus %s using a connection without TLS: %w",
				sanitizedOctopusGrpcUrl,
				err,
			)
			octopusErrors = append(octopusErrors, plainTextErr)
		}
	}

	if len(octopusErrors) > 0 {
		return multierr.Combine(octopusErrors...)
	}

	return nil
}

func ensureValidArgoCdTlsConfiguration(
	ctx context.Context,
	log *slog.Logger,
	config *configuration.Configuration,
) error {
	sanitizedArgoCDUrl := SanitizeGrpcUrl(config.Gateway.ArgoCdServerUrl)
	argoCdErrors := make([]error, 0)

	grpcWebMode := config.Gateway.ArgoCdGrpcWeb || config.Gateway.ArgoCdGrpcWebRootPath != ""

	if !config.Gateway.ArgoCdServerTlsInsecure && !config.Gateway.ArgoCdServerTlsPlainText {
		var err error
		if grpcWebMode {
			log.Info("Validating connection to Argo CD using HTTPS (gRPC-Web mode)...")
			err = TestHTTPSTLS(ctx, sanitizedArgoCDUrl, connectionValidationDialTimeout, false)
		} else {
			log.Info("Validating connection to Argo CD using a secure TLS connection...")
			err = TestSecureTLS(ctx, sanitizedArgoCDUrl, connectionValidationDialTimeout)
		}
		if err != nil {
			secureErr := fmt.Errorf(
				"error validating connection to Argo CD %s using a secure TLS connection: %w",
				sanitizedArgoCDUrl,
				err,
			)
			argoCdErrors = append(argoCdErrors, secureErr)
		}
	} else if config.Gateway.ArgoCdServerTlsInsecure && !config.Gateway.ArgoCdServerTlsPlainText {
		var err error
		if grpcWebMode {
			log.Info("Validating connection to Argo CD using HTTPS without certificate verification (gRPC-Web mode)...")
			err = TestHTTPSTLS(ctx, sanitizedArgoCDUrl, connectionValidationDialTimeout, true)
		} else {
			log.Info("Validating connection to Argo CD using a TLS connection without certificate verification...")
			err = TestInsecureTLS(ctx, sanitizedArgoCDUrl, connectionValidationDialTimeout, false, "")
		}
		if err != nil {
			insecureErr := fmt.Errorf(
				"error validating connection to Argo CD %s using a TLS connection without certificate verification: %w",
				sanitizedArgoCDUrl,
				err,
			)
			argoCdErrors = append(argoCdErrors, insecureErr)
		}
	} else {
		log.Info("Validating connection to Argo CD using a plain text connection...")
		err := TestPlaintextTLS(ctx, sanitizedArgoCDUrl, connectionValidationDialTimeout)
		if err != nil {
			plainTextErr := fmt.Errorf(
				"error validating connection to Argo CD %s using a connection without TLS: %w",
				sanitizedArgoCDUrl,
				err,
			)
			argoCdErrors = append(argoCdErrors, plainTextErr)
		}
	}

	if len(argoCdErrors) > 0 {
		return multierr.Combine(argoCdErrors...)
	}

	return nil
}

func TestHTTPSTLS(parentCtx context.Context, address string, dialTime time.Duration, insecureSkipVerify bool) error {
	rootCerts, err := certpool.GetRootCertificatePool()
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{
		RootCAs:            rootCerts,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
	}
	ctx, cancel := context.WithTimeout(parentCtx, dialTime)
	defer cancel()

	conn, err := (&tls.Dialer{NetDialer: &net.Dialer{}, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}

func TestSecureTLS(parentCtx context.Context, address string, dialTime time.Duration) error {
	rootCerts, err := certpool.GetRootCertificatePool()
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		RootCAs: rootCerts,
	}

	creds := credentials.NewTLS(tlsConfig)

	// Secure connection
	ctx, cancel := context.WithTimeout(parentCtx, dialTime)
	defer cancel()
	conn, err := BlockingNewClient(ctx, "tcp", address, creds)
	if err != nil {
		return err
	}

	_ = conn.Close()
	return nil
}

func TestInsecureTLS(
	parentCtx context.Context,
	address string,
	dialTime time.Duration,
	isOctopusConnection bool,
	expectedThumbprint string,
) error {
	rootCerts, err := certpool.GetRootCertificatePool()
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{
		RootCAs:            rootCerts,
		InsecureSkipVerify: true,
	}

	// For Octopus insecure connection validation we check the server's thumbprint against the configuration of the gateway as a fallback if standard TLS verification fails
	// whereas for insecure Argo CD connections we don't validate the server cert at all
	if isOctopusConnection {
		// Slimmed down version of the certificate verification method used on the Octopus gRPC connection
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			serverCert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}

			result := certificate.VerifyServerCertificate(serverCert, expectedThumbprint)

			return result
		}
	}

	creds := credentials.NewTLS(tlsConfig)
	ctx, cancel := context.WithTimeout(parentCtx, dialTime)
	defer cancel()

	conn, err := BlockingNewClient(ctx, "tcp", address, creds)
	if err != nil {
		return err
	}

	_ = conn.Close()
	return nil
}

func TestPlaintextTLS(parentCtx context.Context, address string, dialTime time.Duration) error {
	ctx, cancel := context.WithTimeout(parentCtx, dialTime)

	defer cancel()
	conn, err := BlockingNewClient(ctx, "tcp", address, nil)
	if err != nil {
		return err
	}

	_ = conn.Close()
	return nil
}

// BlockingNewClient is a helper method to dial the given address, using optional TLS credentials,
// and blocking until the returned connection is ready. If the given credentials are nil, the
// connection will be insecure (plain-text).
// Based on: https://github.com/argoproj/argo-cd/blob/b7f60b7f76dd7d526d0b9a336f4a27d4c76a4459/util/grpc/grpc.go#L38
func BlockingNewClient(
	ctx context.Context,
	network, address string,
	creds credentials.TransportCredentials,
	opts ...grpc.DialOption,
) (*grpc.ClientConn, error) {
	proxyDialer := proxy.FromEnvironment()
	// Add explicit timeout to the dial operation, since proxyDialer.Dial's internal timeout is longer than our connection validation timeout
	type dialResult struct {
		conn net.Conn
		err  error
	}

	resultCh := make(chan dialResult, 1)
	go func() {
		conn, err := proxyDialer.Dial(network, address)
		resultCh <- dialResult{conn: conn, err: err}
	}()

	var rawConn net.Conn
	var err error

	select {
	case result := <-resultCh:
		rawConn = result.conn
		err = result.err
	case <-ctx.Done():
		return nil, fmt.Errorf(
			"timed out while dialing, this could be due to misconfigured TLS or the target host is unavailable: %w",
			ctx.Err(),
		)
	}

	if err != nil {
		return nil, err
	}

	if creds != nil {
		rawConn, _, err = creds.ClientHandshake(ctx, address, rawConn)
		if err != nil {
			return nil, err
		}
	}
	customDialer := func(_ context.Context, _ string) (net.Conn, error) {
		return rawConn, nil
	}

	opts = append(opts,
		grpc.WithContextDialer(customDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(connection.KeepAliveParams()),
	)

	conn, err := grpc.NewClient("passthrough:"+address, opts...)
	if err != nil {
		return nil, err
	}

	conn.Connect()
	if err := WaitForReady(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

func WaitForReady(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf(
				"timed out while waiting for the connection to be ready, this could be due to misconfigured TLS or the target host %s is unavailable: %w",
				conn.Target(),
				ctx.Err(),
			)
		}
	}
}

func monitorConnectionState(ctx context.Context, logger *slog.Logger, conn *grpc.ClientConn) {
	for {
		state := conn.GetState()
		logger.Info("Connection state change", slog.Any("target", conn.Target()), slog.Any("state", state))
		if !conn.WaitForStateChange(ctx, state) {
			return
		}
	}
}

// WaitForReadyConnections waits until connections become READY or the context ends.
// If argoCdGrpcConn is nil (gRPC-Web via local proxy), only the Octopus connection is checked.
func WaitForReadyConnections(
	ctx context.Context,
	logger *slog.Logger,
	octopusGrpcConn, argoCdGrpcConn *grpc.ClientConn,
) error {
	type result struct {
		ready bool
		err   error
	}

	octopusCh := make(chan result, 1)
	go func() {
		octopusGrpcConn.Connect()
		if err := WaitForReady(ctx, octopusGrpcConn); err != nil {
			_ = octopusGrpcConn.Close()
			octopusCh <- result{ready: false, err: err}
			return
		}
		octopusCh <- result{ready: true}
	}()
	go monitorConnectionState(ctx, logger, octopusGrpcConn)

	octopusResult := <-octopusCh

	if argoCdGrpcConn == nil {
		if !octopusResult.ready {
			return multierr.Combine(fmt.Errorf("failed to establish connection to Octopus"), octopusResult.err)
		}
		return nil
	}

	argoCh := make(chan result, 1)
	go func() {
		argoCdGrpcConn.Connect()
		if err := WaitForReady(ctx, argoCdGrpcConn); err != nil {
			_ = argoCdGrpcConn.Close()
			argoCh <- result{ready: false, err: err}
			return
		}
		argoCh <- result{ready: true}
	}()
	go monitorConnectionState(ctx, logger, argoCdGrpcConn)

	argoResult := <-argoCh

	if !octopusResult.ready || !argoResult.ready {
		return multierr.Combine(
			fmt.Errorf("failed to establish connections"),
			octopusResult.err,
			argoResult.err,
		)
	}

	return nil
}
