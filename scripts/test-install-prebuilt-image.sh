#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf -- "${test_root}"' EXIT

if LC_ALL=C grep -q $'\r' "${repo_root}/scripts/install-prebuilt-image.sh"; then
  echo 'installer template must use LF line endings' >&2
  exit 1
fi

installer="${test_root}/install-sub2api-v1.2.3.sh"
sed \
  -e 's/__COMPONENT__/sub2api/g' \
  -e 's/__CHANNEL__//g' \
  -e 's/__VERSION__/1.2.3/g' \
  "${repo_root}/scripts/install-prebuilt-image.sh" >"${installer}"
chmod 0755 "${installer}"
bash -n "${installer}"

hamster_installer="${test_root}/install-sub2api-hamster-v1.2.3.sh"
sed \
  -e 's/__COMPONENT__/sub2api/g' \
  -e 's/__CHANNEL__/hamster/g' \
  -e 's/__VERSION__/1.2.3/g' \
  "${repo_root}/scripts/install-prebuilt-image.sh" >"${hamster_installer}"
bash -n "${hamster_installer}"
bash "${hamster_installer}" --help | grep -q 'hamster-switch/sub2api:hamster-v1.2.3'
if grep -q '__CHANNEL__' "${hamster_installer}"; then
  echo 'channel placeholder was not replaced' >&2
  exit 1
fi

fixture_root="${test_root}/release"
fake_bin="${test_root}/bin"
mkdir -p "${fixture_root}" "${fake_bin}"
printf 'fake docker image archive\n' | gzip >"${fixture_root}/sub2api-image-v1.2.3.tar.gz"
printf 'fake hamster channel image archive\n' | gzip >"${fixture_root}/sub2api-hamster-image-v1.2.3.tar.gz"
(
  cd "${fixture_root}"
  sha256sum sub2api-image-v1.2.3.tar.gz sub2api-hamster-image-v1.2.3.tar.gz >SHA256SUMS
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
if [[ "$*" == "ps --format {{.ID}}|{{.Image}}|{{.Names}}|{{.Label \"com.docker.compose.service\"}}" ]]; then
  if [[ "${MOCK_MULTIPLE_CONTAINERS:-0}" == "1" ]]; then
    printf 'container-one|hamster-switch/sub2api:hs-v1.0.0|sub2api-one|sub2api\ncontainer-two|hamster-switch/sub2api:hs-v1.0.0|sub2api-two|sub2api\n'
  elif [[ -n "${MOCK_COMPOSE_TARGET:-}" ]]; then
    echo "running-container-id|${MOCK_IMAGE:-hamster-switch/sub2api:hs-v1.0.0}|${MOCK_CONTAINER_NAME:-sub2api}|${MOCK_SERVICE:-sub2api}"
  fi
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.project.working_dir* ]]; then
  echo "${MOCK_COMPOSE_TARGET}"
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.project* && "$*" != *config_files* && "$*" != *working_dir* ]]; then
  echo 'original-project-name'
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.project.config_files* ]]; then
  echo "${MOCK_COMPOSE_TARGET}/deploy/docker-compose.yml"
  exit 0
fi
if [[ "$*" == inspect*com.docker.compose.service* ]]; then
  echo "${MOCK_SERVICE:-sub2api}"
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
  local service="${2:-sub2api}"
  local image="${3:-hamster-switch/sub2api:hs-v1.0.0}"
  mkdir -p "${target}/deploy"
  cat >"${target}/deploy/docker-compose.yml" <<EOF
services:
  ${service}:
    image: ${image}
    restart: unless-stopped
  postgres:
    image: postgres:18-alpine
EOF
}

success_target="${test_root}/success"
success_log="${test_root}/success-docker.log"
write_compose "${success_target}"
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${success_log}" MOCK_HEALTH=healthy \
  MOCK_COMPOSE_TARGET="${success_target}" \
  PATH="${fake_bin}:${PATH}" \
  bash "${installer}"

grep -q 'image: hamster-switch/sub2api:hs-v1.2.3' "${success_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${success_target}/deploy/docker-compose.yml"
grep -q 'load' "${success_log}"
grep -q 'compose -p original-project-name -f .* up -d --no-build sub2api' "${success_log}"
compgen -G "${success_target}/deploy/docker-compose.yml.hamster-switch.*.bak" >/dev/null

hamster_target="${test_root}/hamster-success"
hamster_log="${test_root}/hamster-success-docker.log"
write_compose "${hamster_target}"
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${hamster_log}" MOCK_HEALTH=healthy \
  MOCK_COMPOSE_TARGET="${hamster_target}" \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}"

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${hamster_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${hamster_target}/deploy/docker-compose.yml"
grep -q 'load' "${hamster_log}"

themed_target="${test_root}/themed-success"
themed_log="${test_root}/themed-success-docker.log"
write_compose "${themed_target}" backend 'private/sub2api-themed:latest'
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${themed_log}" MOCK_HEALTH=healthy \
  MOCK_COMPOSE_TARGET="${themed_target}" MOCK_SERVICE=backend \
  MOCK_IMAGE='private/sub2api-themed:latest' MOCK_CONTAINER_NAME='custom-router' \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}"

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${themed_target}/deploy/docker-compose.yml"
grep -q 'image: postgres:18-alpine' "${themed_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build backend' "${themed_log}"

explicit_target="${test_root}/explicit-success"
explicit_log="${test_root}/explicit-success-docker.log"
write_compose "${explicit_target}" backend 'private/renamed-router:latest'
FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${explicit_log}" MOCK_HEALTH=healthy \
  PATH="${fake_bin}:${PATH}" \
  bash "${hamster_installer}" \
    --compose-file "${explicit_target}/deploy/docker-compose.yml" \
    --service backend

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${explicit_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build backend' "${explicit_log}"

stopped_target="${test_root}/stopped-success"
stopped_log="${test_root}/stopped-success-docker.log"
write_compose "${stopped_target}"
(
  cd "${stopped_target}"
  FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${stopped_log}" MOCK_HEALTH=healthy \
    PATH="${fake_bin}:${PATH}" \
    bash "${hamster_installer}"
)

grep -q 'image: hamster-switch/sub2api:hamster-v1.2.3' "${stopped_target}/deploy/docker-compose.yml"
grep -q 'up -d --no-build sub2api' "${stopped_log}"

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

ambiguous_log="${test_root}/ambiguous-docker.log"
if FIXTURE_ROOT="${fixture_root}" DOCKER_LOG="${ambiguous_log}" MOCK_MULTIPLE_CONTAINERS=1 \
  PATH="${fake_bin}:${PATH}" bash "${installer}"; then
  echo 'expected ambiguous auto-detection to fail' >&2
  exit 1
fi

echo 'prebuilt image installer tests passed'
