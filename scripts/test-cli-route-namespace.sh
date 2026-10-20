#!/usr/bin/env bash
set -euo pipefail

script_path="$(realpath "${BASH_SOURCE[0]}")"

if [[ "${1:-}" != "--inside" ]]; then
  if [[ $# -ne 1 ]]; then
    echo "usage: $0 <netgap-cli>" >&2
    exit 2
  fi
  if [[ "$EUID" -ne 0 ]]; then
    echo "must run as root (the CI caller should use sudo)" >&2
    exit 2
  fi

  cli_path="$(realpath "$1")"
  parent_netns="$(readlink /proc/self/ns/net)"
  parent_routes4="$(ip -4 route show)"
  parent_routes6="$(ip -6 route show)"

  set +e
  unshare --net -- bash "$script_path" --inside "$cli_path" "$parent_netns"
  test_status=$?
  set -e

  if [[ "$(readlink /proc/self/ns/net)" != "$parent_netns" || "$(ip -4 route show)" != "$parent_routes4" || "$(ip -6 route show)" != "$parent_routes6" ]]; then
    echo "the CI runner network namespace changed during the isolated CLI test" >&2
    exit 3
  fi
  exit "$test_status"
fi

if [[ $# -ne 3 || "$EUID" -ne 0 ]]; then
  echo "invalid isolated invocation" >&2
  exit 2
fi
shift
cli_path="$1"
parent_netns="$2"
test_netns="$(readlink /proc/self/ns/net)"
if [[ "$test_netns" == "$parent_netns" ]]; then
  echo "refusing to run route changes outside the isolated network namespace" >&2
  exit 3
fi

if [[ -n "$(ip -4 route show)" || -n "$(ip -6 route show)" ]]; then
  echo "isolated network namespace unexpectedly contains routes" >&2
  exit 3
fi

tmpdir="$(mktemp -d /tmp/netgap-cli-route.XXXXXX)"
recovery_dir="$tmpdir/recovery"
home_dir="$tmpdir/home"
mkdir -m 700 -p "$home_dir"

run_cli() {
  env -u XDG_CURRENT_DESKTOP \
    -u XDG_SESSION_TYPE \
    -u XDG_RUNTIME_DIR \
    -u DBUS_SESSION_BUS_ADDRESS \
    -u DESKTOP_SESSION \
    -u DISPLAY \
    -u WAYLAND_DISPLAY \
    -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u NO_PROXY \
    -u http_proxy -u https_proxy -u all_proxy -u no_proxy \
    HOME="$home_dir" "$cli_path" --recovery-dir "$recovery_dir" "$@"
}

restore_on_exit() {
  local status=$?
  if [[ -f "$recovery_dir/iproute.cache.json" ]]; then
    if [[ -z "$(ip -4 route show default)" || -z "$(ip -6 route show default)" ]]; then
      echo "attempting recovery during test cleanup" >&2
      run_cli --recover || true
    fi
  fi
  rm -rf "$tmpdir"
  return "$status"
}
trap restore_on_exit EXIT

ip link set lo up
ip address add 192.0.2.2/32 dev lo
ip -6 address add 2001:db8::2/128 dev lo nodad
ip route add default dev lo
ip -6 route add default dev lo

before4="$(ip -4 route show)"
before6="$(ip -6 route show)"
if [[ -z "$(ip -4 route show default)" || -z "$(ip -6 route show default)" ]]; then
  echo "failed to install IPv4 and IPv6 test default routes" >&2
  exit 4
fi

shutdown_output="$(run_cli --shutdown-test)"
printf '%s\n' "$shutdown_output"
if ! grep -Fq "default route deleted successfully" <<<"$shutdown_output"; then
  echo "CLI did not report successful route deletion" >&2
  exit 5
fi
if [[ -n "$(ip -4 route show default)" || -n "$(ip -6 route show default)" ]]; then
  echo "CLI left a default route in the isolated namespace" >&2
  exit 5
fi

preflight_output="$(run_cli --recovery-check)"
printf '%s\n' "$preflight_output"
if ! grep -Fq "checks passed; routes unchanged" <<<"$preflight_output"; then
  echo "recovery preflight did not pass" >&2
  exit 6
fi
if [[ -n "$(ip -4 route show default)" || -n "$(ip -6 route show default)" ]]; then
  echo "recovery preflight unexpectedly changed routes" >&2
  exit 6
fi

recover_output="$(run_cli --recover)"
printf '%s\n' "$recover_output"
if ! grep -Fq "所有保存的 IPv4/IPv6 默认路由已恢复" <<<"$recover_output"; then
  echo "CLI did not report successful route recovery" >&2
  exit 7
fi

after4="$(ip -4 route show)"
after6="$(ip -6 route show)"
if [[ "$before4" != "$after4" || "$before6" != "$after6" ]]; then
  printf 'IPv4 route mismatch\nBefore:\n%s\nAfter:\n%s\nIPv6 before:\n%s\nIPv6 after:\n%s\n' \
    "$before4" "$after4" "$before6" "$after6" >&2
  exit 8
fi

echo "isolated CLI route deletion, recovery preflight, and IPv4/IPv6 restoration passed"
