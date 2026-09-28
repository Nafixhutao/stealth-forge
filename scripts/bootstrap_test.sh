#!/bin/sh
set -eu

script="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/bootstrap.sh"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/stealth-bootstrap-test.XXXXXX")"
trap 'rm -rf "$temporary_dir"' EXIT

system_dir="${temporary_dir}/system"
mkdir -p "${system_dir}/sources.list.d" "${system_dir}/keyrings"
printf '%s\n' 'ID=ubuntu' 'VERSION_ID="24.04"' 'VERSION_CODENAME=noble' > "${system_dir}/os-release"
test_script="${temporary_dir}/bootstrap.sh"
cp "$script" "$test_script"
sed -i \
	-e "s|os_release_path=\"/etc/os-release\"|os_release_path=\"${system_dir}/os-release\"|" \
	-e "s|apt_sources_list_path=\"/etc/apt/sources.list\"|apt_sources_list_path=\"${system_dir}/sources.list\"|" \
	-e "s|apt_sources_dir=\"/etc/apt/sources.list.d\"|apt_sources_dir=\"${system_dir}/sources.list.d\"|" \
	-e "s|apt_keyrings_dir=\"/etc/apt/keyrings\"|apt_keyrings_dir=\"${system_dir}/keyrings\"|" \
	"$test_script"
script="$test_script"

mock_bin="${temporary_dir}/bin"
mkdir -p "$mock_bin"

printf '%s\n' '#!/bin/sh' 'printf "%s\\n" "${STEALTH_TEST_UID:-1000}"' > "${mock_bin}/id"
chmod 0755 "${mock_bin}/id"

cat > "${mock_bin}/docker" <<'MOCK_DOCKER'
#!/bin/sh
case "$1" in
	--version)
		printf '%s\n' 'Docker version 27.0.0, build test'
		;;
	compose)
		if [ "${STEALTH_TEST_COMPOSE_MISSING:-0}" = 1 ] && [ ! -e "${STEALTH_TEST_INSTALLED_MARKER:-/nonexistent}" ]; then
			printf '%s\n' 'docker: unknown command: compose' >&2
			exit 1
		fi
		printf '%s\n' 'Docker Compose version v2.30.0'
		;;
	info)
		if [ "${STEALTH_TEST_DOCKER_DAEMON_FAIL:-0}" = 1 ] && [ ! -e "${STEALTH_TEST_INSTALLED_MARKER:-/nonexistent}" ]; then
			printf '%s\n' 'Cannot connect to the Docker daemon' >&2
			exit 1
		fi
		if [ "${STEALTH_TEST_DOCKER_INFO_AFTER_INSTALL_FAIL:-0}" = 1 ] && [ -e "${STEALTH_TEST_INSTALLED_MARKER:-/nonexistent}" ] && [ "${STEALTH_TEST_DOCKER_GROUP_CONTEXT:-0}" != 1 ] && [ "${STEALTH_TEST_DOCKER_AS_ROOT:-0}" != 1 ]; then
			printf '%s\n' 'Docker socket access requires group membership' >&2
			exit 1
		fi
		;;
	esac
MOCK_DOCKER
chmod 0755 "${mock_bin}/docker"

cat > "${mock_bin}/apt-get" <<'MOCK_APT'
#!/bin/sh
if [ -n "${STEALTH_TEST_APT_LOG:-}" ]; then
	printf 'apt-get %s\n' "$*" >> "$STEALTH_TEST_APT_LOG"
fi
case "$*" in
	"-s install "*)
		if [ "${STEALTH_TEST_APT_PLAN_REMOVE:-0}" = 1 ]; then
			printf '%s\n' 'Remv docker.io [1.0]'
		else
			for package do
				case "$package" in -s|install) continue ;; esac
				printf 'Inst %s [0] (1 test)\n' "$package"
			done
		fi
		;;
	"install -y "*)
		if [ "${STEALTH_TEST_APT_INSTALL_FAIL:-0}" = 1 ]; then
			exit 42
		fi
		[ -z "${STEALTH_TEST_INSTALLED_MARKER:-}" ] || : > "$STEALTH_TEST_INSTALLED_MARKER"
		;;
esac
MOCK_APT
chmod 0755 "${mock_bin}/apt-get"

