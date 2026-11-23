package app

import (
	"encoding/json"
	"testing"
)

func TestOVPNSessionResponseParsesCamelCaseFields(t *testing.T) {
	const payload = `{
		"status":"success",
		"message":"",
		"data":{
			"Title":"OpenVPN",
			"ClientList":[
				{
					"CommonName":"Mikhail-Mikrotik-NL-1",
					"VirtualAddress":"10.1.165.2"
				}
			]
		}
	}`

	var resp ovpnAPIResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Data.ClientList) != 1 {
		t.Fatalf("expected 1 client, got %d", len(resp.Data.ClientList))
	}
	client := resp.Data.ClientList[0]
	if client.CommonName != "Mikhail-Mikrotik-NL-1" {
		t.Fatalf("unexpected common name: %q", client.CommonName)
	}
	if client.VirtualAddress != "10.1.165.2" {
		t.Fatalf("unexpected virtual address: %q", client.VirtualAddress)
	}
}

func TestParseOVPNCertificateProfiles(t *testing.T) {
	const body = `
<table>
  <a href="/restart/openvpn-server" title="Restart OpenVPN container">Restart Container</a>
  <tr>
    <td><a href="/certificates/Mikhail-Mikrotik-NL-1">Mikhail-Mikrotik-NL-1</a></td>
    <td><a href="/certificates/revoke/Mikhail-Mikrotik-NL-1/123?tfaname=x">Revoke</a></td>
  </tr>
  <tr>
    <td><a href="/certificates/Revoked-Profile">Revoked-Profile</a></td>
    <td><a class="btn" disabled>Revoke</a></td>
  </tr>
</table>`

	profiles := parseOVPNCertificateProfiles(body)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "Mikhail-Mikrotik-NL-1" {
		t.Fatalf("unexpected first profile name: %q", profiles[0].Name)
	}
	if profiles[0].RevokeURL == "" {
		t.Fatal("expected revoke url for first profile")
	}
	if profiles[0].RestartContainerURL != "/restart/openvpn-server" {
		t.Fatalf("unexpected restart container url: %q", profiles[0].RestartContainerURL)
	}
	if profiles[1].Name != "Revoked-Profile" {
		t.Fatalf("unexpected second profile name: %q", profiles[1].Name)
	}
	if profiles[1].RevokeURL != "" {
		t.Fatalf("expected empty revoke url for second profile, got %q", profiles[1].RevokeURL)
	}
}
