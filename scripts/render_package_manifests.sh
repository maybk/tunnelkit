#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo "Usage: $0 <version> <sha256sums-file> [output-dir]" >&2
  exit 1
fi

version="${1#v}"
sums_file="$2"
out_dir="${3:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Version must match MAJOR.MINOR.PATCH: $version" >&2
  exit 1
fi

if [ ! -f "$sums_file" ]; then
  echo "SHA256SUMS file does not exist: $sums_file" >&2
  exit 1
fi

extract_sum() {
  local asset="$1"
  local sum
  sum="$(
    awk -v target="$asset" '
      NF >= 2 {
        file = $2
        sub(/^\*/, "", file)
        if (file == target) {
          print tolower($1)
          exit
        }
      }
    ' "$sums_file"
  )"
  if [[ ! "$sum" =~ ^[0-9a-f]{64}$ ]]; then
    echo "Missing or invalid SHA-256 for $asset in $sums_file" >&2
    exit 1
  fi
  printf '%s' "$sum"
}

extract_optional_sum() {
  local asset="$1"
  awk -v target="$asset" '
    NF >= 2 {
      file = $2
      sub(/^\*/, "", file)
      if (file == target) {
        print tolower($1)
        exit
      }
    }
  ' "$sums_file"
}

macos_arm64_sha="$(extract_sum "tunnelkit-${version}-macos-arm64.dmg")"
macos_amd64_sha="$(extract_sum "tunnelkit-${version}-macos-amd64.dmg")"
linux_arm64_sha="$(extract_sum "tunnelkit-${version}-linux-arm64.tar.gz")"
linux_amd64_sha="$(extract_sum "tunnelkit-${version}-linux-amd64.tar.gz")"
windows_amd64_sha="$(extract_sum "tunnelkit-${version}-windows-amd64.zip")"
windows_arm64_sha="$(extract_sum "tunnelkit-${version}-windows-arm64.zip")"
windows_amd64_setup_sha="$(extract_optional_sum "tunnelkit-${version}-windows-amd64-setup.exe")"
windows_arm64_setup_sha="$(extract_optional_sum "tunnelkit-${version}-windows-arm64-setup.exe")"

mkdir -p "$out_dir/Casks" "$out_dir/bucket"

cat > "$out_dir/Casks/tunnelkit.rb" <<EOF
cask "tunnelkit" do
  arch arm: "arm64", intel: "amd64"

  version "${version}"
  sha256 arm:          "${macos_arm64_sha}",
         intel:        "${macos_amd64_sha}",
         arm64_linux:  "${linux_arm64_sha}",
         x86_64_linux: "${linux_amd64_sha}"

  name "TunnelKit"
  desc "Fortinet SSL-VPN desktop and menu bar client"
  homepage "https://github.com/maybk/tunnelkit"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true

  on_macos do
    depends_on macos: ">= :big_sur"

    url "https://github.com/maybk/tunnelkit/releases/download/v#{version}/tunnelkit-#{version}-macos-#{arch}.dmg",
        verified: "github.com/maybk/tunnelkit/"

    app "tunnelkit.app"

    postflight_steps do
      system_command "/usr/bin/xattr",
                     args: ["-rd", "com.apple.quarantine", "{{appdir}}/tunnelkit.app"],
                     writable_paths: ["tunnelkit.app"],
                     writable_base: :appdir
    end

    zap trash: [
      "~/.tunnelkit",
      "~/Library/Application Support/com.tunnelkit.app",
      "~/Library/Caches/com.tunnelkit.app",
      "~/Library/Preferences/com.tunnelkit.app.plist",
      "~/Library/Saved Application State/com.tunnelkit.app.savedState",
    ]
  end

  on_linux do
    url "https://github.com/maybk/tunnelkit/releases/download/v#{version}/tunnelkit-#{version}-linux-#{arch}.tar.gz",
        verified: "github.com/maybk/tunnelkit/"

    binary "tunnelkit"
    binary "tunnelkit-helper"

    zap trash: [
      "~/.tunnelkit",
    ]
  end
