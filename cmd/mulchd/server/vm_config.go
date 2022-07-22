package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
)

// auto_rebuild setting values
const (
	VMAutoRebuildDaily   = "daily"
	VMAutoRebuildWeekly  = "weekly"
	VMAutoRebuildMonthly = "monthly"
)

// VM tag from config or from script?
const (
	VMTagFromConfig = true
	VMTagFromScript = false
)

// VMConfig stores needed parameters for a new VM
type VMConfig struct {
	FileContent string // config file content

	Name           string
	Hostname       string
	Timezone       string
	AppUser        string
	Seed           string
	InitUpgrade    bool
	DiskSize       uint64
	RAMSize        uint64
	CPUCount       int
	Domains        []*common.Domain
	Env            map[string]string
	Ports          []*VMPort
	BackupDiskSize uint64
	BackupCompress bool
	RestoreBackup  string
	AutoRebuild    string

	Prepare []*VMConfigScript
	Install []*VMConfigScript
	Backup  []*VMConfigScript
	Restore []*VMConfigScript

	DoActions map[string]*VMDoAction
	Tags      map[string]bool
}

// VMConfigScript is a script for prepare, install, save and restore steps
type VMConfigScript struct {
	ScriptURL string
	As        string
}

// VMDoAction is a script for a "do" action (scripts for usual tasks in the VM)
type VMDoAction struct {
	Name        string
	ScriptURL   string
	User        string
	Description string
	FromConfig  bool
}

type tomlVMConfig struct {
	Name            string
	Hostname        string
	Timezone        string
	AppUser         string `toml:"app_user"`
	Seed            string
	InitUpgrade     bool              `toml:"init_upgrade"`
	DiskSize        datasize.ByteSize `toml:"disk_size"`
	RAMSize         datasize.ByteSize `toml:"ram_size"`
	CPUCount        int               `toml:"cpu_count"`
	Domains         []string
	RedirectToHTTPS bool `toml:"redirect_to_https"`
	Redirects       [][]string
	Env             [][]string
	Ports           []string
	BackupDiskSize  datasize.ByteSize `toml:"backup_disk_size"`
	BackupCompress  bool              `toml:"backup_compress"`
	RestoreBackup   string            `toml:"restore_backup"`
	AutoRebuild     string            `toml:"auto_rebuild"`

	PreparePrefixURL string `toml:"prepare_prefix_url"`
	Prepare          []string
	InstallPrefixURL string `toml:"install_prefix_url"`
	Install          []string
	BackupPrefixURL  string `toml:"backup_prefix_url"`
	Backup           []string
	RestorePrefixURL string `toml:"restore_prefix_url"`
	Restore          []string

	DoActions []tomlVMDoAction `toml:"do-actions"`
	Tags      []string         `toml:"tags"`
}

type tomlVMDoAction struct {
	Name        string
	Script      string
	User        string
	Description string
}

func vmCheckScriptURL(scriptURL string, origins *Origins) error {
	// test readability
	stream, errG := origins.GetContent(scriptURL)
	if errG != nil {
		return fmt.Errorf("unable to get script '%s': %s", scriptURL, errG)
	}
	defer stream.Close()

	// check script signature
	signature := make([]byte, 2)
	n, errR := stream.Read(signature)
	if n != 2 || errR != nil {
		return fmt.Errorf("error reading script '%s' (n=%d)", scriptURL, n)
	}
	if string(signature) != "#!" {
		return fmt.Errorf("script '%s': no shebang found, is it really a shell script?", scriptURL)
	}

	return nil
}

func vmConfigGetScript(tScript string, prefixURL string, origins *Origins) (*VMConfigScript, error) {
	script := &VMConfigScript{}

	sepPlace := strings.Index(tScript, "@")
	if sepPlace == -1 {
		return nil, fmt.Errorf("prepre line should use the 'user@url' format ('%s')", tScript)
	}

	as := tScript[:sepPlace]
	scriptName := tScript[sepPlace+1:]

	if !IsValidName(as) {
		return nil, fmt.Errorf("'%s' is not a valid user name", as)
	}
	script.As = as

	var scriptURL string

	scheme, _ := GetURLScheme(scriptName)

	if scheme != "" {
		scriptURL = scriptName
	} else {
		scriptURL = prefixURL + scriptName
	}

	if err := vmCheckScriptURL(scriptURL, origins); err != nil {
		return nil, err
	}

	script.ScriptURL = scriptURL
	return script, nil
}

