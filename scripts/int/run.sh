#!/usr/bin/env bash
set -uo pipefail

usage() {
  cat << 'TEXT'
Usage: scripts/int/run.sh [options]

One integration run (MSL-INT): Mesail at one commit against given builds of TACHYON (ty),
FYLO (fylo) and DuVay, without changing Mesail's pins. With no options it runs the pins.
It makes its own detached worktree of the Mesail commit, re-vendors DuVay there,
runs the steps, writes summary.md and removes the worktree.

Builds (each binary is checked by its SHA-256 before use; give a file and its hash together):
  --ty <file> --ty-sha256 <hex>                   macOS arm64 ty for the native gates and bundles
  --ty-linux <file> --ty-linux-sha256 <hex>       linux arm64 ty for the Floci images
  --fylo <file> --fylo-sha256 <hex>               macOS arm64 fylo for the native gates
  --fylo-linux <file> --fylo-linux-sha256 <hex>   linux arm64 fylo for the Floci images
  --tachyon-commit <sha>   the TACHYON commit the ty files were built from (for the summary)
  --fylo-commit <sha>      the FYLO commit the fylo files were built from (for the summary)
  --duvay <sha>            DuVay commit (40 characters) to vendor instead of the pin
Run:
  --mesail <ref>           Mesail commit or branch (default origin/develop, after a fetch)
  --steps <list>           comma list of native, a11y, menus, floci (default: all four)
  --native-apps <list>     native step runs only these apps (gates.sh --apps-only --app <app> each),
                           for a short smoke run, e.g. the apps that use FYLO
  --floci-tests <regex>    floci step runs only the blackbox tests matching it (MESAIL_TEST_RUN)
  --keep-worktree          keep the worktree (for debugging)
  --keep-stack             leave the Floci stack running and keep its :int-<run> images, for the next
                           tester on the dev host (who stops the stack); remove the images later with
                           eval "$(mesail-devhost env)"; docker rmi $(docker images -q --filter reference='*:int-<run>')

A released binary is passed like any other build: download the release asset, take its hash
from the release's checksum file, and give both (for example --fylo-linux <file> --fylo-linux-sha256 <hex>).

Steps:
  native  scripts/gates.sh --all (mac-heavy --needs-screen)
  a11y    tests/responsive for web and web-admin, MESAIL_COLOR_SCHEME light and dark
  menus   the w-menu tests (MESAIL_MENU_TESTS=1) for web and web-admin, light and dark
  floci   scripts/int/floci.sh on the running Mesail dev host (ask mesail-devhost for the slot first):
          build images tagged :int-<run>, up, deploy, seed, blackbox tests, then only
          `docker compose stop` (never down.sh: no container, network or volume is removed);
          the :int-<run> images are removed after

Logs and summary: ~/Library/Logs/local/MESAIL/int/<run>/ (summary.md, steps.tsv, one log per step)
Worktree: ~/Developer/AnvilTargets/work/MESAIL/<run> from ~/Developer/AnvilTargets/git/MESAIL
Exit code: 0 when every step passed.
TEXT
  exit 2
}

shared_clone() {
  echo "${MESAIL_INT_CLONE:-$HOME/Developer/AnvilTargets/git/MESAIL}"
}

worktree_base() {
  echo "${MESAIL_INT_WORK:-$HOME/Developer/AnvilTargets/work/MESAIL}"
}

log_base() {
  echo "${MESAIL_INT_LOGS:-$HOME/Library/Logs/local/MESAIL/int}"
}

all_steps() {
  echo "native,a11y,menus,floci"
}

client_apps() {
  echo "web web-admin"
}

color_schemes() {
  echo "light dark"
}

fail() {
  echo "int: $1" >&2
  exit 1
}

sha256_of() {
  shasum -a 256 "$1" | awk '{ print $1 }'
}

option_value() {
  [ "$#" -ge 2 ] || fail "$1 needs a value"
  echo "$2"
}

