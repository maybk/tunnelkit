#!/bin/sh
set -e

# Keep the installed component on upgrade. Package removals require a stopped VPN.
case "${1:-}" in
  upgrade|deconfigure|failed-upgrade|[1-9]) exit 0 ;;
esac
if [ -x /usr/libexec/tunnelkit/tunnelkit-helper ]; then
  /usr/libexec/tunnelkit/tunnelkit-helper uninstall
fi
