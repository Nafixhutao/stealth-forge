#!/bin/sh
set -eu

repository="Stealth-deplover/stealth"
release_root="https://github.com/${repository}/releases/download"
tmp_root="${TMPDIR:-/tmp}"
temporary_dir=""
os_release_path="/etc/os-release"
apt_sources_list_path="/etc/apt/sources.list"
apt_sources_dir="/etc/apt/sources.list.d"
apt_keyrings_dir="/etc/apt/keyrings"

fail() {
	printf 'stealth bootstrap: %s\n' "$1" >&2
	exit 1
}

read_os_release_value() {
	requested_key=$1
	while IFS='=' read -r key value; do
		if [ "$key" = "$requested_key" ]; then
			case "$value" in
				\"*\") value=${value#\"}; value=${value%\"} ;;
				\'*\') value=${value#\'}; value=${value%\'} ;;
			esac
			printf '%s' "$value"
			return 0
		fi
	done < "$os_release_path"
	return 0
}

has_controlling_tty() {
	(: </dev/tty) 2>/dev/null
}

run_privileged() {
	if [ "$current_uid" -eq 0 ]; then
		"$@"
	else
		sudo "$@"
	fi
}

ensure_sudo_auth() {
	[ "$current_uid" -eq 0 ] && return 0
	command -v sudo >/dev/null 2>&1 || fail "Docker prerequisites are missing and sudo is not installed; install Docker Engine and the Docker Compose plugin, then rerun bootstrap"
	if sudo -n -v >/dev/null 2>&1; then
		return 0
	fi
	if ! has_controlling_tty; then
		fail "Docker prerequisites are missing and sudo needs authentication, but no controlling TTY is available; rerun this command from an interactive terminal"
	fi
	printf '%s\n' 'Docker prerequisites require administrator access; sudo will request authentication directly.' >&2
	sudo -v || fail "sudo authentication failed; Docker prerequisites were not installed"
}

apt_plan_is_safe() {
	plan_file=$1
	if awk '/^Remv / { found=1 } END { exit !found }' "$plan_file"; then
		return 1
	fi
	if [ "$install_mode" = compose ] && awk '/^(Inst|Conf) (docker-ce|docker-ce-cli|containerd.io|docker-buildx-plugin)( |$)/ { found=1 } END { exit !found }' "$plan_file"; then
		return 1
	fi
	if [ "$install_mode" = cli_compose ] && awk '/^(Inst|Conf) (docker-ce|containerd.io|docker-buildx-plugin)( |$)/ { found=1 } END { exit !found }' "$plan_file"; then
		return 1
	fi
	return 0
}

docker_engine_installed() {
	command -v dockerd >/dev/null 2>&1 && return 0
	if command -v dpkg-query >/dev/null 2>&1; then
		for engine_package in docker-ce docker.io; do
			engine_package_status=$(dpkg-query -W -f='${db:Status-Status}' "$engine_package" 2>/dev/null || true)
			[ "$engine_package_status" = installed ] && return 0
		done
	fi
	return 1
}

wait_for_new_docker_daemon() {
	attempt=0
	while [ "$attempt" -lt 15 ]; do
		if docker info >/dev/null 2>&1 || { [ "$current_uid" -ne 0 ] && sudo docker info >/dev/null 2>&1; }; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 2
	done
	return 1
}