parse_arguments() {
  RUN_STEPS="$(all_steps)"
  MESAIL_REF=origin/develop
  KEEP_WORKTREE=0
  KEEP_STACK=0
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --ty) TY_MAC="$(option_value "$@")"; shift 2 ;;
      --ty-sha256) TY_MAC_SHA256="$(option_value "$@")"; shift 2 ;;
      --ty-linux) TY_LINUX="$(option_value "$@")"; shift 2 ;;
      --ty-linux-sha256) TY_LINUX_SHA256="$(option_value "$@")"; shift 2 ;;
      --fylo) FYLO_MAC="$(option_value "$@")"; shift 2 ;;
      --fylo-sha256) FYLO_MAC_SHA256="$(option_value "$@")"; shift 2 ;;
      --fylo-linux) FYLO_LINUX="$(option_value "$@")"; shift 2 ;;
      --fylo-linux-sha256) FYLO_LINUX_SHA256="$(option_value "$@")"; shift 2 ;;
      --tachyon-commit) TACHYON_COMMIT="$(option_value "$@")"; shift 2 ;;
      --fylo-commit) FYLO_COMMIT="$(option_value "$@")"; shift 2 ;;
      --duvay) DUVAY_COMMIT="$(option_value "$@")"; shift 2 ;;
      --mesail) MESAIL_REF="$(option_value "$@")"; shift 2 ;;
      --steps) RUN_STEPS="$(option_value "$@")"; shift 2 ;;
      --native-apps) NATIVE_APPS="$(option_value "$@")"; shift 2 ;;
      --floci-tests) FLOCI_TESTS="$(option_value "$@")"; shift 2 ;;
      --keep-worktree) KEEP_WORKTREE=1; shift ;;
      --keep-stack) KEEP_STACK=1; shift ;;
      -h | --help) usage ;;
      *) fail "unknown option $1 (see --help)" ;;
    esac
  done
}

paired() {
  local name="$1" file="$2" hash="$3"
  [ -z "$file$hash" ] && return 0
  [ -n "$file" ] && [ -n "$hash" ] || fail "--$name and --$name-sha256 go together"
  [[ "$hash" =~ ^[0-9a-f]{64}$ ]] || fail "--$name-sha256 must be 64 lowercase hex characters"
  [ -f "$file" ] || fail "--$name: no file $file"
}

check_arguments() {
  paired ty "${TY_MAC:-}" "${TY_MAC_SHA256:-}"
  paired ty-linux "${TY_LINUX:-}" "${TY_LINUX_SHA256:-}"
  paired fylo "${FYLO_MAC:-}" "${FYLO_MAC_SHA256:-}"
  paired fylo-linux "${FYLO_LINUX:-}" "${FYLO_LINUX_SHA256:-}"
  [ -z "${DUVAY_COMMIT:-}" ] || [[ "$DUVAY_COMMIT" =~ ^[0-9a-f]{40}$ ]] || fail "--duvay must be a full 40-character commit SHA"
  [ -z "${NATIVE_APPS:-}" ] || [[ "$NATIVE_APPS" =~ ^[a-z0-9-]+(,[a-z0-9-]+)*$ ]] || fail "--native-apps takes a comma list of app names"
  [[ "$RUN_STEPS" =~ ^(native|a11y|menus|floci)(,(native|a11y|menus|floci))*$ ]] || fail "--steps takes native, a11y, menus, floci"
}

wanted_step() {
  case ",$RUN_STEPS," in
    *",$1,"*) return 0 ;;
  esac
  return 1
}

start_run() {
  RUN_ID="int-$(date +%Y%m%d-%H%M%S)"
  RUN_LOGS="$(log_base)/$RUN_ID"
  WORKTREE="$(worktree_base)/$RUN_ID"
  STEPS_FILE="$RUN_LOGS/steps.tsv"
  mkdir -p "$RUN_LOGS" || fail "cannot make $RUN_LOGS"
  : > "$STEPS_FILE"
  echo "$$" > "$RUN_LOGS/run.pid"
  echo "=== run $RUN_ID: logs in $RUN_LOGS"
}

machine_state() {
  echo "- free disk: internal $(df -h ~ | awk 'NR == 2 { print $4 }'), ANVIL $(df -h "$(worktree_base)" | awk 'NR == 2 { print $4 }'), HANGAR $(df -h /Volumes/HANGAR 2> /dev/null | awk 'NR == 2 { print $4 }')"
  echo "- memory: $(memory_pressure 2> /dev/null | grep -i 'free percentage' | sed 's/^ *//'), swap $(sysctl -n vm.swapusage | awk '{ print $6 " of " $3 }'), load $(sysctl -n vm.loadavg | awk '{ print $2 }')"
}

