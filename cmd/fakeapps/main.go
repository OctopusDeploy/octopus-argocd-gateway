package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/certificate"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/logger"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopus"
	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/octopusdeploy"
	pb "github.com/OctopusDeploy/octopus-argocd-gateway/internal/protos"
)

const (
	// applicationEventInterval is how often the application events are re-sent
	// after the initial POST.
	applicationEventInterval = 5 * time.Minute
	// applicationEventDuration is how long the application events keep being
	// re-sent for.
	applicationEventDuration = time.Hour
	// defaultPort is the port the web server listens on when PORT is not set.
	defaultPort = "8085"
	// shutdownTimeout is how long in-flight requests get to finish when the
	// web server is stopped.
	shutdownTimeout = 30 * time.Second
)

// CreateApplicationsRequest is the JSON body for POST /api/applications.
// It contains all configuration previously passed as CLI flags plus the
// list of fake applications to register.
type CreateApplicationsRequest struct {
	ServerApiUrl      string                  `json:"server_api_url"`
	ServerAccessToken string                  `json:"server_access_token"`
	SpaceId           string                  `json:"space_id"`
	GatewayName       string                  `json:"gateway_name"`
	ArgoCDWebUIUrl    string                  `json:"argocd_web_ui_url"`
	Environments      []string                `json:"environments"`
	Tenants           []string                `json:"tenants"`
	GrpcUrl           string                  `json:"grpc_url"`
	Plaintext         bool                    `json:"plaintext"`
	PinCertificate    bool                    `json:"pin_certificate"`
	CertificatePath   string                  `json:"certificate_path"`
	Applications      []ApplicationDefinition `json:"applications"`
}

// ApplicationDefinition represents a single fake ArgoCD application.
type ApplicationDefinition struct {
	Name                 string `json:"name"`
	Project              string `json:"project"`
	Namespace            string `json:"namespace"`
	DestinationServer    string `json:"destination_server"`
	DestinationNamespace string `json:"destination_namespace"`
	RepoURL              string `json:"repo_url"`
	Path                 string `json:"path"`
	SyncStatus           string `json:"sync_status"`
	HealthStatus         string `json:"health_status"`
	OctopusProject       string `json:"octopus_project"`
	OctopusEnvironment   string `json:"octopus_environment"`
}

// ApplicationResult reports the outcome of sending a single application event.
type ApplicationResult struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

// CreateApplicationsResponse is returned from POST /api/applications.
type CreateApplicationsResponse struct {
	GatewayId string              `json:"gateway_id"`
	ClientId  string              `json:"client_id"`
	Results   []ApplicationResult `json:"results"`
}

func main() {
	debug := os.Getenv("DEBUG") == "true"
	log := logger.New(debug)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	log.Info("Fake apps server starting", slog.Bool("debug", debug), slog.String("port", port))

	// ctx is cancelled when the web server is stopped, which shuts down any
	// application event loops still running in the background.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var eventLoops sync.WaitGroup

	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api")
	api.POST("/applications", handleCreateApplications(ctx, &eventLoops, log))
	api.GET("/health", handleHealth())

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		log.Info("Shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			log.Error("Web server failed", slog.Any("error", err))
		}
	}

	// Cancel the event loops, stop accepting requests, and wait for everything
	// to finish before the process exits.
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("Failed to shut down web server cleanly", slog.Any("error", err))
	}

	eventLoops.Wait()

	log.Info("Fake apps server stopped")
}

