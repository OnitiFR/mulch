package common

// ProxyChainDomains list domains to forward to a target proxy
type ProxyChainDomains struct {
	Domains   []string
	ForwardTo string
}