cat > "${mock_bin}/sudo" <<'MOCK_SUDO'
#!/bin/sh
if [ "${STEALTH_TEST_SUDO_LOG:-}" ]; then printf 'sudo %s\n' "$*" >> "$STEALTH_TEST_SUDO_LOG"; fi
case "$*" in
	'-n -v') [ "${STEALTH_TEST_SUDO_AUTH_REQUIRED:-0}" != 1 ] && exit 0 || exit 1 ;;
	-v) exit 0 ;;
esac
command_name=$1
shift
if [ "$command_name" = docker ]; then
	STEALTH_TEST_DOCKER_AS_ROOT=1 "$command_name" "$@"
	exit $?
fi
"$command_name" "$@"
MOCK_SUDO
chmod 0755 "${mock_bin}/sudo"

cat > "${mock_bin}/getent" <<'MOCK_GETENT'
#!/bin/sh
[ "${STEALTH_TEST_DOCKER_GROUP_EXISTS:-0}" = 1 ] && [ "$1" = group ] && [ "$2" = docker ]
MOCK_GETENT
chmod 0755 "${mock_bin}/getent"

for group_command in groupadd usermod systemctl; do
	cat > "${mock_bin}/${group_command}" <<'MOCK_GROUP_COMMAND'
#!/bin/sh
if [ -n "${STEALTH_TEST_GROUP_LOG:-}" ]; then printf '%s %s\n' "${0##*/}" "$*" >> "$STEALTH_TEST_GROUP_LOG"; fi
exit 0
MOCK_GROUP_COMMAND
	chmod 0755 "${mock_bin}/${group_command}"
done

cat > "${mock_bin}/sg" <<'MOCK_SG'
#!/bin/sh
if [ -n "${STEALTH_TEST_SG_LOG:-}" ]; then printf 'sg %s\n' "$*" >> "$STEALTH_TEST_SG_LOG"; fi
[ "$1" = docker ] && [ "$2" = -c ] || exit 2
shift 2
STEALTH_TEST_DOCKER_GROUP_CONTEXT=1 sh -c "$1"
MOCK_SG
chmod 0755 "${mock_bin}/sg"

printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf "Linux" ;; -m) printf "x86_64" ;; esac' > "${mock_bin}/uname"
chmod 0755 "${mock_bin}/uname"

unsupported_bin="${temporary_dir}/unsupported-bin"
mkdir -p "$unsupported_bin"
printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf "Darwin" ;; -m) printf "arm64" ;; esac' > "${unsupported_bin}/uname"
chmod 0755 "${unsupported_bin}/uname"
if PATH="${unsupported_bin}:${PATH}" "$script" --version v1.2.3 >"${temporary_dir}/unsupported.out" 2>&1; then
	printf '%s\n' 'unsupported OS was accepted' >&2
	exit 1
fi
grep -q 'unsupported operating system' "${temporary_dir}/unsupported.out"

architecture_bin="${temporary_dir}/architecture-bin"
mkdir -p "$architecture_bin"
printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf "Linux" ;; -m) printf "ppc64le" ;; esac' > "${architecture_bin}/uname"
chmod 0755 "${architecture_bin}/uname"
if PATH="${architecture_bin}:${PATH}" "$script" --version v1.2.3 >"${temporary_dir}/architecture.out" 2>&1; then
	printf '%s\n' 'unsupported architecture was accepted' >&2
	exit 1
fi
grep -q 'unsupported architecture' "${temporary_dir}/architecture.out"

printf '%s\n' '#!/bin/sh' \
	'output=""' \
	'while [ "$#" -gt 0 ]; do' \
	'  if [ "$1" = "-o" ]; then output="$2"; shift 2; else shift; fi' \
	'done' \
	'case "$output" in' \
	'  *checksums.txt) printf "%064d  stealth_Linux_x86_64.tar.gz\n" 0 > "$output" ;;' \
	'  *) printf "%s" invalid-archive > "$output" ;;' \
	'esac' > "${mock_bin}/curl"
chmod 0755 "${mock_bin}/curl"

if PATH="${mock_bin}:${PATH}" "$script" --version v1.2.3-rc.1 >"${temporary_dir}/version.out" 2>&1; then
	printf '%s\n' 'pre-release bootstrap unexpectedly completed with the download stub' >&2
	exit 1
fi
grep -q 'download verification failed' "${temporary_dir}/version.out"
if grep -q 'release version must match' "${temporary_dir}/version.out"; then
	printf '%s\n' 'valid pre-release version hit version validation' >&2
	exit 1
fi

