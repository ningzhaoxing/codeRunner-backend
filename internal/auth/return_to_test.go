package auth

import "testing"

func TestNormalizeReturnTo(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty defaults root", in: "", want: "/"},
		{name: "relative path accepted", in: "/posts/go-concurrency", want: "/posts/go-concurrency"},
		{name: "query accepted", in: "/posts?id=1", want: "/posts?id=1"},
		{name: "fragment accepted", in: "/posts#code", want: "/posts#code"},
		{name: "http rejected", in: "http://evil.com", wantErr: true},
		{name: "https rejected", in: "https://evil.com", wantErr: true},
		{name: "protocol relative rejected", in: "//evil.com", wantErr: true},
		{name: "non slash rejected", in: "posts/1", wantErr: true},
		{name: "backslash rejected", in: `/\evil`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeReturnTo(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeReturnTo(%q) expected error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeReturnTo(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeReturnTo(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
