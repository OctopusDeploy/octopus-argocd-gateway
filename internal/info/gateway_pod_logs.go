package info

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/OctopusDeploy/octopus-argocd-gateway/internal/kubernetes"
)

const maxLogSizeBytes = 10 * 1024 * 1024

type LogLine struct {
	Timestamp time.Time
	Message   string
}

func getGatewayPodLogs(
	ctx context.Context,
	logger *slog.Logger,
	podName string,
	podNamespace string,
	showPreviousContainer bool,
) ([]LogLine, error) {
	clientSet, err := kubernetes.NewKubernetesClient()
	if err != nil {
		return nil, err
	}
	// Assume a minimum of 32 bytes per line (this accounts for just timestamps)
	tailLines := int64(maxLogSizeBytes / 32)
	req := clientSet.CoreV1().Pods(podNamespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:  "octopus-argocd-gateway",
		Previous:   showPreviousContainer,
		TailLines:  &tailLines,
		Timestamps: true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}

	defer func(stream io.ReadCloser) {
		deferErr := stream.Close()
		if deferErr != nil {
			logger.Error("Failed to close podLogs", slog.Any("error", deferErr))
		}
	}(stream)

	return parseLogs(stream)
}

func parseLogs(stream io.Reader) ([]LogLine, error) {
	cb := NewCircularBuffer(maxLogSizeBytes)
	readBuf := make([]byte, 1024) // 1KB buffer
	// Read the stream into the ring buffer
	for {
		n, err := stream.Read(readBuf)
		if n > 0 {
			cb.Write(readBuf[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	finalBytes := cb.Bytes()

	lines := bytes.Split(finalBytes, []byte("\n"))

	logLines := make([]LogLine, 0)

	for i, l := range lines {
		// Skip empty lines
		if len(l) == 0 {
			continue
		}
		line := string(l)
		ts, message, err := splitLogLine(line)
		// First line may be malformed, so we ignore it
		if i == 0 && err != nil {
			continue
		} else if err != nil {
			return logLines, err
		}
		logLines = append(logLines, LogLine{
			Timestamp: ts,
			Message:   message,
		})
	}
	return logLines, nil
}

func splitLogLine(line string) (time.Time, string, error) {
	split := strings.SplitN(line, " ", 2)
	if len(split) != 2 {
		return time.Time{}, line, errors.New("malformed log line")
	}
	ts, err := time.Parse(time.RFC3339Nano, split[0])
	return ts, split[1], err
}
