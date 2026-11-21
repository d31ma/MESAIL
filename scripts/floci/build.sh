#!/usr/bin/env bash
set -euo pipefail
# shellcheck source-path=SCRIPTDIR
. "$(dirname "$0")/common.sh"

build_image() {
  local target="$1" tag="$2"
  shift 2
  BUILDX_GIT_INFO=false docker --context default buildx build --builder default --load --progress plain \
    --platform "linux/$(mesail_stack .architecture)" \
    --provenance=false --label org.mesail.stack=dev ${target:+--target "$target"} --tag "$tag" "$@" "$(mesail_root)"
}

golang_image() {
  sed -n 's/^ARG GOLANG_IMAGE=//p' "$(mesail_root)/Dockerfile"
}

build_relay_sink() {
  build_image "" "$(mesail_stack .relaySink.image):$MESAIL_IMAGE_TAG" \
    --file "$(mesail_root)/tests/relaysink/Dockerfile" --build-arg "GOLANG_IMAGE=$(golang_image)"
}

trace_argument() {
  echo "MESAIL_FYLO_TRACE=$MESAIL_FYLO_TRACE"
}

build_container() {
  local service="$1"
  build_image "$service" "$(mesail_service_field "$service" image):$MESAIL_IMAGE_TAG" --build-arg "$(trace_argument)"
}

build_service() {
  local service="$1"
  if mesail_is_container "$service"; then build_container "$service"; return; fi
  build_image lambda "$(mesail_stack .imagePrefix)$service:$MESAIL_IMAGE_TAG" \
    --build-arg "APP=$service" --build-arg "FYLO=$(mesail_service_field "$service" fylo)" \
    --build-arg "DEV_DNS_RESOLVER=http://router.localhost:18080/_dev/dns-query" --build-arg "$(trace_argument)"
}

check_trace_wrapper() {
  local image
  image="$(mesail_stack .imagePrefix)directory:$MESAIL_IMAGE_TAG"
  docker run --rm --network none --user 10001:10001 --entrypoint /bin/bash "$image" \
    -c "$(cat "$(mesail_root)/deploy/lambda/fylo-trace_test.sh")" fylo-trace_test
}

trace_check() {
  if [ "$MESAIL_FYLO_TRACE" != 1 ]; then return 0; fi
  mesail_step trace-wrapper-check check_trace_wrapper
}

chosen_services() {
  if [ "$#" -gt 0 ]; then printf '%s\n' "$@"; return; fi
  mesail_services
}

main() {
  local service
  mesail_load_settings
  echo "fylo trace wrapper and start snapshot: MESAIL_FYLO_TRACE=$MESAIL_FYLO_TRACE (1 = on, test images only)"
  mesail_step build-router build_image router "mesail-router:$MESAIL_IMAGE_TAG"
  mesail_step build-scheduler build_image scheduler "mesail-scheduler:$MESAIL_IMAGE_TAG"
  mesail_step build-keycloak build_image mesail-keycloak "$(mesail_stack .keycloak.image):$MESAIL_IMAGE_TAG"
  mesail_step build-relay-sink build_relay_sink || echo "warning: the relay sink did not build; FlowMailOutRelay_test.go skips"
  for service in $(chosen_services "$@"); do
    mesail_step "build-$service" build_service "$service" || echo "warning: $service did not build; it is not deployed"
  done
  trace_check || echo "warning: the fylo trace wrapper check failed; the trace may be wrong"
  mesail_step prune-images docker image prune --force --filter label=org.mesail.stack=dev
}

main "$@"