install_docker_prerequisites() {
	missing_description=$1
	install_mode=$2
	case "$distribution_id:$distribution_version:$distribution_codename" in
		ubuntu:22.04:jammy|ubuntu:24.04:noble|ubuntu:26.04:resolute)
			docker_distribution=ubuntu
			;;
		debian:12:bookworm|debian:13:trixie)
			docker_distribution=debian
			;;
		*)
			fail "unsupported distribution ${distribution_id:-unknown} ${distribution_version:-unknown} (${distribution_codename:-unknown}); missing prerequisites: ${missing_description}. Install Docker Engine and the Docker Compose v2 plugin using your distribution's supported packages, then rerun bootstrap"
			;;
	esac
	case "$docker_arch" in
		amd64|arm64) ;;
		*) fail "unsupported Docker repository architecture: $docker_arch" ;;
	esac

	ensure_sudo_auth
	temporary_dir="$(mktemp -d "${tmp_root%/}/stealth-bootstrap.XXXXXX")" || fail "could not create a temporary directory"
	docker_key="${temporary_dir}/docker.asc"
	docker_sources="${temporary_dir}/docker.sources"
	apt_plan="${temporary_dir}/apt-plan.txt"

	if ! grep -R -qF "https://download.docker.com/linux/${docker_distribution}" "$apt_sources_list_path" "$apt_sources_dir" 2>/dev/null; then
		curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
			"https://download.docker.com/linux/${docker_distribution}/gpg" -o "$docker_key" || fail "could not download Docker's official APT signing key"
		run_privileged install -d -m 0755 "$apt_keyrings_dir" || fail "could not create Docker's APT keyring directory"
		run_privileged install -m 0644 "$docker_key" "${apt_keyrings_dir}/docker.asc" || fail "could not install Docker's APT signing key"
		cat > "$docker_sources" <<EOF
Types: deb
URIs: https://download.docker.com/linux/${docker_distribution}
Suites: ${distribution_codename}
Components: stable
Architectures: ${docker_arch}
Signed-By: ${apt_keyrings_dir}/docker.asc
EOF
		if [ -e "${apt_sources_dir}/docker.sources" ] || [ -e "${apt_sources_dir}/docker.list" ]; then
			fail "an existing Docker APT source is present but does not point to the expected official repository; review it manually, then rerun bootstrap"
		fi
		run_privileged install -m 0644 "$docker_sources" "${apt_sources_dir}/docker.sources" || fail "could not install Docker's APT source configuration"
	fi

	run_privileged apt-get update || fail "APT update failed; Docker prerequisites were not installed"
	case "$install_mode" in
		compose) set -- docker-compose-plugin ;;
		cli_compose) set -- docker-ce-cli docker-compose-plugin ;;
		engine_compose) set -- docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin ;;
		*) fail "internal error: unknown Docker package install mode" ;;
	esac
	run_privileged apt-get -s install "$@" > "$apt_plan" 2>&1 || fail "APT could not calculate a safe installation plan; review the package conflict and rerun bootstrap"
	if ! apt_plan_is_safe "$apt_plan"; then
		fail "APT's installation plan would remove packages or replace existing Docker Engine components; review it manually and rerun bootstrap"
	fi
	run_privileged apt-get install -y "$@" || fail "APT failed while installing ${missing_description}; bootstrap stopped"

	if [ "$install_mode" = compose ]; then
		docker info >/dev/null 2>&1 || fail "Docker daemon is inaccessible after installing the Compose plugin; inspect the Docker service and socket permissions"
	elif [ "$install_mode" = cli_compose ]; then
		if ! docker info >/dev/null 2>&1 && ! { [ "$current_uid" -ne 0 ] && sudo docker info >/dev/null 2>&1; }; then
			fail "Docker CLI and Compose were installed but the existing Docker daemon is inaccessible; inspect its service and socket permissions"
		fi
	elif ! docker info >/dev/null 2>&1 && ! { [ "$current_uid" -ne 0 ] && sudo docker info >/dev/null 2>&1; }; then
		if command -v systemctl >/dev/null 2>&1 && ! run_privileged systemctl is-active --quiet docker.service; then
			run_privileged systemctl start docker.service || fail "Docker was installed but docker.service could not be started"
		fi
		wait_for_new_docker_daemon || fail "Docker Engine was installed but docker info did not become available; inspect docker.service and its logs"
	fi

	if [ "$current_uid" -ne 0 ] && ! docker info >/dev/null 2>&1; then
		if ! getent group docker >/dev/null 2>&1; then
			run_privileged groupadd --system docker || fail "Docker was installed, but the docker group could not be created"
		fi
		login_user=${SUDO_USER:-${USER:-}}
		[ -n "$login_user" ] || fail "Docker was installed, but the current login user could not be determined for Docker group access"
		run_privileged usermod -aG docker "$login_user" || fail "Docker was installed, but the current user could not be added to the docker group"
		command -v sg >/dev/null 2>&1 || fail "Docker was installed and ${login_user} was added to the docker group; log out and back in, then rerun bootstrap because this shell cannot refresh group membership"
		if ! sg docker -c 'docker info' >/dev/null 2>&1; then
			fail "Docker was installed and ${login_user} was added to the docker group; log out and back in, then rerun bootstrap to refresh group membership"
		fi
		STEALTH_BOOTSTRAP_GROUP_CONTEXT=1
		export STEALTH_BOOTSTRAP_GROUP_CONTEXT
	fi
	rm -rf "$temporary_dir"
	temporary_dir=""
}

