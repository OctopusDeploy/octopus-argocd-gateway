# Build
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /build

# Dependency installation
COPY go.mod go.sum ./
RUN go mod download

# Build the app from source
COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o gateway .

# Runtime image
FROM gcr.io/distroless/static:debug@sha256:63b4c0065ff7bd68753510400b2d5cedd5d5571d15eac4be0b6b0dd236a2d148

# Copy only the binary from the build stage to the final image
COPY --from=build /build/gateway /

# Use a non-root user from distroless for better security
USER 65532:65532

# Set the entry point for the container
ENTRYPOINT ["/gateway"]