record_step() {
  local name="$1" code="$2" seconds="$3" log="$4" command="$5"
  printf '%s\t%s\t%s\t%s\t%s\n' "$name" "$code" "$seconds" "$log" "$command" >> "$STEPS_FILE"
}

run_step() {
  local name="$1" command="$2" log="$RUN_LOGS/$1.log" started code
  shift 2
  started="$(date +%s)"
  echo "=== $name $(date +%H:%M:%S) (log $log)"
  ("$@") > "$log" 2>&1
  code=$?
  echo "=== $name rc=$code"
  record_step "$name" "$code" "$(($(date +%s) - started))" "$log" "$command"
  return "$code"
}

verified_copy() {
  local file="$1" hash="$2" target="$3" found
  found="$(sha256_of "$file")"
  [ "$found" = "$hash" ] || { echo "checksum mismatch for $file: expected $hash, found $found"; return 1; }
  mkdir -p "$(dirname "$target")" && cp "$file" "$target" && chmod 755 "$target" && echo "$target: sha256 $found ok"
}

mac_version() {
  "$1" --version
}

verify_builds() {
  local tools="$RUN_LOGS/tools" status=0
  if [ -n "${TY_MAC:-}" ]; then verified_copy "$TY_MAC" "$TY_MAC_SHA256" "$tools/macos/ty" && mac_version "$tools/macos/ty" || status=1; fi
  if [ -n "${FYLO_MAC:-}" ]; then verified_copy "$FYLO_MAC" "$FYLO_MAC_SHA256" "$tools/macos/fylo" && mac_version "$tools/macos/fylo" || status=1; fi
  if [ -n "${TY_LINUX:-}" ]; then verified_copy "$TY_LINUX" "$TY_LINUX_SHA256" "$tools/linux/ty" || status=1; fi
  if [ -n "${FYLO_LINUX:-}" ]; then verified_copy "$FYLO_LINUX" "$FYLO_LINUX_SHA256" "$tools/linux/fylo" || status=1; fi
  return "$status"
}

run_alive() {
  local pid
  pid="$(cat "$(log_base)/$1/run.pid" 2> /dev/null)"
  [ -n "$pid" ] && kill -0 "$pid" 2> /dev/null
}

remove_stale_worktree() {
  local folder="$1" name
  name="$(basename "$folder")"
  run_alive "$name" && return 0
  git -C "$(shared_clone)" worktree remove --force "$folder" && echo "removed the worktree of the ended run $name"
}

remove_stale_worktrees() {
  local folder
  for folder in "$(worktree_base)"/int-20*; do
    [ -d "$folder" ] || continue
    remove_stale_worktree "$folder" || return 1
  done
}

make_worktree() {
  remove_stale_worktrees || return 1
  local clone commit
  clone="$(shared_clone)"
  git -C "$clone" fetch --prune origin || return 1
  commit="$(git -C "$clone" rev-parse --verify "$MESAIL_REF^{commit}")" || return 1
  git -C "$clone" worktree add --detach "$WORKTREE" "$commit" || return 1
  echo "worktree $WORKTREE at $commit"
}

manifest_file() {
  echo "$WORKTREE/apps/$1/client/manifest.json"
}

set_manifest_hashes() {
  local app="$1" folder script_sha style_sha manifest
  folder="$WORKTREE/apps/$app/client/shared/vendor/duvay"
  script_sha="$(sha256_of "$folder/duvay-mesail.js")"
  style_sha="$(sha256_of "$folder/duvay-mesail.css")"
  manifest="$(manifest_file "$app")"
  jq --arg script "$script_sha" --arg style "$style_sha" \
    '.tac.scriptSources["/shared/vendor/duvay/duvay-mesail.js"].sha256 = $script
     | .tac.styleSources["/shared/vendor/duvay/duvay-mesail.css"].sha256 = $style' "$manifest" > "$manifest.new" &&
    mv "$manifest.new" "$manifest" && echo "apps/$app manifest: script $script_sha, style $style_sha"
}

vendor_duvay() {
  local app
  export MESAIL_DUVAY_COMMIT="$DUVAY_COMMIT"
  for app in $(client_apps); do
    "$WORKTREE/scripts/vendor/duvay.sh" --app "$app" || return 1
    set_manifest_hashes "$app" || return 1
  done
}