inspect_and_prepare_docker() {
	current_uid=$(id -u 2>/dev/null) || fail "could not determine the current UID"
	case "$current_uid" in ''|*[!0-9]*) fail "could not determine the current UID" ;; esac
	if has_controlling_tty; then controlling_tty=yes; else controlling_tty=no; fi
	command_report=""
	missing_bootstrap_commands=""
	for required_command in curl tar awk sha256sum; do
		if command -v "$required_command" >/dev/null 2>&1; then
			command_state=present
		else
			command_state=missing
			missing_bootstrap_commands="${missing_bootstrap_commands} ${required_command}"
		fi
		command_report="${command_report}${required_command}=${command_state} "
	done

	distribution_id=$(read_os_release_value ID)
	distribution_version=$(read_os_release_value VERSION_ID)
	distribution_codename=$(read_os_release_value VERSION_CODENAME)
	if [ -z "$distribution_codename" ]; then distribution_codename=$(read_os_release_value UBUNTU_CODENAME); fi
	printf 'Detected host: OS=%s version=%s architecture=%s UID=%s TTY=%s commands=%s\n' \
		"${distribution_id:-unknown}" "${distribution_version:-unknown}" "$arch_name" "$current_uid" "$controlling_tty" "$command_report" >&2

	docker_arch=$asset_arch
	if command -v docker >/dev/null 2>&1; then
		docker_cli_present=yes
		docker_engine_present=yes
		if docker --version >/dev/null 2>&1; then docker_cli_works=yes; else docker_cli_works=no; fi
		compose_output=$(docker compose version 2>&1) && compose_command_works=yes || compose_command_works=no
		if command -v awk >/dev/null 2>&1; then
			compose_major=$(printf '%s\n' "$compose_output" | awk '{ for (i=1; i<=NF; i++) if ($i ~ /^v?[0-9]+\./) { v=$i; sub(/^v/, "", v); split(v, parts, "."); print parts[1]; exit } }')
			case "$compose_major" in ''|*[!0-9]*) compose_v2=no ;; *) if [ "$compose_major" -ge 2 ]; then compose_v2=yes; else compose_v2=no; fi ;; esac
		else
			compose_v2=no
		fi
		if docker info >/dev/null 2>&1; then docker_daemon_accessible=yes; else docker_daemon_accessible=no; fi
	else
		docker_cli_present=no
		if docker_engine_installed; then docker_engine_present=yes; else docker_engine_present=no; fi
		docker_cli_works=no
		compose_command_works=no
		compose_v2=no
		docker_daemon_accessible=no
	fi
	printf 'Docker state: engine=%s cli=%s compose_plugin=%s daemon_accessible=%s\n' \
		"$docker_engine_present" "$docker_cli_present" "${compose_v2:-no}" "$docker_daemon_accessible" >&2
	if [ -n "$missing_bootstrap_commands" ]; then
		missing_bootstrap_commands=${missing_bootstrap_commands# }
		fail "missing bootstrap commands: ${missing_bootstrap_commands}; install them and rerun bootstrap"
	fi

	if [ "$docker_engine_present" = yes ] && [ "$docker_cli_present" = yes ] && [ "$docker_cli_works" = no ]; then
		fail "Docker is installed but docker --version failed; inspect the Docker CLI before rerunning bootstrap"
	fi
	if [ "$docker_engine_present" = yes ] && [ "$docker_cli_present" = yes ] && [ "$docker_daemon_accessible" = no ]; then
		fail "Docker is installed but the daemon is inaccessible; bootstrap will not replace Docker or change daemon settings. Check docker info, service state, and socket permissions"
	fi
	if [ "$docker_engine_present" = yes ] && [ "$docker_cli_present" = yes ] && [ "$compose_v2" = yes ] && [ "$docker_daemon_accessible" = yes ]; then
		return 0
	fi
	if [ "$docker_engine_present" = yes ] && [ "$docker_cli_present" = no ]; then
		install_docker_prerequisites "Docker CLI and Docker Compose v2 plugin" cli_compose
	elif [ "$docker_engine_present" = yes ]; then
		install_docker_prerequisites "Docker Compose v2 plugin" compose
	else
		install_docker_prerequisites "Docker Engine and Docker Compose v2 plugin" engine_compose
	fi

	printf '%s\n' 'Verifying installed Docker prerequisites:' >&2
	docker --version >&2 || fail "Docker was installed but docker --version could not be verified"
	compose_output=$(docker compose version 2>&1) || fail "Docker was installed but the Docker Compose v2 plugin could not be verified"
	printf '%s\n' "$compose_output" >&2
	compose_major=$(printf '%s\n' "$compose_output" | awk '{ for (i=1; i<=NF; i++) if ($i ~ /^v?[0-9]+\./) { v=$i; sub(/^v/, "", v); split(v, parts, "."); print parts[1]; exit } }')
	case "$compose_major" in ''|*[!0-9]*) fail "Docker Compose plugin version could not be parsed" ;; esac
	[ "$compose_major" -ge 2 ] || fail "Docker Compose v2 or newer is required"
	if [ -n "${STEALTH_BOOTSTRAP_GROUP_CONTEXT:-}" ]; then
		sg docker -c 'docker info' >/dev/null 2>&1 || fail "Docker daemon is inaccessible in the refreshed docker group context"
	else
		docker info >/dev/null 2>&1 || fail "Docker daemon is inaccessible after prerequisite installation"
	fi
	printf '%s\n' 'Docker daemon is accessible.' >&2
}

