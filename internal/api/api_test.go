package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/update"
	"tunnelkit/internal/vpn"
)

type fakeLauncher struct {
	eventsCh chan helperclient.Message
}

func newFakeLauncher() *fakeLauncher {
	return &fakeLauncher{eventsCh: make(chan helperclient.Message, 16)}
}

func (f *fakeLauncher) Launch(_ context.Context, req helperclient.LaunchRequest) (vpn.Conn, helperclient.Ticket, error) {
	f.eventsCh <- helperclient.Message{
		Event: &fortivpn.Event{
			Phase: fortivpn.PhaseConnected,
			Info: &fortivpn.TunnelInfo{
				LocalIP:     "10.200.0.2",
				GatewayHost: req.Config.Host,
			},
		},
	}
	return f, helperclient.Ticket{
		Endpoint:    "fake.sock",
		Token:       "token-1",
		PID:         1234,
		ProfileID:   req.ProfileID,
		ProfileName: req.ProfileName,
	}, nil
}

func (f *fakeLauncher) Attach(_ context.Context, _ helperclient.Ticket) (vpn.Conn, *fortivpn.Event, error) {
	return nil, nil, errors.New("not running")
}

func (f *fakeLauncher) Events() <-chan helperclient.Message { return f.eventsCh }
func (f *fakeLauncher) Answer(_ context.Context, _, _ string, _ bool) error {
	return nil
}
func (f *fakeLauncher) Stop(_ context.Context) error {
	close(f.eventsCh)
	return nil
}
func (f *fakeLauncher) Close() error { return nil }

func TestProfileAPICrudTextAndActiveProtection(t *testing.T) {
	rootDir := t.TempDir()
	store := profile.NewStore(rootDir)
	launcher := newFakeLauncher()
	vpnSvc := vpn.NewService(rootDir, store, launcher)

	var changedCount atomic.Int32
	profileAPI := NewProfileAPI(store, vpnSvc, func() {
		changedCount.Add(1)
	})
	vpnAPI := NewVPNAPI(vpnSvc)

	if blank := profileAPI.FormatText(profile.Input{Port: 443}, false); blank != "" {
		t.Fatalf("expected empty FormatText for blank input, got %q", blank)
	}

	saved, err := profileAPI.Save(profile.Input{
		Name:     "Corp VPN",
		Host:     "vpn.example.com",
		Port:     8443,
		Username: "alice",
		Password: "secret-password",
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if changedCount.Load() != 1 {
		t.Fatalf("expected onChanged to be called once after Save, got %d", changedCount.Load())
	}

	list, err := profileAPI.List()
	if err != nil || len(list) != 1 || list[0].ID != saved.ID {
		t.Fatalf("List() = %+v, err = %v", list, err)
	}

	formattedSaved := profileAPI.FormatText(profile.Input{
		ID:       saved.ID,
		Name:     saved.Name,
		Host:     saved.Host,
		Port:     saved.Port,
		Username: saved.Username,
	}, true)
	if strings.Contains(formattedSaved, "secret-password") || !strings.Contains(formattedSaved, "# password =") {
		t.Fatalf("FormatText with saved password and empty input password must mask plaintext password: %s", formattedSaved)
	}

	parsed, err := profileAPI.ParseText(profile.Input{Name: "Corp VPN"}, "host = gw.example.com\nport = 10443\nusername = bob\n")
	if err != nil {
		t.Fatalf("ParseText failed: %v", err)
	}
	if parsed.Input.Host != "gw.example.com" || parsed.Input.Port != 10443 || parsed.Input.Username != "bob" {
		t.Fatalf("unexpected ParseText result: %+v", parsed.Input)
	}

	const validPin = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	trusted, err := profileAPI.TrustCert(saved.ID, validPin)
	if err != nil || trusted.TrustedCert != validPin {
		t.Fatalf("TrustCert failed: profile=%+v, err=%v", trusted, err)
	}

	if err := vpnAPI.Connect(context.Background(), saved.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for vpnAPI.State().Status != vpn.StatusConnected && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if vpnAPI.State().Status != vpn.StatusConnected {
		t.Fatalf("expected Connected state, got %+v", vpnAPI.State())
	}

	if err := profileAPI.Delete(saved.ID); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("Delete on active profile = %v, want ErrProfileInUse", err)
	}

	_ = vpnAPI.Answer(context.Background(), "none", "", true)
	if _, err := vpnAPI.Log(saved.ID); err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if err := vpnAPI.ClearLog(saved.ID); err != nil {
		t.Fatalf("ClearLog failed: %v", err)
	}

	if err := vpnAPI.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}
	vpnAPI.Cancel()
	if err := profileAPI.Delete(saved.ID); err != nil {
		t.Fatalf("Delete after disconnect failed: %v", err)
	}
}

func TestSystemAPIInfoAndCheckUpdate(t *testing.T) {
	payload := []byte("fake-update-binary")
	sumBytes := sha256.Sum256(payload)
	sum := hex.EncodeToString(sumBytes[:])

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v0.1.1",
			"html_url":"https://github.com/maybk/tunnelkit/releases/tag/v0.1.1",
			"assets":[{
				"name":"tunnelkit-0.1.1-windows-amd64.zip",
				"browser_download_url":"` + srv.URL + `/maybk/tunnelkit/releases/download/v0.1.1/tunnelkit-0.1.1-windows-amd64.zip",
				"size":18,
				"digest":"sha256:` + sum + `"
			}]
		}`))
	}))
	defer srv.Close()

	var preHookCalled, restarted atomic.Bool
	sysAPI := NewSystemAPI(
		"0.1.0",
		update.WithHTTPClient(srv.Client()),
		update.WithReleaseURLs(srv.URL),
		update.WithProxyPrefix(""),
		update.WithPlatform("windows", "amd64"),
		update.WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, srv.URL+"/")
		}),
		update.WithPreUpdateHook(func(context.Context) error {
			preHookCalled.Store(true)
			return nil
		}),
		update.WithApplier(func(_ context.Context, _ string, _ update.ProgressFunc) (func() error, error) {
			return func() error {
				restarted.Store(true)
				return nil
			}, nil
		}),
	)
	info := sysAPI.Info(context.Background())
	if info.AppVersion != "0.1.0" {
		t.Fatalf("AppVersion = %q, want 0.1.0", info.AppVersion)
	}
	if info.Platform == "" {
		t.Fatal("Platform must not be empty")
	}

	upd, err := sysAPI.CheckUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckUpdate failed: %v", err)
	}
	if !upd.HasUpdate || upd.LatestVersion != "0.1.1" || !upd.CanSelfUpdate {
		t.Fatalf("unexpected UpdateInfo: %+v", upd)
	}

	if err := sysAPI.ApplyUpdate(context.Background()); err != nil {
		t.Fatalf("ApplyUpdate failed: %v", err)
	}
	if !preHookCalled.Load() || !restarted.Load() {
		t.Fatalf("expected preHookCalled=true and restarted=true, got pre=%v restart=%v", preHookCalled.Load(), restarted.Load())
	}
}
