package server

import (
	"strings"
	"testing"
)

func TestNeutralizeTOMLReplicationPeer(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantChanged bool
	}{
		{
			name:        "simple assignment",
			content:     "name = \"myvm\"\nreplication_peer = \"peerA\"\nreplication_interval = \"5m\"\n",
			wantChanged: true,
		},
		{
			name:        "indented, no spaces around equal",
			content:     "name = \"myvm\"\n  replication_peer=\"peerA\"\n",
			wantChanged: true,
		},
		{
			name:        "no replication_peer",
			content:     "name = \"myvm\"\nreplication_interval = \"5m\"\n",
			wantChanged: false,
		},
		{
			name:        "already commented out",
			content:     "name = \"myvm\"\n# replication_peer = \"peerA\"\n",
			wantChanged: false,
		},
		{
			name:        "longer key sharing the prefix",
			content:     "name = \"myvm\"\nreplication_peer_backup = \"peerA\"\n",
			wantChanged: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := NeutralizeTOMLReplicationPeer(tc.content, "test-api-key")

			if !tc.wantChanged {
				if out != tc.content {
					t.Fatalf("content should be untouched, got:\n%s", out)
				}
				return
			}

			if out == tc.content {
				t.Fatal("content should have been modified")
			}
			for line := range strings.SplitSeq(out, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "replication_peer") {
					t.Fatalf("replication_peer line still active: %q", line)
				}
				if strings.HasPrefix(trimmed, "#") && strings.Contains(line, "replication_peer") {
					if !strings.Contains(line, "neutralized by 'replica promote'") {
						t.Fatalf("neutralized line misses its marker: %q", line)
					}
					if !strings.Contains(line, "by test-api-key") {
						t.Fatalf("neutralized line misses the author: %q", line)
					}
					// the original assignment must be preserved in the comment
					if !strings.Contains(line, "peerA") {
						t.Fatalf("neutralized line lost the original value: %q", line)
					}
				}
			}
			// other keys must be preserved
			if !strings.Contains(out, "name = \"myvm\"") {
				t.Fatal("unrelated content was altered")
			}
		})
	}
}