cleanup() {
	if [ -n "$temporary_dir" ] && [ -d "$temporary_dir" ]; then
		rm -rf "$temporary_dir"
	fi
}
trap cleanup EXIT

version="${STEALTH_VERSION:-}"
explicit_version=0
[ -n "$version" ] && explicit_version=1
while [ "$#" -gt 0 ]; do
	case "$1" in
		--version)
			[ "$#" -ge 2 ] || fail "--version requires a value"
			version="$2"
			explicit_version=1
			shift 2
			;;
		--version=*)
			version=${1#--version=}
			explicit_version=1
			shift
			;;
		-h|--help)
			printf '%s\n' 'Usage: bootstrap.sh [--version vMAJOR.MINOR.PATCH[(-rc.N)]]'
			exit 0
			;;
		*)
			fail "unknown option: $1"
			;;
	esac
done

os_name="$(uname -s 2>/dev/null || true)"
arch_name="$(uname -m 2>/dev/null || true)"
[ "$os_name" = "Linux" ] || fail "unsupported operating system: ${os_name:-unknown}; Linux is supported"

case "$arch_name" in
	x86_64|amd64)
		asset="stealth_Linux_x86_64.tar.gz"
		asset_arch=amd64
		;;
	aarch64|arm64)
		asset="stealth_Linux_arm64.tar.gz"
		asset_arch=arm64
		;;
	*)
		fail "unsupported architecture: ${arch_name:-unknown}; use Linux amd64 or arm64"
		;;
esac

