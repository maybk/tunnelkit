package sslvpn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"

	"tunnelkit/fortivpn"
)

// TunnelConfig holds the client VPN parameters parsed from /remote/fortisslvpn_xml
// (or fallback /remote/fortisslvpn).
type TunnelConfig struct {
	AssignedIP             netip.Addr
	DNS                    []netip.Addr
	DNSDomains             []string
	SplitRoutes            []netip.Prefix
	ExcludeRoutes          []netip.Prefix
	IdleTimeout            int
	AuthTimeout            int
	FortiOS                string
	ReconnectWithoutReauth bool
	RawXML                 []byte
}

// FetchTunnelConfig retrieves and parses /remote/fortisslvpn_xml (with fallback to /remote/fortisslvpn).
func (c *Client) FetchTunnelConfig(ctx context.Context, cookie string) (TunnelConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+PathConfigXML, nil)
	if err != nil {
		return TunnelConfig{}, fortivpn.WrapError(fortivpn.CodeGatewayUnreachable, "failed to create tunnel config request", err)
	}
	c.setCommonHeaders(req)
	req.Header.Set("Cookie", CookieName+"="+cookie)

	resp, err := c.hc.Do(req)
	if err != nil {
		return TunnelConfig{}, c.classifyTransportError(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return TunnelConfig{}, ctxErr
	}
	if readErr != nil {
		return TunnelConfig{}, c.classifyTransportError(readErr)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return TunnelConfig{}, fortivpn.NewError(fortivpn.CodeTunnelDenied, fmt.Sprintf("gateway denied tunnel configuration with HTTP %d", resp.StatusCode))
	}
	if resp.StatusCode == http.StatusNotFound {
		return c.fetchLegacyConfig(ctx, cookie)
	}
	if resp.StatusCode != http.StatusOK {
		return TunnelConfig{}, fortivpn.NewError(fortivpn.CodeTunnelDenied, fmt.Sprintf("unexpected HTTP %d from fortisslvpn_xml", resp.StatusCode))
	}

	if c.configXMLHook != nil {
		c.configXMLHook(RedactConfigXML(body))
	}
	cfg, err := ParseConfigXML(body)
	if err != nil {
		if legacyCfg, legacyErr := c.fetchLegacyConfig(ctx, cookie); legacyErr == nil && len(legacyCfg.SplitRoutes) > 0 {
			return legacyCfg, nil
		}
		return TunnelConfig{}, fortivpn.WrapError(fortivpn.CodeTunnelDenied, "failed to parse tunnel XML configuration", err)
	}
	return cfg, nil
}

func (c *Client) fetchLegacyConfig(ctx context.Context, cookie string) (TunnelConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+PathConfigLegacy, nil)
	if err != nil {
		return TunnelConfig{}, err
	}
	c.setCommonHeaders(req)
	req.Header.Set("Cookie", CookieName+"="+cookie)

	resp, err := c.hc.Do(req)
	if err != nil {
		return TunnelConfig{}, c.classifyTransportError(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		return TunnelConfig{}, c.classifyTransportError(readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return TunnelConfig{}, fortivpn.NewError(fortivpn.CodeTunnelDenied, "SSL-VPN tunnel mode is not available on this gateway")
	}
	return ParseLegacyConfigHTML(body)
}