use_builds() {
  export MESAIL_SLICE="$RUN_ID" MAC_HEAVY_COORDINATOR=caduceus-build
  if [ -n "${TY_MAC:-}" ]; then export MESAIL_TY="$RUN_LOGS/tools/macos/ty" MESAIL_TY_SHA256="$TY_MAC_SHA256"; fi
  if [ -n "${FYLO_MAC:-}" ]; then export MESAIL_FYLO="$RUN_LOGS/tools/macos/fylo" MESAIL_FYLO_SHA256="$FYLO_MAC_SHA256"; fi
  if [ -n "${DUVAY_COMMIT:-}" ]; then export MESAIL_DUVAY_COMMIT="$DUVAY_COMMIT"; fi
  use_linux_builds
}

use_linux_builds() {
  [ -n "${TY_LINUX:-}${FYLO_LINUX:-}" ] || return 0
  export MESAIL_INT_TOOLS="$RUN_LOGS/tools/linux"
  export MESAIL_INT_TY_SHA256="${TY_LINUX_SHA256:-}" MESAIL_INT_FYLO_SHA256="${FYLO_LINUX_SHA256:-}"
}

heavy() {
  "$HOME/.local/bin/mac-heavy" --owner msl-int "$@"
}

chosen_ty() {
  echo "${MESAIL_TY:-$HOME/Developer/Tools/tachyon-$(awk '$1 == "ty" { print $2; exit }' "$WORKTREE/scripts/ci/tools.lock")/ty}"
}

native_gates() {
  cd "$WORKTREE" && heavy --needs-screen -- "$WORKTREE/scripts/gates.sh" --all
}

native_app_gates() {
  cd "$WORKTREE" && heavy --needs-screen -- "$WORKTREE/scripts/gates.sh" --apps-only --app "$1"
}

native_steps() {
  local app status=0
  if [ -z "${NATIVE_APPS:-}" ]; then run_step native "scripts/gates.sh --all" native_gates; return; fi
  for app in $(echo "$NATIVE_APPS" | tr ',' ' '); do
    run_step "native-$app" "scripts/gates.sh --apps-only --app $app" native_app_gates "$app" || status=1
  done
  return "$status"
}

bundle_app() {
  cd "$WORKTREE" && heavy -- "$(chosen_ty)" bundle "$WORKTREE/apps/$1" --target web
}

responsive_suite() {
  local app="$1" scheme="$2" menus="$3" filter="$4"
  cd "$WORKTREE/tests/responsive" &&
    MESAIL_APP="$app" MESAIL_DIST="$WORKTREE/apps/$app/dist/web" MESAIL_COLOR_SCHEME="$scheme" MESAIL_MENU_TESTS="$menus" \
      heavy --light --needs-screen -- go test -tags responsive -count=1 -v -run "$filter" ./...
}

suite_command() {
  echo "cd tests/responsive && MESAIL_APP=$1 MESAIL_DIST=apps/$1/dist/web MESAIL_COLOR_SCHEME=$2 MESAIL_MENU_TESTS=$3 go test -tags responsive -count=1 -v -run '$4' ./..."
}

browser_steps() {
  local app scheme status=0
  for app in $(client_apps); do
    run_step "bundle-$app" "ty bundle apps/$app --target web" bundle_app "$app" || { status=1; continue; }
    for scheme in $(color_schemes); do
      browser_step a11y "$app" "$scheme" 0 '.' || status=1
      browser_step menus "$app" "$scheme" 1 'TestClosedMenus|TestMenuKeyboard' || status=1
    done
  done
  return "$status"
}

browser_step() {
  local kind="$1" app="$2" scheme="$3" menus="$4" filter="$5"
  wanted_step "$kind" || return 0
  run_step "$kind-$app-$scheme" "$(suite_command "$app" "$scheme" "$menus" "$filter")" responsive_suite "$app" "$scheme" "$menus" "$filter"
}

image_tag() {
  echo "$RUN_ID" | tr -cd 'a-z0-9-'
}