if [ -z "$version" ]; then
	latest_url="$(curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
		--output /dev/null --write-out '%{url_effective}' \
		"https://github.com/${repository}/releases/latest")" || fail "could not resolve the latest stable release"
	while [ "$latest_url" != "${latest_url%/}" ]; do
		latest_url=${latest_url%/}
	done
	case "$latest_url" in
		"https://github.com/${repository}/releases/tag/"*)
			tag=${latest_url##*/releases/tag/}
			case "$tag" in
				''|*/*|*\?*|*#*) fail "failed to determine latest release" ;;
			esac
			version=$tag
			;;
		"https://github.com/${repository}/releases"|"https://github.com/${repository}/releases/latest")
			fail "no stable GitHub release is available yet"
			;;
		*)
			fail "failed to determine latest release"
			;;
	esac
fi

version_error="release version must match vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N: $version"
case "$version" in
	v*) ;;
	*) fail "$version_error" ;;
esac
version_parts=${version#v}
release_candidate_number=""
case "$version_parts" in
	*-rc.*)
		stable_version_parts=${version_parts%%-rc.*}
		release_candidate_number=${version_parts#*-rc.}
		;;
	*-*)
		fail "$version_error"
		;;
	*)
		stable_version_parts=$version_parts
		;;
esac
case "$stable_version_parts" in
	*.*.*) ;;
	*) fail "$version_error" ;;
esac
major=${stable_version_parts%%.*}
version_remainder=${stable_version_parts#*.}
minor=${version_remainder%%.*}
patch_version=${version_remainder#*.}
case "$major:$minor:$patch_version" in
	''|*[!0-9:]*|*:*:*:*) fail "$version_error" ;;
esac
if [ -n "$release_candidate_number" ]; then
	case "$release_candidate_number" in
		''|*[!0-9]*) fail "$version_error" ;;
		*) ;;
	esac
fi
if [ "$explicit_version" -eq 0 ] && [ -n "$release_candidate_number" ]; then
	fail "latest GitHub release is not stable"
fi

inspect_and_prepare_docker

temporary_dir="$(mktemp -d "${tmp_root%/}/stealth-bootstrap.XXXXXX")" || fail "could not create a temporary directory"
archive="${temporary_dir}/${asset}"
checksums="${temporary_dir}/checksums.txt"

curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
	"${release_root}/${version}/${asset}" -o "$archive" || fail "could not download ${asset} for ${version}"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
	"${release_root}/${version}/checksums.txt" -o "$checksums" || fail "could not download release checksums"

expected="$(awk -v wanted="$asset" '$2 == wanted { print $1; exit }' "$checksums")"
case "$expected" in
	'') fail "checksum for ${asset} is missing from checksums.txt" ;;
	*[!0-9a-fA-F]*) fail "checksum for ${asset} is invalid" ;;
	*) ;;
esac
[ "${#expected}" -eq 64 ] || fail "checksum for ${asset} is invalid"
actual="$(sha256sum "$archive" | awk '{print $1}')"
[ "$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')" = "$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')" ] || fail "download verification failed"

extract_dir="${temporary_dir}/extract"
mkdir -p "$extract_dir"
tar -xzf "$archive" -C "$extract_dir" stealth || fail "release archive does not contain the stealth binary"
[ -f "$extract_dir/stealth" ] || fail "release archive did not produce a stealth binary"

user_home="${HOME:-}"
[ -n "$user_home" ] || fail 'HOME is not set; choose a writable user home before installing'

if [ -n "${STEALTH_BIN_DIR:-}" ]; then
	bin_dir="$STEALTH_BIN_DIR"
	mkdir -p "$bin_dir" 2>/dev/null || fail "cannot create STEALTH_BIN_DIR: $bin_dir"
else
	bin_dir="${user_home%/}/.local/bin"
	if ! mkdir -p "$bin_dir" 2>/dev/null || [ ! -w "$bin_dir" ]; then
		bin_dir="${user_home%/}/.stealth/bin"
		mkdir -p "$bin_dir" 2>/dev/null || fail "no writable CLI install directory under $user_home"
	fi
fi

temporary_binary="${bin_dir}/.stealth.$$"
cp "$extract_dir/stealth" "$temporary_binary" || fail "could not install the CLI binary"
chmod 0755 "$temporary_binary" || fail "could not make the CLI executable"
mv -f "$temporary_binary" "${bin_dir}/stealth" || fail "could not activate the CLI binary"

printf 'Stealth CLI %s installed at %s/stealth\n' "$version" "$bin_dir"
case ":${PATH:-}:" in
	*":${bin_dir}:"*) ;;
	*) printf 'Add %s to PATH to call `stealth` directly.\n' "$bin_dir" ;;
esac

cleanup
temporary_dir=""
if [ -t 1 ] && [ -t 2 ]; then
	if [ -n "${STEALTH_BOOTSTRAP_GROUP_CONTEXT:-}" ]; then
		export STEALTH_INSTALL_BINARY="${bin_dir}/stealth"
		exec sg docker -c 'exec "$STEALTH_INSTALL_BINARY" install'
	fi
	exec "${bin_dir}/stealth" install
fi
fail 'interactive setup requires a TTY; run the installed CLI from a terminal with `stealth install`'
