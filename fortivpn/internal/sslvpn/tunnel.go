package sslvpn

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"tunnelkit/fortivpn"
)

const maxUpgradeHTTPHeader = 16 * 1024

// TunnelRequest builds the raw HTTP GET request that switches a TLS connection
// into the Fortinet 6-byte framed PPP tunnel.
func TunnelRequest(host string, port int, cookie, userAgent string) []byte {
	if userAgent == "" {
		userAgent = fortivpn.DefaultUserAgent
	}
	hostHeader := host
	if port > 0 && port != 443 {
		hostHeader = fmt.Sprintf("%s:%d", host, port)
	}
	return []byte("GET " + PathTunnel + " HTTP/1.1\r\n" +
		"Host: " + hostHeader + "\r\n" +
		"User-Agent: " + userAgent + "\r\n" +
		"Accept: */*\r\n" +
		"Accept-Encoding: identity\r\n" +
		"Pragma: no-cache\r\n" +
		"Cache-Control: no-store, no-cache, must-revalidate\r\n" +
		"Cookie: " + CookieName + "=" + cookie + "\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n")
}

// TunnelConn wraps the upgraded TLS connection and handles the transition from
// an optional HTTP status response (e.g. 403/302 rejection or 200 OK header)
// to the 6-octet Fortinet PPP frame stream.
type TunnelConn struct {
	conn           net.Conn
	reader         io.Reader
	upgradeChecked bool
	writeMu        sync.Mutex
	writeBuf       []byte
}

// NewTunnelConn wraps an upgraded TLS connection.
func NewTunnelConn(conn net.Conn) *TunnelConn {
	return &TunnelConn{
		conn:     conn,
		reader:   conn,
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
	return ReadFrame(t.reader)
}

// ReadFrameInto reads the next PPP frame into scratch if large enough.
func (t *TunnelConn) ReadFrameInto(scratch []byte) ([]byte, error) {
	if !t.upgradeChecked {
		if err := t.ensureUpgraded(); err != nil {
			return nil, err
		}
	}
	return ReadFrameInto(t.reader, scratch)
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

	var prefix [5]byte
	if _, err := io.ReadFull(t.conn, prefix[:]); err != nil {
		return err
	}

	// If the stream does not start with "HTTP/", it is already Fortinet binary framing.
	if string(prefix[:]) != "HTTP/" {
		t.reader = io.MultiReader(bytes.NewReader(prefix[:]), t.conn)
		return nil
	}

	// Read until the end of the HTTP response header (\r\n\r\n).
	buf := bytes.NewBuffer(prefix[:])
	var one [1]byte
	for !bytes.HasSuffix(buf.Bytes(), []byte("\r\n\r\n")) {
		if buf.Len() >= maxUpgradeHTTPHeader {
			return fortivpn.NewError(fortivpn.CodeTunnelDenied, "gateway returned oversized HTTP response on tunnel endpoint")
		}
		if _, err := t.conn.Read(one[:]); err != nil {
			return err
		}
		buf.WriteByte(one[0])
	}

	hdrText := buf.String()
	firstLine, _, _ := strings.Cut(hdrText, "\r\n")
	fields := strings.Fields(firstLine)
	status := 0
	if len(fields) >= 2 {
		status, _ = strconv.Atoi(fields[1])
	}

	if status == 401 || status == 403 || (status >= 300 && status < 400 && strings.Contains(strings.ToLower(hdrText), "/remote/login")) {
		return fortivpn.NewError(fortivpn.CodeAuthFailed, fmt.Sprintf("gateway rejected tunnel cookie with HTTP %d", status))
	}
	if status < 200 || status >= 300 {
		return fortivpn.NewError(fortivpn.CodeTunnelDenied, fmt.Sprintf("gateway refused SSL-VPN tunnel upgrade with HTTP %d", status))
	}

	// HTTP 200 OK header consumed; subsequent bytes are Fortinet frames.
	return nil
}
