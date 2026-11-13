#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 4 ]; then
  echo "Usage: $0 <version> <sha256sums-file> [output-dir] [release-date]" >&2
  exit 1
fi

version="${1#v}"
sums_file="$2"
out_dir="${3:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
release_date="${4:-}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Version must match MAJOR.MINOR.PATCH: $version" >&2
  exit 1
fi

if [ -n "$release_date" ] && [[ ! "$release_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "Release date must match YYYY-MM-DD: $release_date" >&2
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

  on_macos do
    url "https://github.com/maybk/tunnelkit/releases/download/v#{version}/tunnelkit-#{version}-macos-#{arch}.dmg"

    depends_on macos: :monterey

    app "TunnelKit.app"

    postflight_steps do
      run "/usr/bin/xattr",
          args:           ["-rd", "com.apple.quarantine", "{{appdir}}/TunnelKit.app"],
          writable_paths: ["TunnelKit.app"],
          writable_base:  :appdir
    end

    uninstall quit: "com.tunnelkit.app"

    zap delete: [
          "/Library/Application Support/TunnelKit",
          "/Library/PrivilegedHelperTools/com.tunnelkit.helper",
        ],
        trash:  [
          "~/.tunnelkit",
          "~/Library/Application Support/com.tunnelkit.app",
          "~/Library/Caches/com.tunnelkit.app",
          "~/Library/Preferences/com.tunnelkit.app.plist",
          "~/Library/Saved Application State/com.tunnelkit.app.savedState",
        ]
  end
  on_linux do
    url "https://github.com/maybk/tunnelkit/releases/download/v#{version}/tunnelkit-#{version}-linux-#{arch}.tar.gz"

    binary "tunnelkit"
    binary "tunnelkit-helper"

    zap delete: [
          "/usr/libexec/tunnelkit",
          "/var/lib/tunnelkit",
        ],
        trash:  "~/.tunnelkit"
  end

  name "TunnelKit"
  desc "Fortinet SSL-VPN desktop and menu bar client"
  homepage "https://github.com/maybk/tunnelkit"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true
end
EOF

jq --indent 4 -n \
  --arg version "$version" \
  --arg amd64_sha "$windows_amd64_sha" \
  --arg arm64_sha "$windows_arm64_sha" \
  '{
    version: $version,
    description: "Fortinet SSL-VPN desktop and menu bar client",
    homepage: "https://github.com/maybk/tunnelkit",
    license: "MIT",
    architecture: {
      "64bit": {
        url: ("https://github.com/maybk/tunnelkit/releases/download/v" + $version + "/tunnelkit-" + $version + "-windows-amd64.zip"),
        hash: $amd64_sha
      },
      "arm64": {
        url: ("https://github.com/maybk/tunnelkit/releases/download/v" + $version + "/tunnelkit-" + $version + "-windows-arm64.zip"),
        hash: $arm64_sha
      }
    },
    pre_uninstall: [
      "Get-Process -Name '\''tunnelkit'\'' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue",
      "if ($cmd -eq '\''uninstall'\'' -and (Get-Service -Name '\''TunnelKitHelper'\'' -ErrorAction SilentlyContinue) -and (Test-Path \"$dir\\tunnelkit-helper.exe\")) {",
      "    $proc = Start-Process \"$dir\\tunnelkit-helper.exe\" -ArgumentList '\''uninstall-service'\'' -Wait -PassThru -WindowStyle Hidden",
      "    if ($proc.ExitCode -ne 0) {",
      "        warn \"Failed to remove '\''TunnelKitHelper'\'' service automatically. Run as Administrator: sc.exe stop TunnelKitHelper; sc.exe delete TunnelKitHelper\"",
      "    }",
      "}"
    ],
    shortcuts: [
      [
        "tunnelkit.exe",
        "TunnelKit"
      ]
    ],
    checkver: "github",
    autoupdate: {
      architecture: {
        "64bit": {
          url: "https://github.com/maybk/tunnelkit/releases/download/v$version/tunnelkit-$version-windows-amd64.zip"
        },
        "arm64": {
          url: "https://github.com/maybk/tunnelkit/releases/download/v$version/tunnelkit-$version-windows-arm64.zip"
        }
      },
      hash: {
        url: "$baseurl/SHA256SUMS.txt"
      }
    }
  }' > "$out_dir/bucket/tunnelkit.json"

if [[ "$windows_amd64_setup_sha" =~ ^[0-9a-f]{64}$ ]] && [[ "$windows_arm64_setup_sha" =~ ^[0-9a-f]{64}$ ]]; then
  winget_dir="$out_dir/winget/manifests/m/maybk/TunnelKit/${version}"
  mkdir -p "$winget_dir"
  amd64_upper="$(printf '%s' "$windows_amd64_setup_sha" | tr '[:lower:]' '[:upper:]')"
  arm64_upper="$(printf '%s' "$windows_arm64_setup_sha" | tr '[:lower:]' '[:upper:]')"

  cat > "$winget_dir/maybk.TunnelKit.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.1.28.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.28.0
EOF

  {
    cat <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.1.28.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
InstallerType: nullsoft
Scope: machine
UpgradeBehavior: install
ProductCode: maybkTunnelKit
ElevationRequirement: elevatesSelf
EOF
    if [ -n "$release_date" ]; then
      printf 'ReleaseDate: %s\n' "$release_date"
    fi
    cat <<EOF
InstallationMetadata:
  DefaultInstallLocation: '%ProgramFiles%\maybk\TunnelKit'
Installers:
- Architecture: x64
  InstallerUrl: https://github.com/maybk/tunnelkit/releases/download/v${version}/tunnelkit-${version}-windows-amd64-setup.exe
  InstallerSha256: ${amd64_upper}
- Architecture: arm64
  InstallerUrl: https://github.com/maybk/tunnelkit/releases/download/v${version}/tunnelkit-${version}-windows-arm64-setup.exe
  InstallerSha256: ${arm64_upper}
ManifestType: installer
ManifestVersion: 1.28.0
EOF
  } > "$winget_dir/maybk.TunnelKit.installer.yaml"

  cat > "$winget_dir/maybk.TunnelKit.locale.en-US.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.1.28.0.schema.json

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
ShortDescription: Fortinet SSL-VPN desktop and menu bar client
Tags:
- fortigate
- fortinet
- network
- sslvpn
- vpn
ReleaseNotesUrl: https://github.com/maybk/tunnelkit/releases/tag/v${version}
ManifestType: defaultLocale
ManifestVersion: 1.28.0
EOF

  cat > "$winget_dir/maybk.TunnelKit.locale.zh-CN.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.locale.1.28.0.schema.json

PackageIdentifier: maybk.TunnelKit
PackageVersion: ${version}
PackageLocale: zh-CN
PackageName: TunnelKit
ShortDescription: 简洁轻量的跨平台 Fortinet SSL-VPN 桌面与菜单栏客户端。
Tags:
- fortigate
- fortinet
- sslvpn
- vpn
- 网络
ManifestType: locale
ManifestVersion: 1.28.0
EOF
fi