if PATH="${mock_bin}:${PATH}" HOME="$temporary_dir" "$script" --version v1.2.3 >"${temporary_dir}/checksum.out" 2>&1; then
	printf '%s\n' 'checksum mismatch was accepted' >&2
	exit 1
fi
grep -q 'download verification failed' "${temporary_dir}/checksum.out"

failure_bin="${temporary_dir}/failure-bin"
mkdir -p "$failure_bin"
printf '%s\n' '#!/bin/sh' 'exit 1' > "${failure_bin}/curl"
chmod 0755 "${failure_bin}/curl"
if PATH="${failure_bin}:${mock_bin}:${PATH}" "$script" --version v1.2.3 >"${temporary_dir}/download.out" 2>&1; then
	printf '%s\n' 'download failure was accepted' >&2
	exit 1
fi
grep -q 'could not download' "${temporary_dir}/download.out"

latest_bin="${temporary_dir}/latest-bin"
mkdir -p "$latest_bin"
printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf "Linux" ;; -m) printf "x86_64" ;; esac' > "${latest_bin}/uname"
chmod 0755 "${latest_bin}/uname"
cat > "${latest_bin}/curl" <<'MOCK_EOF'
#!/bin/sh
latest_requested=0
for arg in "$@"; do
	case "$arg" in
		*releases/latest*) latest_requested=1 ;;
	esac
done
if [ -n "${STEALTH_TEST_CURL_LOG:-}" ]; then
	printf '%s\n' "$*" >> "$STEALTH_TEST_CURL_LOG"
fi
if [ "$latest_requested" = "1" ]; then
	if [ -n "${STEALTH_TEST_MARKER:-}" ]; then
		: > "$STEALTH_TEST_MARKER"
	fi
	if [ "${STEALTH_TEST_LATEST_FAIL:-0}" = "1" ]; then
		exit 22
	fi
	printf '%s' "${STEALTH_TEST_LATEST_URL:-}"
	exit 0
fi
exit 1
MOCK_EOF
chmod 0755 "${latest_bin}/curl"
cp "${mock_bin}/docker" "${latest_bin}/docker"
cp "${mock_bin}/id" "${latest_bin}/id"

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://github.com/Stealth-deplover/stealth/releases/tag/v0.1.0" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/latest-valid.out" 2>&1; then
	printf '%s\n' 'valid latest redirect was not handled' >&2
	exit 1
fi
grep -q 'could not download .* for v0\.1\.0' "${temporary_dir}/latest-valid.out"
if grep -q 'release version must match' "${temporary_dir}/latest-valid.out"; then
	printf '%s\n' 'valid latest redirect hit version validation' >&2
	exit 1
fi
if grep -q 'no stable GitHub release' "${temporary_dir}/latest-valid.out"; then
	printf '%s\n' 'valid latest redirect reported no release' >&2
	exit 1
fi
if grep -q 'failed to determine latest release' "${temporary_dir}/latest-valid.out"; then
	printf '%s\n' 'valid latest redirect reported undetermined release' >&2
	exit 1
fi

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://github.com/Stealth-deplover/stealth/releases/tag/v0.3.0-rc.1" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/latest-rc.out" 2>&1; then
	printf '%s\n' 'latest release RC redirect was accepted' >&2
	exit 1
fi
grep -q 'latest GitHub release is not stable' "${temporary_dir}/latest-rc.out"

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://github.com/Stealth-deplover/stealth/releases/tag/v10.20.30" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/latest-multidigit.out" 2>&1; then
	printf '%s\n' 'multi-digit latest redirect was not handled' >&2
	exit 1
fi
grep -q 'could not download .* for v10\.20\.30' "${temporary_dir}/latest-multidigit.out"

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://github.com/Stealth-deplover/stealth/releases" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/no-release.out" 2>&1; then
	printf '%s\n' 'no-release redirect was accepted' >&2
	exit 1
fi
grep -q 'no stable GitHub release is available yet' "${temporary_dir}/no-release.out"
if grep -q 'release version must match' "${temporary_dir}/no-release.out"; then
	printf '%s\n' 'no-release case leaked parser output' >&2
	exit 1
fi

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://github.com/Stealth-deplover/stealth/releases/" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/no-release-slash.out" 2>&1; then
	printf '%s\n' 'trailing-slash no-release redirect was accepted' >&2
	exit 1
fi
grep -q 'no stable GitHub release is available yet' "${temporary_dir}/no-release-slash.out"

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://example.com/unexpected" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/malformed.out" 2>&1; then
	printf '%s\n' 'malformed redirect was accepted' >&2
	exit 1
