package sslvpn_test

import (
	"net/url"
	"reflect"
	"strings"
	"testing"

	"tunnelkit/fortivpn/internal/sslvpn"
)

func TestAuthenticationFormsPreserveFieldValues(t *testing.T) {
	username, password, realm := "用户 +&=", "password&=+ %", "realm /&"
	got, err := url.ParseQuery(sslvpn.BuildLoginForm(username, password, realm))
	want := url.Values{"username": {username}, "credential": {password}, "realm": {realm}, "ajax": {"1"}, "just_logged_in": {"1"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("login form fields changed: %v", err)
	}
	for _, push := range []bool{false, true} {
		code := "12+&34"
		if push {
			code = ""
		}
		c := sslvpn.Challenge{TokenInfo: "ftm_push", Echo: map[string]string{"magic": "m +&", "reqid": "r=&", "unknown": "ignored"}}
		encoded := sslvpn.BuildChallengeForm(username, code, realm, c)
		got, err = url.ParseQuery(encoded)
		if err != nil || got.Get("username") != username || got.Get("code") != code || got.Get("realm") != realm || got.Get("reqid") != "r=&" || got.Has("unknown") {
			t.Fatal("challenge form fields changed")
		}
		if push {
			if got.Has("magic") || !strings.HasSuffix(encoded, "&ftmpush=1") {
				t.Fatal("push must replace magic with final ftmpush field")
			}
		} else if !strings.HasSuffix(encoded, "&magic="+url.QueryEscape("m +&")) {
			t.Fatal("magic must remain the final field")
		}
	}
}

func TestLoginPromptPreservesUnknownKeyLikeText(t *testing.T) {
	res, err := sslvpn.ParseLoginResult("ret=2,tokeninfo=totp,chal_msg=Enter code, valid=30 seconds, now,magic=m1,reqid=r1")
	if err != nil || res.Challenge().Message != "Enter code, valid=30 seconds, now" || res.Challenge().Echo["reqid"] != "r1" {
		t.Fatalf("challenge parsing lost message or echo fields: %v", err)
	}
}
