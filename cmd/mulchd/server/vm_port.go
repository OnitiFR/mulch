package server

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VMPort constants
const (
	VMPortProtocolTCP = 0

	VMPortDirectionExport  = 0
	VMPortDirectionImport  = 1
	VMPortDirectionInvalid = -1

	VMPortPublic = "@PUBLIC"
)

// VMPortBaseForward is the value to add to port index
// (ex : first listening port will be 9001, 2nd will be 9002, …)
const VMPortBaseForward uint16 = 9001

// VMPortMaxRangeSize is the maximum size of a port range
// This value is currently very arbitrary, we'll see.
const VMPortMaxRangeSize = 20

// VMPortProxyProtocolDefault is the default port where the PROXY protocol
// server is available in the VM
const VMPortProxyProtocoDefault = 8443

// VMPort is a network port inside a VM
type VMPort struct {
	Port       uint16
	Protocol   int // tcp (VMPortProtocol*)
	Direction  int // export / import
	Index      int // position in the direction (ex: 2nd exported port), 0 indexed
	Group      string
	PublicPort uint16 // exported PUBLIC port (0 = private)
	ProxyPort  uint16 // "PROXY protocol" port (0 = no proxy)
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

	publicPort := ""
	if p.PublicPort != 0 {
		publicPort = fmt.Sprintf(":%d", p.PublicPort)
	}

	return fmt.Sprintf("%d/%s%s%s%s", p.Port, proto, arrow, p.Group, publicPort)
}

// GlobalID will return a "global" ID for the port (public ports are merged)
// Useful for deduplication
func (p *VMPort) GlobalID() string {
	if p.PublicPort != 0 {
		return fmt.Sprintf("...->%s:%d", p.Group, p.PublicPort)
	}
	return p.String()
}

