package redact

import "testing"

func TestUserPathsRedactsAcrossPlatformsAndIsIdempotent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "macOS home path",
			in:   `open /Users/alice/.tunnelkit/logs/vpn.log`,
			want: `open /Users/[REDACTED]/.tunnelkit/logs/vpn.log`,
		},
		{
			name: "macOS APFS data volume path",
			in:   `path=/System/Volumes/Data/Users/alice/Library`,
			want: `path=/System/Volumes/Data/Users/[REDACTED]/Library`,
		},
		{
			name: "macOS file URL",
			in:   `file:///Users/alice/.tunnelkit/config.json`,
			want: `file:///Users/[REDACTED]/.tunnelkit/config.json`,
		},
		{
			name: "Linux home path with trailing prose",
			in:   `dir /home/bob failed to open`,
			want: `dir /home/[REDACTED] failed to open`,
		},
		{
			name: "Windows backslash and spaced username",
			in:   `open C:\Users\John Doe\.tunnelkit\secret.key`,
			want: `open C:\Users\[REDACTED]\.tunnelkit\secret.key`,
		},
		{
			name: "Windows JSON escaped path",
			in:   `{"path":"C:\\Users\\alice\\.tunnelkit\\profiles.json"}`,
			want: `{"path":"C:\\Users\\[REDACTED]\\.tunnelkit\\profiles.json"}`,
		},
		{
			name: "Windows forward slash path",
			in:   `dir=D:/Users/alice/AppData/Local`,
			want: `dir=D:/Users/[REDACTED]/AppData/Local`,
		},
		{
			name: "preserves shared and public system folders",
			in:   `/Users/Shared/data and C:\Users\Public\Documents`,
			want: `/Users/Shared/data and C:\Users\Public\Documents`,
		},
		{
			name: "preserves non-home URL segment",
			in:   `https://api.example.com/v1/Users/alice`,
			want: `https://api.example.com/v1/Users/alice`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := UserPaths(tc.in)
			if got != tc.want {
				t.Fatalf("UserPaths(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := UserPaths(got); again != got {
				t.Fatalf("UserPaths is not idempotent: %q -> %q", got, again)
			}
		})
	}
}
