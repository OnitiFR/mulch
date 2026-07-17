package common

import (
	"fmt"
	"net/url"
)

// ProxyChainChildIdentity returns the canonical identity of a proxy-chain
// child from its registration URL: "hostname:port", the port being defaulted
// from the scheme (http: 80, https: 443).
//
// The full URL keeps being stored and used for request forwarding; this
// identity is only used to compare children with each other (domain ownership
// in ReplaceChainedDomains, conflict pre-checks in GetConflictingDomains), so
// the same child is recognized whatever URL variant it sends, while two
// children on the same host but different ports stay distinct.
func ProxyChainChildIdentity(rawURL string) (string, error) {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid proxy-chain child URL '%s': %s", rawURL, err)
	}

	if u.Hostname() == "" {
		return "", fmt.Errorf("invalid proxy-chain child URL '%s': no host", rawURL)
	}

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", fmt.Errorf("invalid proxy-chain child URL '%s': unsupported scheme '%s'", rawURL, u.Scheme)
		}
	}

	return u.Hostname() + ":" + port, nil
}

// ProxyChainDomains list domains to forward to a target proxy
type ProxyChainDomains struct {
	Domains   []ProxyChainDomain
	ForwardTo string
}

type ProxyChainDomain struct {
	Domain      string
	RateProfile string
	Pinned      bool // see Domain.Pinned
}

// ProxyChainConflictingDomain describes a conflicting domain
type ProxyChainConflictingDomain struct {
	Domain string
	Owner  string
}

// ProxyChainConflictingDomains is a list of conflicting domains
type ProxyChainConflictingDomains []ProxyChainConflictingDomain

// NewProxyChainDomain creates a new ProxyChainDomain
func NewProxyChainDomain(domain, rateProfile string, pinned bool) ProxyChainDomain {
	return ProxyChainDomain{
		Domain:      domain,
		RateProfile: rateProfile,
		Pinned:      pinned,
	}
}

// GetDomainNames returns a list of domain names from the ProxyChainDomains
func (data *ProxyChainDomains) GetDomainNames() []string {
	var names []string
	for _, domain := range data.Domains {
		names = append(names, domain.Domain)
	}
	return names
}
