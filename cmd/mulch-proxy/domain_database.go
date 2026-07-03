package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

// DomainDatabase describes a persistent DataBase of Domain structures
type DomainDatabase struct {
	filename string
	db       map[string]*common.Domain
	mutex    sync.Mutex
}

// NewDomainDatabase instanciates a new DomainDatabase
// Set autoCreate to true if you want to create an empty db when
// no existing file is found. needed for proxy parents, they have
// no nearby mulchd to create the file for them)
func NewDomainDatabase(filename string, autoCreate bool) (*DomainDatabase, error) {
	ddb := &DomainDatabase{
		filename: filename,
	}

	if autoCreate && !common.PathExist(filename) {
		ddb.db = make(map[string]*common.Domain)
		err := ddb.save()
		if err != nil {
			return nil, err
		}
	}

	err := ddb.load()
	if err != nil {
		return nil, err
	}

	return ddb, nil
}

func (ddb *DomainDatabase) load() error {
	f, err := os.Open(ddb.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// clear any previous map
	ddb.db = make(map[string]*common.Domain)

	dec := json.NewDecoder(f)
	err = dec.Decode(&ddb.db)
	if err != nil {
		return err
	}
	return nil
}

// only needed for proxy chain parents
func (ddb *DomainDatabase) save() error {
	f, err := os.OpenFile(ddb.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&ddb.db)
	if err != nil {
		return err
	}
	return nil
}

// Reload is the mutex-protected variant of load()
func (ddb *DomainDatabase) Reload() error {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	return ddb.load()
}

// GetDomainsNames return all domain names in the database
func (ddb *DomainDatabase) GetDomainsNames() []string {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	keys := make([]string, 0, len(ddb.db))
	for key := range ddb.db {
		keys = append(keys, key)
	}
	return keys
}

// GetProxyChainDomains returns all domains as a ProxyChainDomain array
func (ddb *DomainDatabase) GetProxyChainDomains() []common.ProxyChainDomain {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	var domains []common.ProxyChainDomain
	for _, domain := range ddb.db {
		domains = append(domains, common.NewProxyChainDomain(domain.Name, domain.RateProfile, domain.Pinned))
	}
	return domains
}

// GetByName lookups a Domain by its domain
func (ddb *DomainDatabase) GetByName(name string) (*common.Domain, error) {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	domain, exists := ddb.db[name]
	if !exists {
		return nil, fmt.Errorf("domain '%s' not found in database", name)
	}
	return domain, nil
}

// Count returns the number of Domains in the database
func (ddb *DomainDatabase) Count() int {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	return len(ddb.db)
}

// ReplaceChainedDomains remove all domains chain-forwared to "forwardTo"
// and replace it with "domains".
//
// A conflicting domain is normally erased ("last writer wins", this is what
// lets a promoted VM take its domains over), with one exception: a domain
// pinned by ANOTHER child is never overwritten — the first pin wins — and is
// returned in refused, so the caller can log it. The owner itself can always
// update, re-pin or release its own domains (they are dropped in step 1).
func (ddb *DomainDatabase) ReplaceChainedDomains(domains []common.ProxyChainDomain, forwardTo string) ([]string, error) {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	// 1 - delete all previous domains for this child
	for key, domain := range ddb.db {
		if domain.Chained && domain.TargetURL == forwardTo {
			delete(ddb.db, key)
		}
	}

	// 2 - add new domains, erasing any conflicting domain (unless pinned)
	var refused []string
	for _, domain := range domains {
		if existing, exists := ddb.db[domain.Domain]; exists && existing.Pinned {
			// existing.TargetURL != forwardTo here: this child's own entries
			// were deleted in step 1
			refused = append(refused, domain.Domain)
			continue
		}
		ddb.db[domain.Domain] = &common.Domain{
			Name:        domain.Domain,
			TargetURL:   forwardTo,
			Chained:     true,
			RateProfile: domain.RateProfile,
			Pinned:      domain.Pinned,
		}
	}

	err := ddb.save()
	if err != nil {
		return refused, err
	}
	return refused, nil
}

// GetConflictingDomains returns a list of conflicting domains in the provided
// list, excluding those from a specific child
func (ddb *DomainDatabase) GetConflictingDomains(reqDomains []string, childForwardURL string) common.ProxyChainConflictingDomains {
	childURL, err := url.ParseRequestURI(childForwardURL)
	if err != nil {
		return nil
	}

	var conflicts common.ProxyChainConflictingDomains
	for _, reqDomain := range reqDomains {
		domain, err := ddb.GetByName(reqDomain)
		if err != nil {
			// no conflict for this domain
			continue
		}
		targetURL, err := url.ParseRequestURI(domain.TargetURL)
		if err != nil {
			// not a child route? inconsistency?
			continue
		}
		if domain != nil && targetURL.Hostname() != childURL.Hostname() {
			conflicts = append(conflicts, common.ProxyChainConflictingDomain{
				Domain: domain.Name,
				Owner:  targetURL.Hostname(),
			})
		}
	}
	return conflicts
}

// GetChildren returns the list of children-proxies URLs
// This function is only useful for a parent proxy, of course (empty list otherwise)
func (ddb *DomainDatabase) GetChildren() []string {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	children := make(map[string]bool)
	for _, domain := range ddb.db {
		if domain.Chained {
			children[domain.TargetURL] = true
		}
	}

	keys := make([]string, 0, len(children))
	for k := range children {
		keys = append(keys, k)
	}
	return keys
}
