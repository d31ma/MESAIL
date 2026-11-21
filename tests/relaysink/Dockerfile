# syntax=docker/dockerfile:1.7
# Test stack only (slice K34). scripts/floci/build.sh passes GOLANG_IMAGE from
# the repository Dockerfile, so the sink builds on the SMTP edge's pinned base.
ARG GOLANG_IMAGE

FROM ${GOLANG_IMAGE} AS build
WORKDIR /src/tests/relaysink
COPY kit /src/kit
COPY tests/relaysink/ ./
RUN find . -name '*_test.go' -delete \
 && GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/relay-sink .

FROM scratch
COPY --from=build /out/relay-sink /relay-sink
USER 10001:10001
EXPOSE 2525 8025
ENTRYPOINT ["/relay-sink"]
