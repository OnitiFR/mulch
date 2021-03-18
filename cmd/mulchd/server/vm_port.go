package server

import (
	"fmt"
	"strconv"
	"strings"
)

// VMPort constants
const (
	VMPortProtocolTCP = 0

	VMPortDirectionExport  = 0
	VMPortDirectionImport  = 1
	VMPortDirectionInvalid = -1
)

// VMPortBaseForward is the value to add to port index
// (ex : first listening port will be 9001, 2nd will be 9002, …)
const VMPortBaseForward uint16 = 9001

// VMPort is a network port inside a VM
type VMPort struct {
	Port      uint16
	Protocol  int // tcp
	Direction int // export / import
	Index     int // position in the direction (ex: 2nd exported port), 0 indexed
	Group     string
}

// String version of the VMPort (Index is not part of the string)
func (p *VMPort) String() string {
	arrow := "->"
	if p.Direction == VMPortDirectionImport {
		arrow = "<-"
	}

	proto := "???"
	if p.Protocol == VMPortProtocolTCP {
		proto = "tcp"
	}

	return fmt.Sprintf("%d/%s%s%s", p.Port, proto, arrow, p.Group)
}

// NewVMPortArray will parse an array of strings and return an array of *VMPort
func NewVMPortArray(strPorts []string) ([]*VMPort, error) {
	var ports []*VMPort
	importIndex := 0
	exportIndex := 0

	for _, line := range strPorts {
		var port VMPort
		var sep string

		if strings.Contains(line, "->") {
			sep = "->"
			port.Direction = VMPortDirectionExport
			port.Index = exportIndex
			exportIndex++
		} else if strings.Contains(line, "<-") {
			sep = "<-"
			port.Direction = VMPortDirectionImport
			port.Index = importIndex
			importIndex++
		} else {
			return nil, fmt.Errorf("can't find port direction (<- or ->) in line '%s'", line)
		}

		lineParts := strings.Split(line, sep)
		if len(lineParts) != 2 {
			return nil, fmt.Errorf("invalid port string '%s'", line)
		}

		strPort := strings.TrimSpace(strings.ToLower(lineParts[0]))

		portParts := strings.Split(strPort, "/")
		if len(portParts) != 2 {
			return nil, fmt.Errorf("invalid port '%s' (ex: 22/tcp)", strPort)
		}

		portNum, err := strconv.Atoi(portParts[0])
		if err != nil {
			return nil, fmt.Errorf("cannot parse port as an integer in '%s' (ex: 22/tcp)", line)
		}
		if portNum < 1 && portNum > 65535 {
			return nil, fmt.Errorf("port '%d' is out of bound ", portNum)
		}

		if portParts[1] != "tcp" {
			return nil, fmt.Errorf("only tcp protocol is supported (%s)", line)
		}

		port.Protocol = VMPortProtocolTCP
		port.Port = uint16(portNum)

		group := strings.TrimSpace(strings.ToLower(lineParts[1]))
		if !IsValidGroupName(group) {
			return nil, fmt.Errorf("invalid group name '%s' (ex: @my_group)", group)
		}
		port.Group = group

		// check duplicates
		for _, p := range ports {
			if port.Port == p.Port && port.Direction == p.Direction && port.Protocol == p.Protocol && port.Group == p.Group {
				return nil, fmt.Errorf("duplicate port '%s'", line)
			}
		}

		ports = append(ports, &port)
	}

	return ports, nil
}

// CheckPortsConflicts will detect exported port conflicts with existing VMs
// and warn if an imported port is not exported (yet?) by another VM (if log is not nil)
func CheckPortsConflicts(db *VMDatabase, ports []*VMPort, excludeVM string, log *Log) error {
	exportPortMap := make(map[string]*VM)

	// build maps
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

		for _, port := range entry.VM.Config.Ports {
			if port.Direction == VMPortDirectionExport {
				exportPortMap[port.String()] = entry.VM
			}
		}
	}

	// search duplicate imports, warn about missing imports
	for _, port := range ports {
		if port.Direction == VMPortDirectionExport {
			vm, exist := exportPortMap[port.String()]
			if exist {
				return fmt.Errorf("vm '%s' is already exporting '%s'", vm.Config.Name, port.String())
			}
		} else if port.Direction == VMPortDirectionImport {
			reversed := *port
			reversed.Direction = VMPortDirectionExport
			for _, p := range ports {
				if p.String() == reversed.String() {
					return fmt.Errorf("cannot import one of our ports (%s)", port.String())
				}
			}

			_, exist := exportPortMap[reversed.String()]
			if !exist && log != nil {
				log.Warningf("port '%s' is not exported by anyone (yet?)", port.String())
			}
		}
	}

	return nil
}
