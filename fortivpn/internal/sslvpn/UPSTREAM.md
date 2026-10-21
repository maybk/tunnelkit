# Upstream Attribution (`fortivpn/internal/sslvpn`)

Portions of `frame.go`, `auth.go`, and `tunconf.go` (6-byte `0x5050` frame
encoding/decoding, `/remote/logincheck` form and `ret=2` challenge handling,
and `<sslvpn-tunnel>` XML structure) are adapted from:

- **Repository**: [xen0bit/veepin](https://github.com/xen0bit/veepin) (`internal/fortinet`)
- **Commit**: `bcac1789f0a7eaa7687b2777d6d6765fb87158e0`
- **License**: MIT License (Copyright (c) 2026 Remy)