fi
grep -q 'failed to determine latest release' "${temporary_dir}/malformed.out"

if STEALTH_VERSION= STEALTH_TEST_LATEST_URL="https://github.com/Stealth-deplover/stealth/releases/tag/" \
	STEALTH_TEST_LATEST_FAIL=0 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/empty-tag.out" 2>&1; then
	printf '%s\n' 'empty tag redirect was accepted' >&2
	exit 1
fi
grep -q 'failed to determine latest release' "${temporary_dir}/empty-tag.out"

if STEALTH_VERSION= STEALTH_TEST_LATEST_FAIL=1 PATH="${latest_bin}:${PATH}" \
	"$script" >"${temporary_dir}/latest-fail.out" 2>&1; then
	printf '%s\n' 'latest network failure was accepted' >&2
	exit 1
fi
grep -q 'could not resolve the latest stable release' "${temporary_dir}/latest-fail.out"

for invalid_version in latest releases main v1 v1.2 1.2.3 v1.2.3-beta v1.2.3-rc v1.2.3-rc.foo v1.2.3-rc.1.2; do
	if PATH="${mock_bin}:${PATH}" "$script" --version "$invalid_version" >"${temporary_dir}/invalid-${invalid_version}.out" 2>&1; then
		printf '%s\n' "invalid version ${invalid_version} was accepted" >&2
		exit 1
	fi
	grep -q 'release version must match' "${temporary_dir}/invalid-${invalid_version}.out"
done

rm -f "${temporary_dir}/explicit-marker"
if STEALTH_VERSION= STEALTH_TEST_MARKER="${temporary_dir}/explicit-marker" \
	STEALTH_TEST_LATEST_FAIL=1 PATH="${latest_bin}:${PATH}" \
	"$script" --version v0.1.0 >"${temporary_dir}/explicit-valid.out" 2>&1; then
	printf '%s\n' 'explicit valid version was accepted without download stub failure' >&2
	exit 1
fi
grep -q 'could not download .* for v0\.1\.0' "${temporary_dir}/explicit-valid.out"
if [ -e "${temporary_dir}/explicit-marker" ]; then
	printf '%s\n' 'explicit --version contacted /releases/latest' >&2
	exit 1
fi

rm -f "${temporary_dir}/explicit-env-marker"
if STEALTH_TEST_MARKER="${temporary_dir}/explicit-env-marker" \
	STEALTH_TEST_LATEST_FAIL=1 PATH="${latest_bin}:${PATH}" \
	STEALTH_VERSION=v0.1.0 "$script" >"${temporary_dir}/explicit-env.out" 2>&1; then
	printf '%s\n' 'STEALTH_VERSION was accepted without download stub failure' >&2
	exit 1
fi
grep -q 'could not download .* for v0\.1\.0' "${temporary_dir}/explicit-env.out"
if [ -e "${temporary_dir}/explicit-env-marker" ]; then
	printf '%s\n' 'STEALTH_VERSION contacted /releases/latest' >&2
	exit 1
fi

rm -f "${temporary_dir}/explicit-rc-marker"
if STEALTH_TEST_MARKER="${temporary_dir}/explicit-rc-marker" \
	STEALTH_TEST_LATEST_FAIL=1 PATH="${latest_bin}:${PATH}" \
	STEALTH_VERSION=v0.1.0-rc.1 "$script" >"${temporary_dir}/explicit-rc.out" 2>&1; then
	printf '%s\n' 'explicit RC version was accepted without download stub failure' >&2
	exit 1
fi
grep -q 'could not download' "${temporary_dir}/explicit-rc.out"
if [ -e "${temporary_dir}/explicit-rc-marker" ]; then
	printf '%s\n' 'explicit RC version contacted /releases/latest' >&2
	exit 1
fi

if STEALTH_VERSION= PATH="${latest_bin}:${PATH}" "$script" --version=v1.2.3 >"${temporary_dir}/version-equals.out" 2>&1; then
	printf '%s\n' '--version= form unexpectedly succeeded without download stub' >&2
	exit 1
fi
grep -q 'could not download' "${temporary_dir}/version-equals.out"

arm_bin="${temporary_dir}/arm-bin"
mkdir -p "$arm_bin"
printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf "Linux" ;; -m) printf "aarch64" ;; esac' > "${arm_bin}/uname"
chmod 0755 "${arm_bin}/uname"
cp "${latest_bin}/curl" "${arm_bin}/curl"
cp "${mock_bin}/docker" "${arm_bin}/docker"
cp "${mock_bin}/id" "${arm_bin}/id"
chmod 0755 "${arm_bin}/curl"