func handleHealth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// handleCreateApplications registers the gateway and sends an event for every
// application in the request. The events are then re-sent in the background
// until rootCtx is cancelled, and eventLoops tracks those background loops so
// the server can wait for them while shutting down.
func handleCreateApplications(rootCtx context.Context, eventLoops *sync.WaitGroup, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateApplicationsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Error("Failed to decode request body", slog.Any("error", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.ServerApiUrl == "" || req.ServerAccessToken == "" || req.SpaceId == "" || req.GatewayName == "" {
			log.Error("Missing required fields in request")
			c.JSON(http.StatusBadRequest, gin.H{"error": "server_api_url, server_access_token, space_id, and gateway_name are required"})
			return
		}
		if len(req.Applications) == 0 {
			log.Error("No applications provided in request")
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one application is required"})
			return
		}

		if req.CertificatePath != "" {
			if err := certificate.InitializeCertificatePool(req.CertificatePath, log); err != nil {
				log.Warn("Failed to initialize certificate pool", slog.Any("error", err))
			}
		}

		// If the token is an API key (starts with "API-"), exchange it for an access token
		if strings.HasPrefix(req.ServerAccessToken, "API-") {
			log.Info("ServerAccessToken is an API key, exchanging for access token...")
			accessToken, err := exchangeAPIKeyForAccessToken(log, req.ServerApiUrl, req.ServerAccessToken)
			if err != nil {
				log.Error("Failed to exchange API key for access token", slog.Any("error", err))
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to exchange API key for access token: %v", err)})
				return
			}
			req.ServerAccessToken = accessToken
			log.Info("Successfully exchanged API key for access token")
		}

		// Register gateway with Octopus
		log.Info("Registering gateway with Octopus Server...", slog.String("name", req.GatewayName))
		regResponse, err := registerGateway(&req)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				log.Info("Gateway already exists, skipping")
				c.JSON(http.StatusOK, gin.H{"message": "gateway already exists, skipping registration"})
				return
			}
			log.Error("Failed to register gateway", slog.Any("error", err))
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to register gateway: %v", err)})
			return
		}
		log.Info("Gateway registered",
			slog.String("id", regResponse.Resource.Id),
			slog.String("clientId", regResponse.Resource.ClientId),
		)

		// Determine gRPC URL: use explicit grpc_url if provided, otherwise derive from API URL
		grpcUrl := req.GrpcUrl
		if grpcUrl == "" {
			grpcUrl, err = deriveGrpcUrl(req.ServerApiUrl)
			if err != nil {
				log.Error("Failed to derive gRPC URL", slog.Any("error", err))
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to derive gRPC URL: %v", err)})
				return
			}
		}

		ctx := c.Request.Context()
		log.Info("Connecting to Octopus gRPC...",
			slog.String("url", grpcUrl),
			slog.String("clientId", regResponse.Resource.ClientId),
			slog.Bool("hasAuthToken", regResponse.AuthenticationToken != ""),
			slog.Bool("hasThumbprint", regResponse.CertificateThumbprint != ""),
			slog.Bool("plaintext", req.Plaintext),
			slog.Bool("pinCertificate", req.PinCertificate),
		)
		octopusClient, err := octopus.NewClient(rootCtx, log, octopus.Config{
			ServerURL:   grpcUrl,
			ClientID:    regResponse.Resource.ClientId,
			BearerToken: regResponse.AuthenticationToken,
			TLS: octopus.TLSConfig{
				Plaintext:      req.Plaintext,
				Thumbprint:     regResponse.CertificateThumbprint,
				PinCertificate: req.PinCertificate,
			},
		})
		if err != nil {
			log.Error("Failed to create gRPC client", slog.Any("error", err))
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to create gRPC client: %v", err)})
			return
		}
		octopusClient.Connect()

		// The background event loop started below outlives this request and
		// takes ownership of the client, so only close it here if we never get
		// that far.
		clientHandedOver := false
		defer func() {
			if !clientHandedOver {
				octopusClient.Close() //nolint:errcheck
			}
		}()

		readyCtx, readyCancel := context.WithTimeout(ctx, 30*time.Second)
		defer readyCancel()
		if err := octopusClient.WaitForReady(readyCtx); err != nil {
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": fmt.Sprintf("gRPC connection not ready: %v", err)})
			return
		}
		log.Info("gRPC connection established")

		// Send fake application events
		eventsClient := octopusClient.ApplicationEvents()
		results := sendApplicationEvents(ctx, log, eventsClient, req.Applications)

		// Keep re-sending the events in the background so the applications stay
		// visible in Octopus for a while after this request completes.
		clientHandedOver = startApplicationEventLoop(rootCtx, eventLoops, log, octopusClient, eventsClient, req.Applications)

		c.JSON(http.StatusOK, CreateApplicationsResponse{
			GatewayId: regResponse.Resource.Id,
			ClientId:  regResponse.Resource.ClientId,
			Results:   results,
		})
	}
}

// startApplicationEventLoop hands octopusClient to a background goroutine that
// re-sends the application events on a schedule and closes the client once it
// finishes. It reports whether the loop was started, which it is not when the
// server is already shutting down.
func startApplicationEventLoop(
	ctx context.Context,
	eventLoops *sync.WaitGroup,
	log *slog.Logger,
	octopusClient octopus.Client,
	eventsClient pb.ApplicationEventServiceClient,
	applications []ApplicationDefinition,
) bool {
	if ctx.Err() != nil {
		log.Info("Server is shutting down, not starting application event loop", slog.Any("reason", ctx.Err()))
		return false
	}

	eventLoops.Add(1)
	go func() {
		defer eventLoops.Done()
		defer octopusClient.Close() //nolint:errcheck
		repeatApplicationEvents(ctx, log, eventsClient, applications)
	}()

	return true
}

// sendApplicationEvents builds and sends an application event for each
// application definition, returning one result per application.
func sendApplicationEvents(
	ctx context.Context,
	log *slog.Logger,
	eventsClient pb.ApplicationEventServiceClient,
	applications []ApplicationDefinition,
) []ApplicationResult {
	results := make([]ApplicationResult, 0, len(applications))

	for _, appDef := range applications {
		result := ApplicationResult{Name: appDef.Name}

		event := buildApplicationEvent(&appDef)
		eventJSON, err := json.Marshal(event)
		if err != nil {
			result.Error = fmt.Sprintf("marshal error: %v", err)
			results = append(results, result)
			continue
		}

		grpcReq := &pb.ApplicationEventUpdateRequest{
			EventPayloadJson: string(eventJSON),
		}
		_, err = eventsClient.ApplicationEventUpdate(ctx, grpcReq)
		if err != nil {
			result.Error = fmt.Sprintf("send error: %v", err)
			log.Error("Failed to send application event", slog.String("app", appDef.Name), slog.Any("error", err))
		} else {
			log.Info("Sent application event", slog.String("app", appDef.Name))
		}

		results = append(results, result)
	}

	return results
}

