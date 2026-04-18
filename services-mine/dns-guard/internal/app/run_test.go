package app

import "testing"

func TestProfileWhitelistedMatchesByKindAndNameCaseInsensitive(t *testing.T) {
	ref := ProfileRef{Kind: "WG", Name: " Alice-Phone "}
	whitelist := map[string][]string{
		"wg":   {"alice-phone"},
		"ovpn": {"other"},
	}
	if !profileWhitelisted(ref, whitelist) {
		t.Fatal("expected profile to be whitelisted")
	}
}

func TestProfileWhitelistedDoesNotMatchOtherKind(t *testing.T) {
	ref := ProfileRef{Kind: "awg", Name: "alice-phone"}
	whitelist := map[string][]string{
		"wg": {"alice-phone"},
	}
	if profileWhitelisted(ref, whitelist) {
		t.Fatal("did not expect profile to be whitelisted for another kind")
	}
}
