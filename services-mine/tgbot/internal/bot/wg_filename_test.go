package bot

import "testing"

func TestNormalizeWgConfigFilename(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "uses two last segments after dropping first",
			raw:  "Chernikov_Kondraichuk_Olga_Phone1",
			want: "OlgaPhone1.conf",
		},
		{
			name: "uses more chars from second when first is short",
			raw:  "Root_A_Berlin22",
			want: "ABerlin22.conf",
		},
		{
			name: "removes disallowed characters before composing",
			raw:  "Root_Ol#ga_Ph#one1",
			want: "OlgaPhone1.conf",
		},
		{
			name: "drops first segment and uses second when only two segments exist",
			raw:  "User_Phone1",
			want: "Phone1.conf",
		},
		{
			name: "returns fallback when no valid chars remain",
			raw:  "Root_@@@_!!!",
			want: "wg.conf",
		},
		{
			name: "uses first eight and last two for single segment",
			raw:  "SingleSegmentName",
			want: "SingleSeme.conf",
		},
		{
			name: "keeps short single segment as is",
			raw:  "Phone1",
			want: "Phone1.conf",
		},
		{
			name: "handles existing extension",
			raw:  "Root_Kate_iPad99.conf",
			want: "KateiPad99.conf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWgConfigFilename(tt.raw); got != tt.want {
				t.Fatalf("normalizeWgConfigFilename(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
