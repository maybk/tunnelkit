// Package fortivpn defines the public configuration, event, prompt, and error
// types for the embedded Fortinet SSL-VPN client library.
//
// Runtime execution (session orchestration, PPP negotiation, TUN device
// management, and host network configuration) lives in subpackage
// tunnelkit/fortivpn/engine so applications that only manipulate configurations
// or consume events do not link low-level networking dependencies.
package fortivpn