floci_settings() {
  MESAIL_IMAGE_TAG="$(image_tag)"
  export MESAIL_INT_KEEP_STACK="$KEEP_STACK"
  export MESAIL_OWNER=msl-int MESAIL_LOG_DIR="$RUN_LOGS/floci" MESAIL_IMAGE_TAG MESAIL_NO_PRUNE=1
  if [ -n "${FLOCI_TESTS:-}" ]; then export MESAIL_TEST_RUN="$FLOCI_TESTS"; fi
  mkdir -p "$MESAIL_LOG_DIR"
}

floci_stack() {
  floci_settings
  cd "$WORKTREE" && heavy -- "$WORKTREE/scripts/floci/run-all.sh" --bundle-only || return 1
  TACHYON_LOCK_ROOT="$HOME/Library/Caches/mac-locks/mesail-docker" MAC_HEAVY_COORDINATOR=mesail-docker \
    heavy --weight 1 -- "$WORKTREE/scripts/int/floci.sh"
}

own_images() {
  docker images --format '{{.Repository}}:{{.Tag}}' | grep -E ":$(image_tag)\$"
}

remove_own_images() {
  local image
  eval "$("$HOME/.local/bin/mesail-devhost" env)" || return 1
  for image in $(own_images); do
    docker rmi "$image" || return 1
  done
  echo "removed the :$(image_tag) images"
}

floci_steps() {
  wanted_step floci || return 0
  run_step floci "MESAIL_IMAGE_TAG=$(image_tag) MESAIL_TEST_RUN='${FLOCI_TESTS:-}' scripts/int/floci.sh (after run-all.sh --bundle-only)" floci_stack
  local code=$?
  if [ "$KEEP_STACK" = 1 ] && [ "$code" = 0 ]; then echo "=== the stack and its :$(image_tag) images are kept (--keep-stack)"; return "$code"; fi
  run_step floci-images "docker rmi <every :$(image_tag) image>" remove_own_images
  return "$code"
}

run_steps() {
  local status=0
  if wanted_step native; then native_steps || status=1; fi
  if wanted_step a11y || wanted_step menus; then browser_steps || status=1; fi
  floci_steps || status=1
  return "$status"
}

fylo_slice_folder() {
  echo "$HOME/Developer/AtlasFylo/mesail/slice-$RUN_ID"
}

clean_up() {
  if [ -d "$(fylo_slice_folder)" ]; then rm -rf "$(fylo_slice_folder)" && echo "removed $(fylo_slice_folder)"; fi
  rm -rf "$RUN_LOGS/tools"
  if [ "$KEEP_WORKTREE" = 1 ] || [ ! -d "$WORKTREE" ]; then return 0; fi
  git -C "$(shared_clone)" worktree remove --force "$WORKTREE" && echo "removed $WORKTREE"
}

pinned_duvay() {
  sed -n 's/^pinned_commit() { echo "\([0-9a-f]*\)"; }$/\1/p' "$WORKTREE/scripts/vendor/duvay.sh"
}

pinned_version() {
  awk -v tool="$1" '$1 == tool { print $2; exit }' "$WORKTREE/scripts/ci/tools.lock"
}

build_line() {
  local name="$1" commit="$2" mac="$3" linux="$4"
  [ -n "$mac$linux" ] || { echo "- $name: the pin, release $(pinned_version "$5")"; return; }
  echo "- $name: commit ${commit:-not given}; macOS ${mac:-pin}; linux ${linux:-pin}"
}

shas_section() {
  echo "## Commits"
  echo "- Mesail: $(git -C "$WORKTREE" rev-parse HEAD 2> /dev/null) ($MESAIL_REF)"
  echo "- DuVay: ${DUVAY_COMMIT:-$(pinned_duvay) (the pin)}"
  build_line TACHYON "${TACHYON_COMMIT:-}" "${TY_MAC_SHA256:+ty sha256 $TY_MAC_SHA256}" "${TY_LINUX_SHA256:+ty sha256 $TY_LINUX_SHA256}" ty
  build_line FYLO "${FYLO_COMMIT:-}" "${FYLO_MAC_SHA256:+fylo sha256 $FYLO_MAC_SHA256}" "${FYLO_LINUX_SHA256:+fylo sha256 $FYLO_LINUX_SHA256}" fylo
}

