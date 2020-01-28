package common

// ProxyChainDomains list domains to forward to a target proxy
type ProxyChainDomains struct {
	Domains   []string
	ForwardTo string
}

// ProxyChainConflictingDomain describes a conflicting domain
type ProxyChainConflictingDomain struct {
	Domain string
	Owner  string
}

// ProxyChainConflictingDomains is a list of conflicting domains
type ProxyChainConflictingDomains []ProxyChainConflictingDomain
