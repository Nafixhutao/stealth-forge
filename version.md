# Stealth v0.2.6 Beta 5

This guide installs prerelease `v0.2.6-rc.5`. The bootstrap selects stable
releases by default, so pin this version to install the Beta.

## Install on a VPS

Run this block in the same interactive Bash or SSH session. The URL is built
from separate parts so it is not accidentally split at `/stealth/`:

```sh
bootstrap_base='https://raw.githubusercontent.com/Nafixhutao/stealth-forge/'
bootstrap_ref='main'
bootstrap_url="${bootstrap_base}${bootstrap_ref}/scripts/bootstrap.sh"
curl -fsSL "$bootstrap_url" -o /tmp/stealth-bootstrap.sh &&
  STEALTH_VERSION=v0.2.6-rc.5 sh /tmp/stealth-bootstrap.sh
```

The `&&` ensures the installer runs only after the bootstrap script downloads
successfully. Keep the commands in the same shell so the URL variables remain
available.

Automatic Docker Engine and Compose v2 installation is supported on Ubuntu
22.04, 24.04, and 26.04, and Debian 12 and 13, on Linux amd64 and arm64. If
Docker and Compose already work, bootstrap continues without reinstalling
them. If prerequisites are missing, root installs them automatically; a
non-root user authenticates through the normal `sudo` prompt. Stealth never
reads, receives, stores, or logs the sudo password.

The interactive `stealth install` wizard starts after bootstrap verifies and
installs the CLI. If bootstrap reports that a TTY is required, open an
interactive terminal and run `stealth install` there.

After setup, check the installation with:

```sh
stealth status
stealth doctor
```

See the [Beta release](https://github.com/Nafixhutao/stealth-forge/releases/tag/v0.2.6-rc.5)
and the [CLI guide](docs/cli.md) for more detail.
