package sslvpn

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"tunnelkit/fortivpn"
)

const (
	maxUpgradeHTTPHeader = 16 * 1024
	tunnelReadBufSize    = 16 * 1024
)

// TunnelRequest builds the raw HTTP/1.1 GET request that switches a TLS connection
// into the Fortinet 6-octet 0x5050 framed PPP stream.
func TunnelRequest(host string, port int, cookie, userAgent string) []byte {
	if userAgent == "" {
		userAgent = fortivpn.DefaultUserAgent
	}
	hostHeader := host
	if port > 0 && port != 443 {
		hostHeader = fmt.Sprintf("%s:%d", host, port)
	}
	var b strings.Builder
	b.Grow(256 + len(cookie))
	b.WriteString("GET ")
	b.WriteString(PathTunnel)
	b.WriteString(" HTTP/1.1\r\nHost: ")
	b.WriteString(hostHeader)
	b.WriteString("\r\nUser-Agent: ")
	b.WriteString(userAgent)
	b.WriteString("\r\nAccept: */*\r\nAccept-Encoding: identity\r\nPragma: no-cache\r\nCache-Control: no-store, no-cache, must-revalidate\r\nCookie: ")
	b.WriteString(CookieName)
	b.WriteByte('=')
	b.WriteString(cookie)
	b.WriteString("\r\nContent-Length: 0\r\n\r\n")
	return []byte(b.String())
}

// TunnelConn wraps an upgraded TLS connection, handling any optional HTTP response
// header before transitioning to the 6-octet Fortinet PPP frame stream.
type TunnelConn struct {
	conn           net.Conn
	br             *bufio.Reader
	upgradeChecked bool
	writeMu        sync.Mutex
	writeBuf       []byte
}

// NewTunnelConn wraps an upgraded TLS connection with a buffered reader.
func NewTunnelConn(conn net.Conn) *TunnelConn {
	return &TunnelConn{
		conn:     conn,
		br:       bufio.NewReaderSize(conn, tunnelReadBufSize),
		writeBuf: make([]byte, FrameHeaderLen+2048),
	}
}

// ReadFrame reads the next PPP frame from the tunnel stream.
func (t *TunnelConn) ReadFrame() ([]byte, error) {
	if !t.upgradeChecked {
		if err := t.ensureUpgraded(); err != nil {
			return nil, err
		}
	}
	return ReadFrame(t.br)
}

// ReadFrameInto reads the next PPP frame into scratch when capacity suffices.
func (t *TunnelConn) ReadFrameInto(scratch []byte) ([]byte, error) {
	if !t.upgradeChecked {
		if err := t.ensureUpgraded(); err != nil {
			return nil, err
		}
	}
	return ReadFrameInto(t.br, scratch)
}

// WriteFrame encodes pppFrame with the 6-octet Fortinet header and writes it to the tunnel.
func (t *TunnelConn) WriteFrame(pppFrame []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	needed := FrameHeaderLen + len(pppFrame)
	if cap(t.writeBuf) < needed {
		t.writeBuf = make([]byte, needed)
	}
	buf := t.writeBuf[:needed]
	EncodeFrameInto(buf, pppFrame)
	_, err := t.conn.Write(buf)
	return err
}

// WritePreframed writes a buffer that already starts with the 6-octet Fortinet header
// directly to the underlying TLS connection without copying into writeBuf.
func (t *TunnelConn) WritePreframed(framed []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_, err := t.conn.Write(framed)
	return err
}

// SetReadDeadline sets the read deadline on the underlying connection.
func (t *TunnelConn) SetReadDeadline(d time.Time) error {
	return t.conn.SetReadDeadline(d)
}

// SetWriteDeadline sets the write deadline on the underlying connection.
func (t *TunnelConn) SetWriteDeadline(d time.Time) error {
	return t.conn.SetWriteDeadline(d)
}

// Close aborts any pending read/write immediately and closes the underlying TLS connection.
func (t *TunnelConn) Close() error {
	_ = t.conn.SetDeadline(time.Now())
	return t.conn.Close()
}

func (t *TunnelConn) ensureUpgraded() error {
	t.upgradeChecked = true

	prefix, err := t.br.Peek(5)
	if err != nil {
		return err
	}
	// If the stream does not begin with "HTTP/", it is already binary Fortinet framing.
	if string(prefix) != "HTTP/" {
		return nil
	}

	var hdrBuilder strings.Builder
	for {
		line, err := t.br.ReadString('\n')
		if err != nil {
			return err
		}
		hdrBuilder.WriteString(line)
		if hdrBuilder.Len() > maxUpgradeHTTPHeader {
			return fortivpn.NewError(fortivpn.CodeTunnelDenied, "gateway returned oversized HTTP response on tunnel endpoint")
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	hdrText := hdrBuilder.String()
	firstLine, _, _ := strings.Cut(hdrText, "\n")
	fields := strings.Fields(strings.TrimSpace(firstLine))
	status := 0
	if len(fields) >= 2 {
		status, _ = strconv.Atoi(fields[1])
	}

	lowerHdr := strings.ToLower(hdrText)
	if status == 401 || status == 403 || (status >= 300 && status < 400 && strings.Contains(lowerHdr, "/remote/login")) {
		return fortivpn.NewError(fortivpn.CodeAuthFailed, fmt.Sprintf("gateway rejected tunnel cookie with HTTP %d", status))
	}
	if status < 200 || status >= 300 {
		return fortivpn.NewError(fortivpn.CodeTunnelDenied, fmt.Sprintf("gateway refused SSL-VPN tunnel upgrade with HTTP %d", status))
	}
	if strings.Contains(lowerHdr, "content-type: text/html") {
		return fortivpn.NewError(fortivpn.CodeTunnelDenied, "gateway returned HTML page instead of binary tunnel (tunnel mode disabled)")
	}
	if next, peekErr := t.br.Peek(1); peekErr == nil && len(next) == 1 && next[0] == '<' {
		return fortivpn.NewError(fortivpn.CodeTunnelDenied, "gateway returned HTML page instead of binary tunnel (tunnel mode disabled)")
	}
	return nil
}
