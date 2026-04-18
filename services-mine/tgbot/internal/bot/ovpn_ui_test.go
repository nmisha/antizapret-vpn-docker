package bot

import "testing"

func TestParseOvpnProfilesFromCertificatesPage(t *testing.T) {
	body := `
<table>
  <a href="/restart/openvpn-server" title="Restart OpenVPN container">Restart Container</a>
  <tr>
    <td><a href="/certificates/Alice-Phone">Alice-Phone</a></td>
    <td>
      <a href="/certificates/revoke/Alice-Phone/123?tfaname=alice">Revoke</a>
      <a class="btn btn-default btn-xs" disabled>Delete</a>
    </td>
  </tr>
  <tr>
    <td><a href="/certificates/Bob-Laptop">Bob-Laptop</a></td>
    <td>
      <a class="btn btn-default btn-xs" disabled>Revoke</a>
      <a href="/certificates/burn/Bob-Laptop/456?tfaname=bob">Delete</a>
    </td>
  </tr>
</table>`

	profiles := parseOvpnProfilesFromCertificatesPage(body)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	if profiles[0].Name != "Alice-Phone" {
		t.Fatalf("expected first profile Alice-Phone, got %q", profiles[0].Name)
	}
	if profiles[0].RevokeURL == "" {
		t.Fatalf("expected revoke url for Alice-Phone")
	}
	if profiles[0].BurnURL != "" {
		t.Fatalf("expected empty burn url for Alice-Phone, got %q", profiles[0].BurnURL)
	}
	if profiles[0].RestartContainerURL != "/restart/openvpn-server" {
		t.Fatalf("expected restart container url for Alice-Phone, got %q", profiles[0].RestartContainerURL)
	}

	if profiles[1].Name != "Bob-Laptop" {
		t.Fatalf("expected second profile Bob-Laptop, got %q", profiles[1].Name)
	}
	if profiles[1].RevokeURL != "" {
		t.Fatalf("expected empty revoke url for Bob-Laptop, got %q", profiles[1].RevokeURL)
	}
	if profiles[1].BurnURL == "" {
		t.Fatalf("expected burn url for Bob-Laptop")
	}
	if profiles[1].RestartContainerURL != "/restart/openvpn-server" {
		t.Fatalf("expected restart container url for Bob-Laptop, got %q", profiles[1].RestartContainerURL)
	}
}