rm -f "${temporary_dir}/curl-amd64.log"
if STEALTH_TEST_CURL_LOG="${temporary_dir}/curl-amd64.log" PATH="${latest_bin}:${PATH}" \
	"$script" --version v1.2.3 >"${temporary_dir}/arch-amd64.out" 2>&1; then
	printf '%s\n' 'amd64 mapping unexpectedly succeeded without download stub' >&2
	exit 1
fi
grep -q 'stealth_Linux_x86_64.tar.gz' "${temporary_dir}/curl-amd64.log"

rm -f "${temporary_dir}/curl-arm64.log"
if STEALTH_TEST_CURL_LOG="${temporary_dir}/curl-arm64.log" PATH="${arm_bin}:${PATH}" \
	"$script" --version v1.2.3 >"${temporary_dir}/arch-arm64.out" 2>&1; then
	printf '%s\n' 'arm64 mapping unexpectedly succeeded without download stub' >&2
	exit 1
fi
grep -q 'stealth_Linux_arm64.tar.gz' "${temporary_dir}/curl-arm64.log"

existing_apt_log="${temporary_dir}/existing-apt.log"
rm -f "$existing_apt_log"
if STEALTH_TEST_APT_LOG="$existing_apt_log" PATH="${mock_bin}:${PATH}" \
	"$script" --version v1.2.3 >"${temporary_dir}/docker-existing.out" 2>&1; then
	printf '%s\n' 'existing Docker bootstrap unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'download verification failed' "${temporary_dir}/docker-existing.out"
[ ! -e "$existing_apt_log" ] || { printf '%s\n' 'working Docker triggered package installation' >&2; exit 1; }

missing_script="${temporary_dir}/bootstrap-docker-missing.sh"
cp "$script" "$missing_script"
sed -i 's/if command -v docker >\/dev\/null 2>&1; then/if false; then/' "$missing_script"
sed -i '/^docker_engine_installed() {$/,/^}$/c\docker_engine_installed() { return 1; }' "$missing_script"
if grep -q 'if command -v docker >\/dev\/null 2>&1; then' "$missing_script"; then
	printf '%s\n' 'Docker-missing test seam was not applied' >&2
	exit 1
fi

root_apt_log="${temporary_dir}/root-apt.log"
root_installed_marker="${temporary_dir}/root-installed"
if STEALTH_TEST_UID=0 STEALTH_TEST_APT_LOG="$root_apt_log" \
	STEALTH_TEST_INSTALLED_MARKER="$root_installed_marker" PATH="${mock_bin}:${PATH}" \
	"$missing_script" --version v1.2.3 >"${temporary_dir}/docker-missing-root.out" 2>&1; then
	printf '%s\n' 'root Docker installation unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'download verification failed' "${temporary_dir}/docker-missing-root.out"
grep -q 'apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin' "$root_apt_log"
[ -e "$root_installed_marker" ] || { printf '%s\n' 'root package installation mock did not run' >&2; exit 1; }
[ -e "${system_dir}/sources.list.d/docker.sources" ] || { printf '%s\n' 'official Docker APT source was not configured in the isolated test system' >&2; exit 1; }
grep -q 'Signed-By:' "${system_dir}/sources.list.d/docker.sources"

cli_missing_script="${temporary_dir}/bootstrap-cli-missing.sh"
cp "$script" "$cli_missing_script"
sed -i 's/if command -v docker >\/dev\/null 2>&1; then/if false; then/' "$cli_missing_script"
sed -i '/^docker_engine_installed() {$/,/^}$/c\docker_engine_installed() { return 0; }' "$cli_missing_script"
cli_apt_log="${temporary_dir}/cli-missing-apt.log"
cli_installed_marker="${temporary_dir}/cli-missing-installed"
if STEALTH_TEST_UID=0 STEALTH_TEST_APT_LOG="$cli_apt_log" \
	STEALTH_TEST_INSTALLED_MARKER="$cli_installed_marker" PATH="${mock_bin}:${PATH}" \
	"$cli_missing_script" --version v1.2.3 >"${temporary_dir}/cli-missing.out" 2>&1; then
	printf '%s\n' 'installed Engine without CLI test unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'apt-get install -y docker-ce-cli docker-compose-plugin' "$cli_apt_log"
