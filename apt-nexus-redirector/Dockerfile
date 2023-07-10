ARG BUILD_IMAGE=golang:latest
ARG BASE_IMAGE=gcr.io/distroless/static-debian11:nonroot

FROM ${BUILD_IMAGE} as build
SHELL [ "/bin/sh", "-ec" ]

ARG GOPROXY
ARG GOSUMDB
ARG GOPRIVATE

WORKDIR /go/src
COPY /* ./

ENV CGO_ENABLED=0
RUN go env | grep -F -e GOPROXY -e GOSUMDB -e GOPRIVATE ; \
    go build -o ./apt-nexus-redirector ; \
    strip --strip-debug ./apt-nexus-redirector ; \
    # cleanup intermediate layer
    eval "$(go env | grep -F -e GOCACHE -e GOMODCACHE)" ; \
    rm -rf ${GOCACHE} ${GOMODCACHE}

# ---

FROM ${BASE_IMAGE}

COPY --from=build /go/src/apt-nexus-redirector /bin/

USER nonroot:nonroot

ENTRYPOINT [ "/bin/apt-nexus-redirector" ]
