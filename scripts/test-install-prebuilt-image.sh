#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf -- "${test_root}"' EXIT

installer="${test_root}/install-sub2api-v1.2.3.sh"
sed \
  -e 's/__COMPONENT__/sub2api/g' \
  -e 's/__VERSION__/1.2.3/g' \
  "${repo_root}/scripts/install-prebuilt-image.sh" >"${installer}"
chmod 0755 "${installer}"
bash -n "${installer}"

fixture_root="${test_root}/release"
fake_bin="${test_root}/bin"
mkdir -p "${fixture_root}" "${fake_bin}"
printf 'fake docker image archive\n' | gzip >"${fixture_root}/sub2api-image-v1.2.3.tar.gz"
(
  cd "${fixture_root}"
  sha256sum sub2api-image-v1.2.3.tar.gz >SHA256SUMS
)

cat >"${fake_bin}/id" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-u" ]]; then
  echo 0
  exit 0
fi
exec /usr/bin/id "$@"
EOF

cat >"${fake_bin}/chown" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat >"${fake_bin}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    http://*|https://*)
      url="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done
[[ -n "${output}" && -n "${url}" ]]
cp "${FIXTURE_ROOT}/${url##*/}" "${output}"
EOF

cat >"${fake_bin}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${DOCKER_LOG}"
if [[ "$*" == "compose version" ]]; then
  exit 0
fi
if [[ "$*" == "load" ]]; then
  cat >/dev/null
  exit 0
fi
if [[ "$*" == image\ inspect* ]]; then
  exit 0
fi
if [[ "$*" == compose*" ps -q "* ]]; then
  echo fake-container-id
  exit 0
fi
if [[ "$*" == inspect\ --format* ]]; then
  echo "${MOCK_HEALTH:-healthy}"
  exit 0
fi
exit 0
EOF

chmod 0755 "${fake_bin}"/*

write_compose() {
  local target="$1"
  mkdir -p "${target}/deploy"
  cat >"${target}/deploy/docker-compose.yml" <<'EOF'
services:
  sub2api:
    image: hamster-switch/sub2api:hs-v1.0.0
    restart: unless-stopped
  postgres:
    image: postgres:18-alpine
EOF
}

success_target="${test_root}/success"
success_log="${test_root}/success-docker.log"
write_compose "${success_target}"
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${success_log}" MOCK_HEALTH=healthy \
  PATH="${fake_bin}:${PATH}" \
  bash "${installer}" --target "${success_target}"

grep -q 'image: hamster-switch/sub2api:hs-v1.2.3' "${success_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${success_target}/deploy/docker-compose.yml"
grep -q 'load' "${success_log}"
grep -q 'up -d --no-build sub2api' "${success_log}"
compgen -G "${success_target}/deploy/docker-compose.yml.hamster-switch.*.bak" >/dev/null

rollback_target="${test_root}/rollback"
rollback_log="${test_root}/rollback-docker.log"
write_compose "${rollback_target}"
if FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${rollback_log}" MOCK_HEALTH=unhealthy \
  PATH="${fake_bin}:${PATH}" \
  bash "${installer}" --target "${rollback_target}"; then
  echo 'expected unhealthy installation to fail' >&2
  exit 1
fi

grep -q 'image: hamster-switch/sub2api:hs-v1.0.0' "${rollback_target}/deploy/docker-compose.yml"
[[ "$(grep -c 'up -d --no-build sub2api' "${rollback_log}")" -eq 2 ]]

echo 'prebuilt image installer tests passed'