if grep -q 'apt-get install -y .*containerd.io' "$cli_apt_log"; then
	printf '%s\n' 'CLI repair reinstalled the existing Docker Engine' >&2
	exit 1
fi

group_apt_log="${temporary_dir}/group-apt.log"
group_log="${temporary_dir}/group-commands.log"
group_sg_log="${temporary_dir}/group-sg.log"
group_installed_marker="${temporary_dir}/group-installed"
if STEALTH_TEST_UID=1000 USER=stealth-test-user STEALTH_TEST_DOCKER_INFO_AFTER_INSTALL_FAIL=1 \
	STEALTH_TEST_APT_LOG="$group_apt_log" STEALTH_TEST_GROUP_LOG="$group_log" \
	STEALTH_TEST_SG_LOG="$group_sg_log" STEALTH_TEST_INSTALLED_MARKER="$group_installed_marker" \
	PATH="${mock_bin}:${PATH}" "$missing_script" --version v1.2.3 >"${temporary_dir}/docker-group.out" 2>&1; then
	printf '%s\n' 'docker group recovery test unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'usermod -aG docker stealth-test-user' "$group_log"
grep -q '^sg docker -c docker info$' "$group_sg_log"
grep -q 'Docker daemon is accessible' "${temporary_dir}/docker-group.out"

sudo_apt_log="${temporary_dir}/sudo-apt.log"
sudo_log="${temporary_dir}/sudo.log"
sudo_installed_marker="${temporary_dir}/sudo-installed"
if STEALTH_TEST_UID=1000 STEALTH_TEST_APT_LOG="$sudo_apt_log" STEALTH_TEST_SUDO_LOG="$sudo_log" \
	STEALTH_TEST_INSTALLED_MARKER="$sudo_installed_marker" PATH="${mock_bin}:${PATH}" \
	"$missing_script" --version v1.2.3 >"${temporary_dir}/docker-missing-sudo.out" 2>&1; then
	printf '%s\n' 'sudo Docker installation unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'download verification failed' "${temporary_dir}/docker-missing-sudo.out"
grep -q '^sudo apt-get install -y docker-ce ' "$sudo_log"

make_tty_script() {
	source_script=$1
	target_script=$2
	tty_result=$3
	awk -v tty_result="$tty_result" '
		/^has_controlling_tty\(\) \{$/ {
			print "has_controlling_tty() {"
			print "\treturn " tty_result
			skip_body = 1
			next
		}
		skip_body && /^}$/ {
			print "}"
			skip_body = 0
			next
		}
		!skip_body { print }
	' "$source_script" > "$target_script"
	chmod 0755 "$target_script"
}

tty_script="${temporary_dir}/bootstrap-tty.sh"
make_tty_script "$missing_script" "$tty_script" 0
auth_sudo_log="${temporary_dir}/auth-sudo.log"
auth_apt_log="${temporary_dir}/auth-apt.log"
auth_installed_marker="${temporary_dir}/auth-installed"
if STEALTH_TEST_UID=1000 STEALTH_TEST_SUDO_AUTH_REQUIRED=1 STEALTH_TEST_SUDO_LOG="$auth_sudo_log" \
	STEALTH_TEST_APT_LOG="$auth_apt_log" STEALTH_TEST_INSTALLED_MARKER="$auth_installed_marker" \
	PATH="${mock_bin}:${PATH}" "$tty_script" --version v1.2.3 >"${temporary_dir}/sudo-auth.out" 2>&1; then
	printf '%s\n' 'sudo authentication test unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'sudo will request authentication directly' "${temporary_dir}/sudo-auth.out"
grep -q '^sudo -v$' "$auth_sudo_log"
grep -q '^sudo apt-get install -y docker-ce ' "$auth_sudo_log"

no_tty_script="${temporary_dir}/bootstrap-no-tty.sh"
make_tty_script "$missing_script" "$no_tty_script" 1
no_tty_sudo_log="${temporary_dir}/no-tty-sudo.log"
no_tty_apt_log="${temporary_dir}/no-tty-apt.log"
if STEALTH_TEST_UID=1000 STEALTH_TEST_SUDO_AUTH_REQUIRED=1 STEALTH_TEST_SUDO_LOG="$no_tty_sudo_log" \
	STEALTH_TEST_APT_LOG="$no_tty_apt_log" PATH="${mock_bin}:${PATH}" \
	"$no_tty_script" --version v1.2.3 >"${temporary_dir}/sudo-no-tty.out" 2>&1; then
	printf '%s\n' 'sudo without a TTY unexpectedly continued' >&2
	exit 1
