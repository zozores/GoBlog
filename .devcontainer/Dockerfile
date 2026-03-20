FROM mcr.microsoft.com/devcontainers/go:1.26-bookworm

# Install SQLite development headers and other dependencies
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends libsqlite3-dev nodejs npm

# Install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.64.5

# Install gopls (usually included in the devcontainer image, but let's make sure)
RUN go install golang.org/x/tools/gopls@latest
