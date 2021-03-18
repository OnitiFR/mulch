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
// and replace it with "domains"
func (ddb *DomainDatabase) ReplaceChainedDomains(domains []string, forwardTo string) error {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	// 1 - delete all previous domains for this child
	for key, domain := range ddb.db {
		if domain.Chained && domain.TargetURL == forwardTo {
			delete(ddb.db, key)
		}
	}

	// 2 - add new domains, erasing any conflicting domain
	for _, domain := range domains {
		ddb.db[domain] = &common.Domain{
			Name:      domain,
			TargetURL: forwardTo,
			Chained:   true,
		}
	}

	err := ddb.save()
	if err != nil {
		return err
	}
	return nil
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
