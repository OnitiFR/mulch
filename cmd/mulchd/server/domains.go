package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// CheckDomainsConflicts will detect if incoming domains conflicts with existing VMs
// of other mulchd servers (in case of proxy chaining)
// You can exclude a specific VM (every revisions) using its name (use empty string otherwise)
func CheckDomainsConflicts(db *VMDatabase, domains []*common.Domain, excludeVM string, config *AppConfig) error {
	err := CheckDomainsConflictsLocal(db, domains, excludeVM)
	if err != nil {
		return err
	}

	if config.ProxyChainMode == ProxyChainModeChild {
		err := CheckDomainsConflictsOnParent(domains, config)
		if err != nil {
			return err
		}
	}

	return nil
}

// CheckDomainsConflictsLocal is the local-only part of CheckDomainsConflicts:
// it never contacts the proxy-chain parent. Used by the 'replica promote'
// path ("force", see HA_PROMOTE.md step 3): the parent may legitimately still
// route the domains to the dead source peer, but a local conflict remains a
// real one (this host would compete with itself).
func CheckDomainsConflictsLocal(db *VMDatabase, domains []*common.Domain, excludeVM string) error {
	domainMap := make(map[string]*VM)
	vmNames := db.GetNames()
	for _, vmName := range vmNames {
		if excludeVM != "" && vmName.Name == excludeVM {
			continue
		}

		entry, err := db.GetEntryByName(vmName)
		if err != nil {
			return err
		}

		if !entry.Active {
			continue
		}

		for _, domain := range entry.VM.Config.Domains {
			domainMap[domain.Name] = entry.VM
		}
	}

	for _, domain := range domains {
		vm, exist := domainMap[domain.Name]
		if exist {
			return fmt.Errorf("vm '%s' already registered domain '%s'", vm.Config.Name, domain.Name)
		}
	}

	return nil
}

// CheckDomainsConflictsOnParent will contact proxy-chain parent and ask if any
// domain is conflicting with another child mulchd
func CheckDomainsConflictsOnParent(domains []*common.Domain, config *AppConfig) error {
	var pcDomains []common.ProxyChainDomain
	for _, domain := range domains {
		pcDomains = append(pcDomains, common.ProxyChainDomain{
			Domain: domain.Name,
		})
	}

	data := common.ProxyChainDomains{
		Domains:   pcDomains,
		ForwardTo: config.ProxyChainChildURL,
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error preparing proxy chain parent request: %s", err)
	}

	client := http.Client{
		Timeout: time.Duration(10 * time.Second),
	}

	req, err := http.NewRequest(
		"POST",
		config.ProxyChainParentURL+"/domains/conflicts",
		bytes.NewBuffer(dataJSON),
	)
	if err != nil {
		return fmt.Errorf("error contacting proxy chain parent: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mulch-PSK", config.ProxyChainPSK)

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot contact proxy chain parent: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("proxy chain parent responded with code %d", res.StatusCode)
	}

	var conflicts common.ProxyChainConflictingDomains
	err = json.NewDecoder(res.Body).Decode(&conflicts)
	if err != nil {
		return fmt.Errorf("error parsing proxy chain parent response: %s", err)
	}

	if len(conflicts) > 0 {
		var parts []string
		for _, conflict := range conflicts {
			parts = append(parts, fmt.Sprintf("%s (%s)", conflict.Domain, conflict.Owner))
		}
		return fmt.Errorf("domain conflict with other mulchd: %s", strings.Join(parts, ", "))
	}

	return nil
}