// NewVMPortArray will parse an array of strings and return an array of *VMPort
func NewVMPortArray(strPorts []string) ([]*VMPort, error) {
	var ports []*VMPort
	importIndex := 0
	exportIndex := 0

	var strPortsAll []string

	// 1 --- "explode" port ranges

	// accepts:
	// 2000-2010/tcp->@xxx
	// 2000-2010/tcp -> @xxx
	// 2000-2010/tcp -> @xxx:3000
	// 2000-2010/tcp -> @xxx:3000-3010
	isRange := regexp.MustCompile(`^(\d+)-(\d+)/(tcp) *(<-|->) *(@[A-Za-z0-9_]+)(:(\d+)(-(\d+))?)?$`)

	for _, lineOrg := range strPorts {
		line, comment := portExtractComment(lineOrg)

		line = strings.TrimSpace(line)
		match := isRange.MatchString(line)
		if !match {
			strPortsAll = append(strPortsAll, lineOrg)
			continue
		}

		items := isRange.FindStringSubmatch(line)

		srcStart, err := strconv.Atoi(items[1])
		if err != nil {
			return nil, fmt.Errorf("invalid source port range '%s' in '%s'", items[1], line)
		}

		srcEnd, err := strconv.Atoi(items[2])
		if err != nil {
			return nil, fmt.Errorf("invalid source port range '%s' in '%s'", items[2], line)
		}

		proto := items[3]
		direction := items[4]
		group := items[5]

		dstStart := srcStart
		dstEnd := srcEnd

		if items[7] != "" {
			dstStart, err = strconv.Atoi(items[7])
			if err != nil {
				return nil, fmt.Errorf("invalid destination port range '%s' in '%s'", items[7], line)
			}

			if items[9] != "" {
				dstEnd, err = strconv.Atoi(items[9])
				if err != nil {
					return nil, fmt.Errorf("invalid destination port range '%s' in '%s'", items[9], line)
				}
			} else {
				dstEnd = dstStart + (srcEnd - srcStart)
			}
		}

		if (srcEnd - srcStart) != (dstEnd - dstStart) {
			return nil, fmt.Errorf("source and destination port ranges sizes are not the same in '%s'", line)
		}

		rangeSize := srcEnd - srcStart + 1

		if rangeSize > VMPortMaxRangeSize {
			return nil, fmt.Errorf("port range is too big in '%s' (maximum: %d ports)", line, VMPortMaxRangeSize)
		}

		if rangeSize < 2 {
			return nil, fmt.Errorf("port range is too small in '%s'", line)
		}

		for i := 0; i < rangeSize; i++ {
			if srcStart == dstStart {
				str := fmt.Sprintf("%d/%s %s %s", srcStart+i, proto, direction, group)
				if comment != "" {
					str += fmt.Sprintf(" (%s)", comment)
				}
				strPortsAll = append(strPortsAll, str)
			} else {
				str := fmt.Sprintf("%d/%s %s %s:%d", srcStart+i, proto, direction, group, dstStart+i)
				if comment != "" {
					str += fmt.Sprintf(" (%s)", comment)
				}
				strPortsAll = append(strPortsAll, str)
			}
		}
	}

	// 2 --- parse ports
	for _, orgLine := range strPortsAll {
		var port VMPort
		var sep string

		line, comment := portExtractComment(orgLine)

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
		if portNum < 1 || portNum > 65535 {
			return nil, fmt.Errorf("port '%d' is out of bound ", portNum)
		}

		if portParts[1] != "tcp" {
			return nil, fmt.Errorf("only tcp protocol is supported (%s)", line)
		}

		port.Protocol = VMPortProtocolTCP
		port.Port = uint16(portNum)

		group := strings.TrimSpace(lineParts[1])
		if !strings.HasPrefix(group, VMPortPublic) {
			// private
			lgroup := strings.ToLower(group)
			if !IsValidGroupName(lgroup) {
				return nil, fmt.Errorf("invalid group name '%s' (ex: @my_group)", lgroup)
			}
			port.Group = lgroup
		} else {
			// public
			if port.Direction != VMPortDirectionExport {
				return nil, errors.New("you can only export a public port (use direct connection to use it)")
			}

			groupParts := strings.Split(group, ":")
			if groupParts[0] != VMPortPublic {
				return nil, fmt.Errorf("invalid public group name '%s'", group)
			}
			port.Group = VMPortPublic

			switch len(groupParts) {
			case 1:
				port.PublicPort = port.Port
			case 2:
				groupPortNum, err := strconv.Atoi(groupParts[1])
				if err != nil {
					return nil, fmt.Errorf("cannot parse port as an integer in '%s'", groupParts[1])
				}
				if groupPortNum < 1 || groupPortNum > 65535 {
					return nil, fmt.Errorf("port '%d' is out of bound ", groupPortNum)
				}
				port.PublicPort = uint16(groupPortNum)
			default:
				return nil, fmt.Errorf("invalid public group '%s' (ex: PUBLIC:8080)", group)
			}

			// check if we found a PROXY comment
			parts := strings.Split(comment, ":")
			if len(parts) > 0 && parts[0] == "PROXY" {
				port.ProxyPort = VMPortProxyProtocoDefault
				if len(parts) == 2 {
					p, err := strconv.Atoi(parts[1])
					if err != nil {
						return nil, fmt.Errorf("cannot parse PROXY port as an integer in '%s'", parts[1])
					}
					port.ProxyPort = uint16(p)
				}
			}
		}

		// check duplicates
		for _, p := range ports {
			if port.Port == p.Port && port.Direction == p.Direction && port.Protocol == p.Protocol && port.Group == p.Group && port.PublicPort == p.PublicPort {
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
				exportPortMap[port.GlobalID()] = entry.VM
			}
		}
	}

	// search duplicate imports, warn about missing imports
	for _, port := range ports {
		if port.Direction == VMPortDirectionExport {
			vm, exist := exportPortMap[port.GlobalID()]
			if exist {
				return fmt.Errorf("vm '%s' is already exporting '%s'", vm.Config.Name, port.GlobalID())
			}
		} else if port.Direction == VMPortDirectionImport {
			reversed := *port
			reversed.Direction = VMPortDirectionExport
			for _, p := range ports {
				if p.GlobalID() == reversed.GlobalID() {
					return fmt.Errorf("cannot import one of our ports (%s)", port.GlobalID())
				}
			}

			_, exist := exportPortMap[reversed.GlobalID()]
			if !exist && log != nil {
				log.Warningf("port '%s' is not exported by anyone (yet?)", port.GlobalID())
			}
		}
	}

	return nil
}

// portExtractComment extracts the (comment) from a port string
func portExtractComment(line string) (port string, comment string) {
	re := regexp.MustCompile(`^(.*) (\((.*)\))?$`)
	matches := re.FindStringSubmatch(line)

	if len(matches) != 4 {
		return line, ""
	}

	return matches[1], matches[3]
}
