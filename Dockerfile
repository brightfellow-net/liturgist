# syntax=docker/dockerfile:1
# Build: docker build -t liturgist .   Run: docker run -p 8080:8080 -v liturgist-data:/data liturgist

FROM --platform=$BUILDPLATFORM node:24-slim AS web
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages ./packages
COPY web ./web
RUN pnpm install --frozen-lockfile
RUN pnpm --filter web build

FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev COMMIT=unknown BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/brightfellow-net/liturgist/server.Version=$VERSION -X github.com/brightfellow-net/liturgist/server.Commit=$COMMIT -X github.com/brightfellow-net/liturgist/server.BuildDate=$BUILD_DATE" \
    -o /liturgist ./cmd/liturgist
# distroless has no shell or mkdir: prepare the data folder here
RUN mkdir -p /data-skel

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /liturgist /liturgist
COPY --from=build --chown=65532:65532 /data-skel /data
ENV LITURGIST_DATA_DIR=/data LITURGIST_LISTEN=:8080
# Set LITURGIST_BASE_URL to the address people type in the browser.
VOLUME /data
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/liturgist", "healthcheck"]
ENTRYPOINT ["/liturgist"]
CMD ["serve"]
