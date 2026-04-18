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
