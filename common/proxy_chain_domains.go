package common

// ProxyChainDomainList is a list of ProxyChainDomain, used by parent and
// children proxies for proxy chaining
type ProxyChainDomainList []ProxyChainDomain

// ProxyChainDomain maps a domain to a target proxy
type ProxyChainDomain struct {
	Domain    string
	ForwardTo string
}
