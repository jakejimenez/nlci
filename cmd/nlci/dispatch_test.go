package main

import "testing"

func TestLooksLikeRoutingIntent(t *testing.T) {
	known := func(name string) bool {
		switch name {
		case "docker", "gh", "ls":
			return true
		}
		return false
	}

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"empty", nil, false},
		{"bare known tool", []string{"docker"}, false},
		{"bare ls", []string{"ls"}, false},
		{"single quoted intent with space", []string{"show me containers"}, true},
		{"single unknown word", []string{"status"}, true},
		{"classic two-arg", []string{"docker", "ps"}, false},
		{"unquoted multi-word intent", []string{"show", "me", "containers"}, true},
		{"classic with spaced intent", []string{"gh", "list my prs"}, false},
		{"unknown tool with extra args", []string{"notreal", "args"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeRoutingIntent(tc.args, known)
			if got != tc.want {
				t.Fatalf("args=%v: want %v, got %v", tc.args, tc.want, got)
			}
		})
	}
}