fi
grep -q 'no controlling TTY is available' "${temporary_dir}/sudo-no-tty.out"
if grep -q '^sudo -v$' "$no_tty_sudo_log"; then
	printf '%s\n' 'bootstrap attempted interactive sudo without a TTY' >&2
	exit 1
fi
[ ! -e "$no_tty_apt_log" ] || { printf '%s\n' 'bootstrap ran APT without sudo authentication' >&2; exit 1; }

printf '%s\n' 'ID=fedora' 'VERSION_ID="42"' 'VERSION_CODENAME=forty-two' > "${system_dir}/os-release"
unsupported_apt_log="${temporary_dir}/unsupported-apt.log"
if STEALTH_TEST_UID=0 STEALTH_TEST_APT_LOG="$unsupported_apt_log" PATH="${mock_bin}:${PATH}" \
	"$missing_script" --version v1.2.3 >"${temporary_dir}/unsupported-distro.out" 2>&1; then
	printf '%s\n' 'unsupported distribution was accepted' >&2
	exit 1
fi
grep -q 'unsupported distribution fedora 42' "${temporary_dir}/unsupported-distro.out"
grep -q 'missing prerequisites: Docker Engine and Docker Compose v2 plugin' "${temporary_dir}/unsupported-distro.out"
[ ! -e "$unsupported_apt_log" ] || { printf '%s\n' 'unsupported distribution ran APT' >&2; exit 1; }
printf '%s\n' 'ID=ubuntu' 'VERSION_ID="24.04"' 'VERSION_CODENAME=noble' > "${system_dir}/os-release"

compose_apt_log="${temporary_dir}/compose-apt.log"
compose_installed_marker="${temporary_dir}/compose-installed"
if STEALTH_TEST_UID=1000 STEALTH_TEST_COMPOSE_MISSING=1 STEALTH_TEST_APT_LOG="$compose_apt_log" \
	STEALTH_TEST_INSTALLED_MARKER="$compose_installed_marker" PATH="${mock_bin}:${PATH}" \
	"$script" --version v1.2.3 >"${temporary_dir}/compose-missing.out" 2>&1; then
	printf '%s\n' 'Compose-missing bootstrap unexpectedly passed its invalid archive fixture' >&2
	exit 1
fi
grep -q 'download verification failed' "${temporary_dir}/compose-missing.out"
grep -q 'apt-get install -y docker-compose-plugin' "$compose_apt_log"
if grep -q 'apt-get install -y .*docker-ce' "$compose_apt_log"; then
	printf '%s\n' 'Compose-only repair replaced Docker Engine packages' >&2
	exit 1
fi

daemon_apt_log="${temporary_dir}/daemon-apt.log"
if STEALTH_TEST_UID=1000 STEALTH_TEST_DOCKER_DAEMON_FAIL=1 STEALTH_TEST_APT_LOG="$daemon_apt_log" \
	PATH="${mock_bin}:${PATH}" "$script" --version v1.2.3 >"${temporary_dir}/daemon-inaccessible.out" 2>&1; then
	printf '%s\n' 'inaccessible Docker daemon unexpectedly continued' >&2
	exit 1
fi
grep -q 'Docker is installed but the daemon is inaccessible' "${temporary_dir}/daemon-inaccessible.out"
[ ! -e "$daemon_apt_log" ] || { printf '%s\n' 'daemon failure triggered package changes' >&2; exit 1; }

apt_failure_log="${temporary_dir}/apt-failure.log"
if STEALTH_TEST_UID=0 STEALTH_TEST_APT_INSTALL_FAIL=1 STEALTH_TEST_APT_LOG="$apt_failure_log" \
	PATH="${mock_bin}:${PATH}" "$missing_script" --version v1.2.3 >"${temporary_dir}/apt-failure.out" 2>&1; then
	printf '%s\n' 'APT package failure was ignored' >&2
	exit 1
fi
grep -q 'APT failed while installing Docker Engine and Docker Compose v2 plugin' "${temporary_dir}/apt-failure.out"
if grep -q 'could not download stealth_Linux' "${temporary_dir}/apt-failure.out"; then
	printf '%s\n' 'bootstrap continued to the Stealth download after APT failed' >&2
	exit 1
fi

