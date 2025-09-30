package common

// ProxyChainDomains list domains to forward to a target proxy
type ProxyChainDomains struct {
	Domains   []ProxyChainDomain
	ForwardTo string
}

type ProxyChainDomain struct {
	Domain      string
	RateProfile string
}

// ProxyChainConflictingDomain describes a conflicting domain
type ProxyChainConflictingDomain struct {
	Domain string
	Owner  string
}

// ProxyChainConflictingDomains is a list of conflicting domains
type ProxyChainConflictingDomains []ProxyChainConflictingDomain

// NewProxyChainDomain creates a new ProxyChainDomain
func NewProxyChainDomain(domain, rateProfile string) ProxyChainDomain {
	return ProxyChainDomain{
		Domain:      domain,
		RateProfile: rateProfile,
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