// repeatApplicationEvents re-sends the application events every
// applicationEventInterval for up to applicationEventDuration, returning early
// when ctx is cancelled because the web server is shutting down.
func repeatApplicationEvents(
	ctx context.Context,
	log *slog.Logger,
	eventsClient pb.ApplicationEventServiceClient,
	applications []ApplicationDefinition,
) {
	ctx, cancel := context.WithTimeout(ctx, applicationEventDuration)
	defer cancel()

	ticker := time.NewTicker(applicationEventInterval)
	defer ticker.Stop()

	log.Info("Started application event loop",
		slog.Int("applications", len(applications)),
		slog.Duration("interval", applicationEventInterval),
		slog.Duration("duration", applicationEventDuration),
	)

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopped application event loop", slog.Any("reason", ctx.Err()))
			return
		case <-ticker.C:
			log.Info("Re-sending application events", slog.Int("applications", len(applications)))
			sendApplicationEvents(ctx, log, eventsClient, applications)
		}
	}
}

// exchangeAPIKeyForAccessToken calls the Octopus server to exchange an API key
// for an access token via /api/users/access-token.
func exchangeAPIKeyForAccessToken(log *slog.Logger, serverApiUrl, apiKey string) (string, error) {
	parsed, err := url.Parse(serverApiUrl)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}

	endpoint := fmt.Sprintf("%s://%s/api/users/access-token", parsed.Scheme, parsed.Host)
	log.Info("Exchanging API key for access token", slog.String("endpoint", endpoint))

	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Octopus-ApiKey", apiKey)

	httpClient := &http.Client{}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling access-token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("access-token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("access-token endpoint returned empty AccessToken")
	}

	return result.AccessToken, nil
}

func registerGateway(req *CreateApplicationsRequest) (*octopusdeploy.RegisterArgoCDGatewayResponse, error) {
	apiUrl, err := url.Parse(req.ServerApiUrl)
	if err != nil {
		return nil, err
	}

	client, err := octopusdeploy.NewOctopusApiClient(apiUrl, req.ServerAccessToken, req.SpaceId)
	if err != nil {
		return nil, err
	}

	clientId := uuid.New().String()
	return client.RegisterGateway(
		clientId,
		req.GatewayName,
		req.ArgoCDWebUIUrl,
		req.Environments,
		req.Tenants,
		false,
	)
}

func buildApplicationEvent(appDef *ApplicationDefinition) v1alpha1.ApplicationWatchEvent {
	namespace := appDef.Namespace
	if namespace == "" {
		namespace = "argocd"
	}
	project := appDef.Project
	if project == "" {
		project = "default"
	}
	syncStatus := v1alpha1.SyncStatusCode(appDef.SyncStatus)
	if syncStatus == "" {
		syncStatus = v1alpha1.SyncStatusCodeSynced
	}
	healthStatus := appDef.HealthStatus
	if healthStatus == "" {
		healthStatus = "Healthy"
	}
	destServer := appDef.DestinationServer
	if destServer == "" {
		destServer = "https://kubernetes.default.svc"
	}

	app := v1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        appDef.Name,
			Namespace:   namespace,
			UID:         k8stypes.UID(uuid.New().String()),
			Annotations: buildAnnotations(appDef),
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "octopus-argocd-gateway-fakeapps",
			},
		},
		Spec: v1alpha1.ApplicationSpec{
			Project: project,
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        appDef.RepoURL,
				Path:           appDef.Path,
				TargetRevision: "HEAD",
			},
			Destination: v1alpha1.ApplicationDestination{
				Server:    destServer,
				Namespace: appDef.DestinationNamespace,
			},
		},
		Status: v1alpha1.ApplicationStatus{
			SourceType: v1alpha1.ApplicationSourceTypeDirectory,
			Sync: v1alpha1.SyncStatus{
				Status: syncStatus,
			},
			Health: v1alpha1.HealthStatus{
				Status: health.HealthStatusCode(healthStatus),
			},
		},
	}

	return v1alpha1.ApplicationWatchEvent{
		Type:        watch.Modified,
		Application: app,
	}
}

func buildAnnotations(appDef *ApplicationDefinition) map[string]string {
	annotations := map[string]string{}
	if appDef.OctopusProject != "" {
		annotations["argo.octopus.com/project"] = appDef.OctopusProject
	}
	if appDef.OctopusEnvironment != "" {
		annotations["argo.octopus.com/environment"] = appDef.OctopusEnvironment
	}
	if len(annotations) == 0 {
		return nil
	}
	return annotations
}

func deriveGrpcUrl(apiURL string) (string, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	// Octopus Cloud serves gRPC on port 8443
	return host + ":8443", nil
}