steps_table() {
  echo "## Steps"
  echo "| step | result | minutes | log |"
  echo "| --- | --- | --- | --- |"
  awk -F '\t' '{ printf "| %s | %s | %.1f | %s |\n", $1, ($2 == 0 ? "pass" : "FAIL rc=" $2), $3 / 60, $4 }' "$STEPS_FILE"
}

failure_lines() {
  grep -E -- '^ *--- FAIL|^FAIL|_test\.go:[0-9]+:|rc=[1-9][0-9]*$|timed out|checksum mismatch|did not' "$1" | head -60
}

failure_section() {
  local name code log command
  echo "## Failures"
  while IFS="$(printf '\t')" read -r name code _ log command; do
    [ "$code" = 0 ] && continue
    echo "### $name (rc=$code)"
    echo "Reproduce in a worktree of the Mesail commit above, with the same builds: \`$command\`"
    echo '```'
    failure_lines "$log"
    echo '```'
  done < "$STEPS_FILE"
}

flows_file() {
  echo "$(cd "$(dirname "$0")" && pwd)/flows.tsv"
}

blackbox_log() {
  echo "$RUN_LOGS/floci/blackbox.log"
}

count_results() {
  sed -n "s/^ *--- $1: \\([A-Za-z0-9_]*\\).*/\\1/p" "$(blackbox_log)" | grep -cE "$2"
}

flow_result() {
  local pattern="$1" passed failed skipped
  [ -n "$pattern" ] || { echo "not covered (no blackbox test yet)"; return; }
  [ -f "$(blackbox_log)" ] || { echo "not run"; return; }
  passed="$(count_results PASS "$pattern")"
  failed="$(count_results FAIL "$pattern")"
  skipped="$(count_results SKIP "$pattern")"
  [ "$failed" = 0 ] || { echo "FAIL ($failed failed, $passed passed)"; return; }
  [ "$passed" != 0 ] || [ "$skipped" = 0 ] || { echo "skipped ($skipped tests; see the blackbox log for why)"; return; }
  [ "$passed" != 0 ] || { echo "not run (no test matched $pattern)"; return; }
  [ "$skipped" = 0 ] || { echo "pass ($passed tests, $skipped skipped)"; return; }
  echo "pass ($passed tests)"
}

coverage_section() {
  local flow pattern
  echo "## Flows on the dev host (Floci blackbox)"
  wanted_step floci || { echo "The floci step was not asked for in this run; no flow was tested."; return 0; }
  echo "| flow | result | tests |"
  echo "| --- | --- | --- |"
  while IFS="$(printf '\t')" read -r flow pattern; do
    echo "| $flow | $(flow_result "$pattern") | \`${pattern:--}\` |"
  done < "$(flows_file)"
}

run_result() {
  if awk -F '\t' '$2 != 0 { found = 1 } END { exit !found }' "$STEPS_FILE"; then echo RED; return; fi
  echo GREEN
}

write_summary() {
  local summary="$RUN_LOGS/summary.md"
  {
    echo "# MSL-INT $RUN_ID: $(run_result)"
    echo "Started $STARTED_AT, ended $(date '+%Y-%m-%d %H:%M:%S %Z'). Steps asked: $RUN_STEPS."
    shas_section
    echo "## Machine at start"
    echo "$MACHINE_AT_START"
    steps_table
    coverage_section
    failure_section
  } > "$summary"
  echo "=== summary: $summary"
  cat "$summary"
}

stop_run() {
  trap - INT TERM
  record_step stopped 130 0 "$RUN_LOGS/steps.tsv" "the run was stopped (INT or TERM)"
  write_summary
  clean_up
  exit 130
}

main() {
  parse_arguments "$@"
  check_arguments
  start_run
  trap stop_run INT TERM
  STARTED_AT="$(date '+%Y-%m-%d %H:%M:%S %Z')"
  MACHINE_AT_START="$(machine_state)"
  run_step builds "verify every given binary by its SHA-256" verify_builds &&
    run_step worktree "git worktree add --detach <Mesail commit>" make_worktree &&
    { [ -z "${DUVAY_COMMIT:-}" ] || run_step duvay "MESAIL_DUVAY_COMMIT=$DUVAY_COMMIT scripts/vendor/duvay.sh --app <app>" vendor_duvay; } &&
    use_builds &&
    run_steps
  write_summary
  clean_up
  [ "$(run_result)" = GREEN ]
}

main "$@"
