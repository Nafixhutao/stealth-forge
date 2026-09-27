#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
	printf 'usage: %s start RUN_SUFFIX OUTPUT_ENV | stop RUN_SUFFIX\n' "$(basename -- "$0")" >&2
	exit 2
}

action="${1:-}"
run_suffix="${2:-}"
if [[ ! "$run_suffix" =~ ^[0-9]+-[0-9]+$ ]]; then usage; fi
runner_temp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
vm_root="$runner_temp/stealth-app-host-reboot-vm-$run_suffix"
pid_file="$vm_root/qemu.pid"

case "$action" in
	start)
		output_env="${3:-}"
		if [[ ! "$output_env" = "$runner_temp/"* ]] || [ -e "$output_env" ] || [ -L "$output_env" ]; then usage; fi
		if [ -e "$vm_root" ] || [ -L "$vm_root" ]; then
			printf 'refusing existing ephemeral VM directory: %s\n' "$vm_root" >&2
			exit 2
		fi
		if [ ! -c /dev/kvm ] || ! sudo -n test -r /dev/kvm -a -w /dev/kvm; then
			printf '%s\n' 'GitHub-hosted runner has no usable KVM device; use the apps-host-reboot SSH mode with a disposable Linux VM.' >&2
			exit 2
		fi
		sudo -n apt-get update
		sudo -n DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends cloud-image-utils qemu-system-x86 qemu-utils
		install -d -m 0700 "$vm_root"
		cloud_image="$vm_root/noble-server-cloudimg-amd64.img"
		image_sums="$vm_root/SHA256SUMS"
		user_data="$vm_root/user-data"
		seed_image="$vm_root/cloud-init.iso"
		guest_disk="$vm_root/guest.qcow2"
		ssh_key="$vm_root/ssh-key"
		known_hosts="$vm_root/known-hosts"
		serial_log="$vm_root/serial.log"
		qemu_log="$vm_root/qemu.log"
		ssh_port=2222
		base_url='https://cloud-images.ubuntu.com/noble/current'
		curl --fail --location --silent --show-error --retry 3 "$base_url/SHA256SUMS" --output "$image_sums"
		curl --fail --location --silent --show-error --retry 3 "$base_url/noble-server-cloudimg-amd64.img" --output "$cloud_image"
		(
			cd "$vm_root"
			grep -E '^[0-9a-f]{64}[[:space:]]+\*?noble-server-cloudimg-amd64\.img$' SHA256SUMS | sha256sum --check --status
		)
		ssh-keygen -q -t ed25519 -N '' -C "stealth-host-reboot-$run_suffix" -f "$ssh_key"
		public_key="$(cat "$ssh_key.pub")"
		cat >"$user_data" <<EOF
#cloud-config
users:
  - default
  - name: acceptance
    shell: /bin/bash
    groups: [adm, sudo]
    sudo: ['ALL=(ALL) NOPASSWD:ALL']
    ssh_authorized_keys:
      - $public_key
package_update: true
packages:
  - apparmor
  - apparmor-utils
  - curl
  - docker-compose-v2
  - docker.io
  - python3
runcmd:
  - [bash, -lc, 'usermod -aG docker acceptance']
  - [systemctl, enable, --now, docker]
EOF
		chmod 0600 "$user_data"
		cloud-localds "$seed_image" "$user_data"
		qemu-img create -q -f qcow2 -F qcow2 -b "$cloud_image" "$guest_disk" 48G
		chmod 0600 "$guest_disk" "$seed_image"
		if python3 - "$ssh_port" <<'PY'
import socket, sys
with socket.socket() as sock:
    try: sock.bind(("127.0.0.1", int(sys.argv[1])))
    except OSError: raise SystemExit(1)
