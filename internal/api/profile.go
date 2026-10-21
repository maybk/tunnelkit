// Package api 暴露给 Wails 前端调用的服务绑定，只做参数转发与跨领域的简单编排。
package api

import (
	"errors"
	"log/slog"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/vpn"
)

var ErrProfileInUse = errors.New("该连接正在使用中，请先断开后再删除")

// ParsedProfileText 是文本模式解析后的配置及兼容提示。
type ParsedProfileText struct {
	Input    profile.Input      `json:"input"`
	Warnings []fortivpn.Warning `json:"warnings,omitempty"`
}

// ProfileAPI 暴露连接配置的增删改查及文本转换接口。
type ProfileAPI struct {
	store     *profile.Store
	vpn       *vpn.Service
	onChanged func()
}

func NewProfileAPI(store *profile.Store, vpnService *vpn.Service, onChanged func()) *ProfileAPI {
	return &ProfileAPI{store: store, vpn: vpnService, onChanged: onChanged}
}

func (a *ProfileAPI) List() ([]profile.Profile, error) {
	return a.store.List()
}

func (a *ProfileAPI) Save(in profile.Input) (profile.Profile, error) {
	p, err := a.store.Save(in)
	if err == nil {
		a.notify()
	}
	return p, err
}

// Delete 删除配置及其密码与日志；正在使用的配置禁止删除。
func (a *ProfileAPI) Delete(id string) error {
	if a.vpn.State().Active(id) {
		return ErrProfileInUse
	}
	if err := a.store.Delete(id); err != nil {
		return err
	}
	if err := a.vpn.RemoveLog(id); err != nil {
		slog.Warn("删除连接日志失败", "profile", id, "error", err)
	}
	a.notify()
	return nil
}

func (a *ProfileAPI) TrustCert(id, sha256 string) (profile.Profile, error) {
	p, err := a.store.TrustCert(id, sha256)
	if err == nil {
		a.notify()
	}
	return p, err
}

// ParseText 将 openfortivpn 兼容格式的配置文本解析为表单 Input。
func (a *ProfileAPI) ParseText(base profile.Input, text string) (ParsedProfileText, error) {
	cfg, warnings, err := fortivpn.ParseConfigText(text)
	if err != nil {
		return ParsedProfileText{}, err
	}
	return ParsedProfileText{
		Input:    profile.FromConfig(base, cfg),
		Warnings: warnings,
	}, nil
}

// FormatText 将当前表单 Input 渲染为 openfortivpn 兼容格式的配置文本（不落盘）。
func (a *ProfileAPI) FormatText(in profile.Input, hasSavedPassword bool) string {
	if !hasSavedPassword &&
		in.Host == "" &&
		(in.Port == 0 || in.Port == profile.DefaultPort) &&
		in.Username == "" &&
		in.Password == "" &&
		in.Realm == "" &&
		in.TrustedCert == "" &&
		len(in.ExtraRoutes) == 0 &&
		len(in.DNSDomains) == 0 {
		return ""
	}
	temp := profile.Profile{
		ID:          in.ID,
		Name:        in.Name,
		Host:        in.Host,
		Port:        in.Port,
		Username:    in.Username,
		Realm:       in.Realm,
		TrustedCert: in.TrustedCert,
		Auth:        in.Auth,
		Routes:      in.Routes,
		ExtraRoutes: in.ExtraRoutes,
		DNS:         in.DNS,
		DNSDomains:  in.DNSDomains,
		Reconnect:   in.Reconnect,
		UserAgent:   in.UserAgent,
		LegacyTLS:   in.LegacyTLS,
		ClientCert:  in.ClientCert,
		ClientKey:   in.ClientKey,
	}
	pwd := in.Password
	includePassword := true
	if pwd == "" && hasSavedPassword {
		pwd = "__SAVED__"
		includePassword = false
	}
	cfg := temp.ToConfig(pwd)
	return fortivpn.FormatConfigText(cfg, includePassword)
}

func (a *ProfileAPI) notify() {
	if a.onChanged != nil {
		a.onChanged()
	}
}
