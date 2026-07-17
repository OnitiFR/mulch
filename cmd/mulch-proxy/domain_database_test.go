package main

import (
	"path/filepath"
	"testing"

	"github.com/OnitiFR/mulch/common"
)

func newTestDomainDatabase(t *testing.T) *DomainDatabase {
	t.Helper()
	ddb, err := NewDomainDatabase(filepath.Join(t.TempDir(), "domains.json"), true)
	if err != nil {
		t.Fatalf("can't create test domain database: %s", err)
	}
	return ddb
}

// two children on the same host but different ports must be told apart:
// the historical hostname-only comparison let a child activate a VM whose
// domain was owned by its same-host neighbour.
func TestGetConflictingDomainsSameHostDifferentPort(t *testing.T) {
	ddb := newTestDomainDatabase(t)

	_, err := ddb.ReplaceChainedDomains([]common.ProxyChainDomain{
		{Domain: "www.example.com"},
	}, "http://mulch.mydomain.tld:8687")
	if err != nil {
		t.Fatal(err)
	}

	// same child (other URL variant of the same hostname:port): no conflict
	conflicts, err := ddb.GetConflictingDomains([]string{"www.example.com"}, "http://mulch.mydomain.tld:8687/")
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("child should not conflict with itself, got %v", conflicts)
	}

	// other child on the same host (different port): conflict
	conflicts, err = ddb.GetConflictingDomains([]string{"www.example.com"}, "http://mulch.mydomain.tld:9797")
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %v", conflicts)
	}
	if conflicts[0].Owner != "mulch.mydomain.tld:8687" {
		t.Fatalf("unexpected owner '%s'", conflicts[0].Owner)
	}

	// unidentifiable requester: fail-closed
	if _, err := ddb.GetConflictingDomains([]string{"www.example.com"}, ""); err == nil {
		t.Fatal("empty child URL should be an error")
	}
}

func TestReplaceChainedDomainsPin(t *testing.T) {
	ddb := newTestDomainDatabase(t)

	childA := "http://hosta.tld:8687"
	childB := "http://hostb.tld:8687"

	// A owns the domain (not pinned)
	if _, err := ddb.ReplaceChainedDomains([]common.ProxyChainDomain{
		{Domain: "www.example.com"},
	}, childA); err != nil {
		t.Fatal(err)
	}

	// B takes it over with a pin (promote case: last writer wins on unpinned)
	refused, err := ddb.ReplaceChainedDomains([]common.ProxyChainDomain{
		{Domain: "www.example.com", Pinned: true},
	}, childB)
	if err != nil || len(refused) != 0 {
		t.Fatalf("pinned takeover of an unpinned domain should succeed (refused=%v, err=%s)", refused, err)
	}

	// A comes back: its registration must be refused for this domain
	refused, err = ddb.ReplaceChainedDomains([]common.ProxyChainDomain{
		{Domain: "www.example.com"},
	}, childA)
	if err != nil {
		t.Fatal(err)
	}
	if len(refused) != 1 || refused[0] != "www.example.com" {
		t.Fatalf("expected www.example.com to be refused, got %v", refused)
	}
	domain, err := ddb.GetByName("www.example.com")
	if err != nil || domain.TargetURL != childB || !domain.Pinned {
		t.Fatalf("domain should still be pinned by B, got %+v", domain)
	}

	// B (same identity, other URL variant) can update its own pinned domain
	refused, err = ddb.ReplaceChainedDomains([]common.ProxyChainDomain{
		{Domain: "www.example.com", Pinned: true, RateProfile: "slow"},
	}, childB+"/")
	if err != nil || len(refused) != 0 {
		t.Fatalf("owner should be able to update its pinned domain (refused=%v, err=%s)", refused, err)
	}

	// B stops publishing the domain: pin released, A can register again
	if _, err := ddb.ReplaceChainedDomains([]common.ProxyChainDomain{}, childB); err != nil {
		t.Fatal(err)
	}
	refused, err = ddb.ReplaceChainedDomains([]common.ProxyChainDomain{
		{Domain: "www.example.com"},
	}, childA)
	if err != nil || len(refused) != 0 {
		t.Fatalf("released domain should be registrable again (refused=%v, err=%s)", refused, err)
	}
}

func TestProxyChainChildIdentity(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"http://mulch.mydomain.tld:8687", "mulch.mydomain.tld:8687", false},
		{"http://mulch.mydomain.tld:8687/", "mulch.mydomain.tld:8687", false},
		{"http://mulch.mydomain.tld", "mulch.mydomain.tld:80", false},
		{"https://mulch.mydomain.tld", "mulch.mydomain.tld:443", false},
		{"", "", true},
		{"not a url", "", true},
		{"ftp://mulch.mydomain.tld", "", true},
	}
	for _, tc := range cases {
		got, err := common.ProxyChainChildIdentity(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected an error, got '%s'", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %s", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got '%s', want '%s'", tc.in, got, tc.want)
		}
	}
}