PY
		then :; else
			printf 'ephemeral VM SSH port is already in use: %s\n' "$ssh_port" >&2
			exit 2
		fi
		sudo -n qemu-system-x86_64 \
			-enable-kvm -machine q35,accel=kvm -cpu host -smp 4 -m 10G \
			-drive "file=$guest_disk,if=virtio,format=qcow2" \
			-drive "file=$seed_image,if=virtio,media=cdrom,readonly=on" \
			-netdev "user,id=net0,hostfwd=tcp:127.0.0.1:$ssh_port-:22" \
			-device virtio-net-pci,netdev=net0 \
			-display none -serial "file:$serial_log" -monitor none \
			-D "$qemu_log" -daemonize -pidfile "$pid_file"
		ssh_options=(-p "$ssh_port" -i "$ssh_key" -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=5)
		ssh_ready=false
		for attempt in $(seq 1 180); do
			if ssh "${ssh_options[@]}" acceptance@127.0.0.1 true >/dev/null 2>&1; then ssh_ready=true; break; fi
			vm_pid="$(sudo -n cat "$pid_file" 2>/dev/null || true)"
			if [[ ! "$vm_pid" =~ ^[0-9]+$ ]] || ! sudo -n kill -0 "$vm_pid" 2>/dev/null; then
				printf '%s\n' 'ephemeral Linux VM stopped during cloud-init boot' >&2
				sudo -n tail -n 80 "$serial_log" "$qemu_log" >&2 || true
				exit 1
			fi
			sleep 5
		done
		if [ "$ssh_ready" != true ]; then
			printf '%s\n' 'ephemeral Linux VM did not enable SSH within 15 minutes' >&2
			sudo -n tail -n 80 "$serial_log" "$qemu_log" >&2 || true
			exit 1
		fi
		known_hosts="$vm_root/known-hosts"
		ssh-keyscan -T 10 -p "$ssh_port" -t ed25519 127.0.0.1 2>/dev/null >"$known_hosts"
		if ! ssh-keygen -F "[127.0.0.1]:$ssh_port" -f "$known_hosts" >/dev/null; then
			printf '%s\n' 'failed to pin the ephemeral VM SSH host key' >&2
			exit 1
		fi
		chmod 0600 "$known_hosts"
		ssh_options=(-p "$ssh_port" -i "$ssh_key" -o UserKnownHostsFile="$known_hosts" -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=15)
		ssh "${ssh_options[@]}" acceptance@127.0.0.1 'cloud-init status --wait && sudo -n true && docker compose version'
		ssh "${ssh_options[@]}" acceptance@127.0.0.1 'id -nG | tr " " "\n" | grep -Fx docker >/dev/null && docker info >/dev/null'
		{
			printf 'SSH_KEY_FILE=%s\n' "$ssh_key"
			printf 'KNOWN_HOSTS_FILE=%s\n' "$known_hosts"
			printf 'SSH_TARGET=acceptance@127.0.0.1\n'
			printf 'SSH_PORT=%s\n' "$ssh_port"
			printf 'REBOOT_HOST_KIND=github-actions-kvm-vm\n'
		} >"$output_env"
		chmod 0600 "$output_env"
		;;
	stop)
		if [ -e "$pid_file" ] && [ ! -L "$pid_file" ]; then
			pid="$(sudo -n cat "$pid_file")"
			if [[ "$pid" =~ ^[0-9]+$ ]] && sudo -n kill -0 "$pid" 2>/dev/null; then
				process_args="$(sudo -n ps -p "$pid" -o args= 2>/dev/null || true)"
				if [[ "$process_args" != *qemu-system-x86_64* || "$process_args" != *"$pid_file"* ]]; then
					printf 'refusing to stop unexpected process recorded in %s\n' "$pid_file" >&2
					exit 1
				fi
				sudo -n kill -TERM "$pid"
				for _ in $(seq 1 30); do
					if ! sudo -n kill -0 "$pid" 2>/dev/null; then break; fi
					sleep 1
				done
				if sudo -n kill -0 "$pid" 2>/dev/null; then sudo -n kill -KILL "$pid"; fi
			fi
		fi
		if [ -e "$vm_root" ] || [ -L "$vm_root" ]; then
			case "$vm_root" in "$runner_temp"/stealth-app-host-reboot-vm-[0-9]*-[0-9]*) rm -rf -- "$vm_root" ;; *) printf 'refusing unexpected ephemeral VM cleanup root: %s\n' "$vm_root" >&2; exit 1 ;; esac
		fi
		;;
	*) usage ;;
esac
