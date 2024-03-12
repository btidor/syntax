FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS build

RUN apk add git bash
COPY dockerfile/ /src
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 \
    go build -o /dockerfile-frontend -ldflags '-s -w' \
    ./cmd/dockerfile-frontend

FROM scratch AS release

LABEL moby.buildkit.frontend.network.none="true"
LABEL moby.buildkit.frontend.caps="moby.buildkit.frontend.inputs,moby.buildkit.frontend.subrequests,moby.buildkit.frontend.contexts"

COPY --from=build /dockerfile-frontend /bin/dockerfile-frontend
ENTRYPOINT ["/bin/dockerfile-frontend"]