func vmConfigGetDoAction(tDoAction *tomlVMDoAction, origin *Origins) (*VMDoAction, error) {
	doAction := &VMDoAction{}

	if tDoAction.Name == "" || !IsValidWord(tDoAction.Name) {
		return nil, fmt.Errorf("invalid action name '%s'", tDoAction.Name)
	}

	scriptURL := tDoAction.Script

	if err := vmCheckScriptURL(scriptURL, origin); err != nil {
		return nil, err
	}

	doAction.Name = tDoAction.Name
	doAction.ScriptURL = scriptURL
	doAction.Description = tDoAction.Description
	doAction.User = tDoAction.User
	doAction.FromConfig = true

	return doAction, nil
}

// NewVMConfigFromTomlReader cretes a new VMConfig instance from
// a io.Reader containing VM configuration description
func NewVMConfigFromTomlReader(configIn io.Reader, origins *Origins) (*VMConfig, error) {
	content, err := ioutil.ReadAll(configIn)
	if err != nil {
		return nil, err
	}

	vmConfig := &VMConfig{
		Env:         make(map[string]string),
		FileContent: string(content),
	}

	// defaults (if not in the file)
	tConfig := &tomlVMConfig{
		Timezone:        "Europe/Paris",
		AppUser:         "app",
		InitUpgrade:     true,
		CPUCount:        1,
		RedirectToHTTPS: true,
		BackupDiskSize:  2 * datasize.GB,
		BackupCompress:  true,
	}

	meta, err := toml.Decode(vmConfig.FileContent, tConfig)

	if err != nil {
		return nil, err
	}

	undecoded := meta.Undecoded()
	for _, param := range undecoded {
		return nil, fmt.Errorf("unknown setting '%s'", param)
	}

	if tConfig.Name == "" || !IsValidName(tConfig.Name) {
		return nil, fmt.Errorf("invalid VM name '%s'", tConfig.Name)
	}
	vmConfig.Name = tConfig.Name

	vmConfig.Hostname = tConfig.Hostname
	vmConfig.Timezone = tConfig.Timezone

	if tConfig.AppUser == "" {
		return nil, fmt.Errorf("invalid app_user name '%s'", tConfig.AppUser)
	}
	vmConfig.AppUser = tConfig.AppUser

	if tConfig.Seed == "" || !IsValidName(tConfig.Seed) {
		return nil, fmt.Errorf("invalid seed image '%s'", tConfig.Seed)
	}
	vmConfig.Seed = tConfig.Seed

	vmConfig.InitUpgrade = tConfig.InitUpgrade

	if tConfig.DiskSize < 1*datasize.MB {
		return nil, fmt.Errorf("looks like a too small disk (%s)", tConfig.DiskSize)
	}
	vmConfig.DiskSize = tConfig.DiskSize.Bytes()

	if tConfig.RAMSize < 1*datasize.MB {
		return nil, fmt.Errorf("looks like a too small RAM amount (%s)", tConfig.RAMSize)
	}
	vmConfig.RAMSize = tConfig.RAMSize.Bytes()

	if tConfig.CPUCount < 1 {
		return nil, fmt.Errorf("need a least one CPU")
	}
	vmConfig.CPUCount = tConfig.CPUCount

	// seeders, compute VMs, etc
	// if len(tConfig.Domains) == 0 {
	// 	log.Warningf("no domain defined for this VM")
	// }

	var domainList []string
	for _, domainName := range tConfig.Domains {
		parts := strings.Split(domainName, "->")
		if len(parts) != 1 && len(parts) != 2 {
			return nil, fmt.Errorf("invalid domain string '%s'", domainName)
		}
		hostName := strings.TrimSpace(strings.ToLower(parts[0]))
		portNum := 80
		if len(parts) == 2 {
			portNum, err = strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid port number '%s'", parts[1])
			}
		}
		domain := common.Domain{
			Name:            hostName,
			DestinationPort: portNum,
			RedirectToHTTPS: tConfig.RedirectToHTTPS,
		}
		vmConfig.Domains = append(vmConfig.Domains, &domain)
		domainList = append(domainList, hostName)
	}

	for _, redirectParts := range tConfig.Redirects {
		if len(redirectParts) != 2 && len(redirectParts) != 3 {
			return nil, fmt.Errorf("values for 'redirects' setting must be two string arrays, plus additional HTTP code (['a', 'b', '301'] will redirect a to b permanently)")
		}
		from := strings.TrimSpace(strings.ToLower(redirectParts[0]))
		dest := strings.TrimSpace(strings.ToLower(redirectParts[1]))

		// default redirect code
		status := http.StatusFound
		if len(redirectParts) == 3 {
			status, err = strconv.Atoi(redirectParts[2])
			if err != nil {
				return nil, fmt.Errorf("can't parse '%s' as an integer (%s)", redirectParts[2], err)
			}
			switch status {
			case http.StatusMovedPermanently: // 301
				status = http.StatusMovedPermanently
			case http.StatusFound: // 302
				status = http.StatusFound

			case http.StatusTemporaryRedirect: // 307
				status = http.StatusTemporaryRedirect
			case http.StatusPermanentRedirect: // 308
				status = http.StatusPermanentRedirect

			default:
				return nil, fmt.Errorf("unsupported HTTP redirect code '%d'", status)
			}
		}

		// check if dest is one of our domains
		found := false
		for _, dom := range domainList {
			if dom == dest {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("cannot redirect to '%s', it's not one of VM's domains", dest)
		}

		domain := common.Domain{
			Name:         from,
			RedirectTo:   dest,
			RedirectCode: status,
		}
		vmConfig.Domains = append(vmConfig.Domains, &domain)
	}

	// check for duplicated domain
	domainMap := make(map[string]bool)
	for _, domain := range vmConfig.Domains {
		_, exist := domainMap[domain.Name]
		if exist {
			return nil, fmt.Errorf("domain '%s' is duplicated in this VM", domain.Name)
		}
		domainMap[domain.Name] = true
	}

	if vmConfig.Hostname == "" {
		if len(vmConfig.Domains) > 0 {
			fqdn := vmConfig.Domains[0].Name
			parts := strings.Split(fqdn, ".")
			vmConfig.Hostname = parts[0]
		} else {
			vmConfig.Hostname = "localhost.localdomain"
		}
	}

	for _, line := range tConfig.Env {
		if len(line) != 2 {
			return nil, fmt.Errorf("invalid 'env' line, need two values (key, val), found %d", len(line))
		}

		key := line[0]
		val := line[1]
		if !IsValidName(key) {
			return nil, fmt.Errorf("invalid 'env' name '%s'", key)
		}

		// TODO: check for reserved names?

		_, exists := vmConfig.Env[key]
		if exists {
			return nil, fmt.Errorf("duplicated 'env' name '%s'", key)
		}

		vmConfig.Env[key] = val
	}

	vmConfig.Ports, err = NewVMPortArray(tConfig.Ports)
	if err != nil {
		return nil, err
	}

	if tConfig.BackupDiskSize < 32*datasize.MB {
		return nil, fmt.Errorf("looks like a too small backup disk (%s, min 32MB)", tConfig.BackupDiskSize)
	}
	vmConfig.BackupDiskSize = tConfig.BackupDiskSize.Bytes()
	vmConfig.BackupCompress = tConfig.BackupCompress

	for _, tScript := range tConfig.Prepare {
		script, err := vmConfigGetScript(tScript, tConfig.PreparePrefixURL, origins)
		if err != nil {
			return nil, err
		}
		vmConfig.Prepare = append(vmConfig.Prepare, script)
	}

	for _, tScript := range tConfig.Install {
		script, err := vmConfigGetScript(tScript, tConfig.InstallPrefixURL, origins)
		if err != nil {
			return nil, err
		}
		vmConfig.Install = append(vmConfig.Install, script)
	}

	for _, tScript := range tConfig.Backup {
		script, err := vmConfigGetScript(tScript, tConfig.BackupPrefixURL, origins)
		if err != nil {
			return nil, err
		}
		vmConfig.Backup = append(vmConfig.Backup, script)
	}

	for _, tScript := range tConfig.Restore {
		script, err := vmConfigGetScript(tScript, tConfig.RestorePrefixURL, origins)
		if err != nil {
			return nil, err
		}
		vmConfig.Restore = append(vmConfig.Restore, script)
	}
	vmConfig.RestoreBackup = tConfig.RestoreBackup

	if tConfig.AutoRebuild != "" && tConfig.AutoRebuild != VMAutoRebuildDaily &&
		tConfig.AutoRebuild != VMAutoRebuildWeekly && tConfig.AutoRebuild != VMAutoRebuildMonthly {
		return nil, fmt.Errorf("'%s' is not a correct value for auto_rebuild setting", tConfig.AutoRebuild)
	}
	vmConfig.AutoRebuild = tConfig.AutoRebuild

	var actions []*VMDoAction

	for _, tDoAction := range tConfig.DoActions {
		doAction, err := vmConfigGetDoAction(&tDoAction, origins)
		if err != nil {
			return nil, err
		}
		actions = append(actions, doAction)
	}

	// build a map from the actions array
	vmConfig.DoActions = make(map[string]*VMDoAction)
	for _, action := range actions {
		_, exist := vmConfig.DoActions[action.Name]
		if exist {
			return nil, fmt.Errorf("duplicate do-action '%s'", action.Name)
		}
		vmConfig.DoActions[action.Name] = action
	}

	// build a map of tags
	vmConfig.Tags = make(map[string]bool)
	for _, tag := range tConfig.Tags {
		if !IsValidWord(tag) {
			return nil, fmt.Errorf("invalid tag name '%s'", tag)
		}
		vmConfig.Tags[tag] = VMTagFromConfig
	}

	return vmConfig, nil
}