end
EOF

cat > "$out_dir/bucket/tunnelkit.json" <<EOF
{
  "version": "${version}",
  "description": "Cross-platform Fortinet SSL-VPN desktop and menu bar client",
  "homepage": "https://github.com/maybk/tunnelkit",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/maybk/tunnelkit/releases/download/v${version}/tunnelkit-${version}-windows-amd64.zip",
      "hash": "${windows_amd64_sha}"
    },
    "arm64": {
      "url": "https://github.com/maybk/tunnelkit/releases/download/v${version}/tunnelkit-${version}-windows-arm64.zip",
      "hash": "${windows_arm64_sha}"
    }
  },
  "bin": "tunnelkit.exe",
  "shortcuts": [
    [
      "tunnelkit.exe",
      "TunnelKit"
    ]
  ],
  "checkver": {
    "github": "https://github.com/maybk/tunnelkit"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/maybk/tunnelkit/releases/download/v\$version/tunnelkit-\$version-windows-amd64.zip"
      },
      "arm64": {
        "url": "https://github.com/maybk/tunnelkit/releases/download/v\$version/tunnelkit-\$version-windows-arm64.zip"
      }
    },
    "hash": {
      "url": "\$baseurl/SHA256SUMS.txt"
    }
  }
}
EOF

if [[ "$windows_amd64_setup_sha" =~ ^[0-9a-f]{64}$ ]] && [[ "$windows_arm64_setup_sha" =~ ^[0-9a-f]{64}$ ]]; then
  winget_dir="$out_dir/winget/manifests/m/maybk/TunnelKit/${version}"
  mkdir -p "$winget_dir"
  amd64_upper="$(printf '%s' "$windows_amd64_setup_sha" | tr '[:lower:]' '[:upper:]')"
  arm64_upper="$(printf '%s' "$windows_arm64_setup_sha" | tr '[:lower:]' '[:upper:]')"

  cat > "$winget_dir/maybk.TunnelKit.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.1.12.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.12.0
EOF

  cat > "$winget_dir/maybk.TunnelKit.installer.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.1.12.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
InstallerType: nullsoft
Scope: machine
UpgradeBehavior: install
ProductCode: maybkTunnelKit
ElevationRequirement: elevatesSelf
Installers:
- Architecture: x64
  InstallerUrl: https://github.com/maybk/tunnelkit/releases/download/v${version}/tunnelkit-${version}-windows-amd64-setup.exe
  InstallerSha256: ${amd64_upper}
- Architecture: arm64
  InstallerUrl: https://github.com/maybk/tunnelkit/releases/download/v${version}/tunnelkit-${version}-windows-arm64-setup.exe
  InstallerSha256: ${arm64_upper}
ManifestType: installer
ManifestVersion: 1.12.0
EOF

  cat > "$winget_dir/maybk.TunnelKit.locale.en-US.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.1.12.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
PackageLocale: en-US
Publisher: maybk
PublisherUrl: https://github.com/maybk
PublisherSupportUrl: https://github.com/maybk/tunnelkit/issues
PackageName: TunnelKit
PackageUrl: https://github.com/maybk/tunnelkit
License: MIT
LicenseUrl: https://github.com/maybk/tunnelkit/blob/HEAD/LICENSE
ShortDescription: Lightweight cross-platform Fortinet SSL-VPN desktop and menu bar client.
Tags:
- fortigate
- fortinet
- network
- sslvpn
- vpn
ReleaseNotesUrl: https://github.com/maybk/tunnelkit/releases/tag/v${version}
ManifestType: defaultLocale
ManifestVersion: 1.12.0
EOF

  cat > "$winget_dir/maybk.TunnelKit.locale.zh-CN.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.locale.1.12.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
PackageLocale: zh-CN
ShortDescription: 简洁轻量的跨平台 Fortinet SSL-VPN 桌面与菜单栏客户端。
Tags:
- fortigate
- fortinet
- sslvpn
- vpn
- 网络
ManifestType: locale
ManifestVersion: 1.12.0
EOF
fi
