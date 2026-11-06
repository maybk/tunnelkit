#!/bin/sh
set -e

# Keep the installed component on upgrade. Package removals require a stopped VPN.
case "${1:-}" in
  upgrade|deconfigure|1) exit 0 ;;
esac
if [ -x /usr/libexec/tunnelkit/tunnelkit-helper ]; then
  /usr/libexec/tunnelkit/tunnelkit-helper uninstall
fi
