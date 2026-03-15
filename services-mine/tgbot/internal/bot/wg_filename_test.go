package bot

import "testing"

func TestNormalizeWgConfigFilename(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "drops leading underscore segments until it fits",
			raw:  "Chernikov_Kondraichuk_Olga_Phone1",
			want: "Phone1.conf",
		},
		{
			name: "keeps allowed short name",
			raw:  "Phone-1",
			want: "Phone-1.conf",
		},
		{
			name: "removes disallowed characters",
			raw:  "My Phone #1!",
			want: "MyPhone1.conf",
		},
		{
			name: "trims from the front when the last segment is still too long",
			raw:  "VeryLongSegmentName",
			want: "egmentName.conf",
		},
		{
			name: "handles existing extension",
			raw:  "abc_DEF.conf",
			want: "abcDEF.conf",
		},
		{
			name: "uses fallback when nothing valid remains",
			raw:  "___!!!",
			want: "wg.conf",
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