if PATH="${mock_bin}:${PATH}" STEALTH_SUDO_PASSWORD='test-only-placeholder' \
	"$script" --sudo-password test >"${temporary_dir}/sudo-password-flag.out" 2>&1; then
	printf '%s\n' 'bootstrap accepted --sudo-password' >&2
	exit 1
fi
grep -q 'unknown option: --sudo-password' "${temporary_dir}/sudo-password-flag.out"
if grep -R -q 'test-only-placeholder' "${temporary_dir}"; then
	printf '%s\n' 'test sudo placeholder was written to a file' >&2
	exit 1
fi

flow_bin="${temporary_dir}/flow-bin"
flow_cli="${temporary_dir}/flow-cli"
flow_install="${temporary_dir}/flow-install"
mkdir -p "$flow_bin" "$flow_cli"
cp "${mock_bin}/docker" "$flow_bin/docker"
cp "${mock_bin}/id" "$flow_bin/id"
cp "${mock_bin}/uname" "$flow_bin/uname"
cat > "$flow_cli/stealth" <<'MOCK_STEALTH'
#!/bin/sh
if [ -n "${STEALTH_TEST_CLI_LOG:-}" ]; then printf '%s\n' "$*" >> "$STEALTH_TEST_CLI_LOG"; fi
exit 99
MOCK_STEALTH
chmod 0755 "$flow_cli/stealth"
flow_archive="${temporary_dir}/stealth_Linux_x86_64.tar.gz"
flow_checksums="${temporary_dir}/flow-checksums.txt"
tar -czf "$flow_archive" -C "$flow_cli" stealth
flow_checksum="$(sha256sum "$flow_archive" | awk '{print $1}')"
printf '%s  stealth_Linux_x86_64.tar.gz\n' "$flow_checksum" > "$flow_checksums"
cat > "$flow_bin/curl" <<'MOCK_FLOW_CURL'
#!/bin/sh
output=''
url=''
while [ "$#" -gt 0 ]; do
	case "$1" in
		-o) output="$2"; shift 2 ;;
		https://*) url="$1"; shift ;;
		*) shift ;;
	esac
done
case "$url" in
	*/checksums.txt) source_file="$STEALTH_TEST_CHECKSUMS" ;;
	*/stealth_Linux_x86_64.tar.gz) source_file="$STEALTH_TEST_ARCHIVE" ;;
	*) exit 1 ;;
esac
cp "$source_file" "$output"
MOCK_FLOW_CURL
chmod 0755 "$flow_bin/curl"
flow_cli_log="${temporary_dir}/flow-cli.log"
if STEALTH_TEST_UID=0 STEALTH_TEST_ARCHIVE="$flow_archive" STEALTH_TEST_CHECKSUMS="$flow_checksums" \
	STEALTH_TEST_CLI_LOG="$flow_cli_log" STEALTH_BIN_DIR="$flow_install" HOME="$temporary_dir" \
	PATH="$flow_bin:$PATH" "$script" --version v1.2.3 >"${temporary_dir}/flow-no-tty.out" 2>&1; then
	printf '%s\n' 'bootstrap unexpectedly continued setup without a TTY' >&2
	exit 1
fi
grep -qi 'interactive setup requires a TTY' "${temporary_dir}/flow-no-tty.out"
[ -x "$flow_install/stealth" ] || { printf '%s\n' 'CLI was not installed before the no-TTY handoff error' >&2; exit 1; }
[ ! -e "$flow_cli_log" ] || { printf '%s\n' 'bootstrap invoked install --wait without a TTY' >&2; exit 1; }

tty_handoff_script="${temporary_dir}/bootstrap-tty-handoff.sh"
sed 's/if \[ -t 1 \] \&\& \[ -t 2 \]; then/if true; then/' "$script" > "$tty_handoff_script"
chmod 0755 "$tty_handoff_script"
rm -rf "$flow_install"
if STEALTH_TEST_UID=0 STEALTH_TEST_ARCHIVE="$flow_archive" STEALTH_TEST_CHECKSUMS="$flow_checksums" \
	STEALTH_TEST_CLI_LOG="$flow_cli_log" STEALTH_BIN_DIR="$flow_install" HOME="$temporary_dir" \
	PATH="$flow_bin:$PATH" "$tty_handoff_script" --version v1.2.3 >"${temporary_dir}/flow-tty.out" 2>&1; then
	printf '%s\n' 'fake CLI unexpectedly succeeded during the TTY handoff test' >&2
	exit 1
fi
grep -qx 'install' "$flow_cli_log"

printf '%s\n' 'bootstrap tests passed'
