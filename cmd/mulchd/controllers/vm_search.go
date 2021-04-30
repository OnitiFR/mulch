package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/Knetic/govaluate"
	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
	"github.com/ryanuber/go-glob"
)

func searchVMsFunctions(vm **server.VM) map[string]govaluate.ExpressionFunction {
	return map[string]govaluate.ExpressionFunction{

		// strlen(string) int
		"strlen": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, errors.New("strlen() need 1 argument")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("strlen() argument must be a string")
			}
			length := len(args[0].(string))
			return (float64)(length), nil
		},

		// env(string) string
		// return an empty string if the env does not exist
		"env": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, errors.New("env() need 1 argument")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("env() argument must be a string")
			}
			envName := args[0].(string)
			return (*vm).Config.Env[envName], nil
		},

		// has_domain(string) bool
		// return true if VM has the specified domain
		"has_domain": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, errors.New("has_domain() need 1 argument")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("has_domain() argument must be a string")
			}
			domainArg := args[0].(string)
			for _, domain := range (*vm).Config.Domains {
				if domain.Name == domainArg {
					return true, nil
				}
			}
			return false, nil
		},

		// has_script(type string, path string) bool
		// return true if VM has the specified script (name without path)
		"has_script": func(args ...interface{}) (interface{}, error) {
			if len(args) != 2 {
				return nil, errors.New("has_script() need 2 arguments")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("has_script() argument 1 must be a string")
			}
			if _, castOK := args[1].(string); !castOK {
				return nil, errors.New("has_script() argument 2 must be a string")
			}
			scriptType := args[0].(string)
			scriptName := args[1].(string)

			var scriptArray []*server.VMConfigScript

			switch scriptType {
			case "install":
				scriptArray = (*vm).Config.Install
			case "prepare":
				scriptArray = (*vm).Config.Prepare
			case "backup":
				scriptArray = (*vm).Config.Backup
			case "restore":
				scriptArray = (*vm).Config.Restore
			default:
				return nil, fmt.Errorf("has_script(): invalid script type '%s' (install, prepare, backup, restore)", scriptType)
			}
			for _, script := range scriptArray {
				if path.Base(script.ScriptURL) == scriptName {
					return true, nil
				}
			}
			return false, nil
		},

		// has_action(string) bool
		// return true if the VM has specified action
		"has_action": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, errors.New("has_action() need 1 argument")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("has_action() argument 1 must be a string")
			}
			action := args[0].(string)
			_, exists := (*vm).Config.DoActions[action]
			return exists, nil
		},

		// has_tag(string) bool
		// return true if the VM has specified tag
		"has_tag": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, errors.New("has_tag() need 1 argument")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("has_tag() argument 1 must be a string")
			}
			tag := args[0].(string)
			_, exists := (*vm).Config.Tags[tag]
			return exists, nil
		},

		// like(string) bool
		// return true if the wildcard match the VM's name
		"like": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, errors.New("like() need 1 argument")
			}
			if _, castOK := args[0].(string); !castOK {
				return nil, errors.New("like() argument 1 must be a string")
			}
			expr := args[0].(string)
			res := glob.Glob(expr, (*vm).Config.Name)
			return res, nil
		},
	}
}

// SearchVMsController search VMs with criteria
func SearchVMsController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "text/plain")

	failOnEmpty := false
	if req.HTTP.FormValue("fail-on-empty") == common.TrueStr {
		failOnEmpty = true
	}

	showRevision := false
	if req.HTTP.FormValue("show-revision") == common.TrueStr {
		showRevision = true
	}

	q := req.HTTP.FormValue("q")

	if strings.TrimSpace(q) == "" {
		http.Error(req.Response, "empty search expression", 400)
		return
	}

	var matches []*server.VMName
	vmNames := req.App.VMDB.GetNames()
	var currentVM *server.VM

	functions := searchVMsFunctions(&currentVM)
	expr, err := govaluate.NewEvaluableExpressionWithFunctions(q, functions)
	if err != nil {
		http.Error(req.Response, err.Error(), 400)
		return
	}

	for _, vmName := range vmNames {
		vm, err := req.App.VMDB.GetByName(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			http.Error(req.Response, msg, 500)
			return
		}

		domain, err := req.App.Libvirt.GetDomainByName(vmName.LibvirtDomainName(req.App))
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		if domain == nil {
			msg := fmt.Sprintf("VM %s: does not exists in libvirt", vmName)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		defer domain.Free()

		state, _, err := domain.GetState()
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		active, err := req.App.VMDB.IsVMActive(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		params := make(map[string]interface{})
		params["name"] = vmName.Name
		params["revision"] = vmName.Revision
		params["locked"] = vm.Locked
		params["active"] = active
		params["state"] = server.LibvirtDomainStateToString(state)
		params["author"] = vm.AuthorKey
		params["init_date"] = vm.InitDate.Unix()
		params["seed"] = vm.Config.Seed
		params["cpu_count"] = vm.Config.CPUCount
		params["ram_gb"] = (float64)(vm.Config.RAMSize) / 1024 / 1024 / 1024
		params["disk_gb"] = (float64)(vm.Config.DiskSize) / 1024 / 1024 / 1024
		params["hostname"] = vm.Config.Hostname

		currentVM = vm
		res, err := expr.Evaluate(params)
		if err != nil {
			http.Error(req.Response, err.Error(), 400)
			return
		}

		if _, castOK := res.(bool); !castOK {
			http.Error(req.Response, "require a boolean expression", 400)
			return
		}

		if res.(bool) {
			matches = append(matches, vmName)
		}
	}

	if failOnEmpty && len(matches) == 0 {
		http.Error(req.Response, "no matches", 404)
		return
	}

	for _, vmName := range matches {
		if showRevision {
			req.Printf("%s;%d\n", vmName.Name, vmName.Revision)
		} else {
			req.Println(vmName.Name)
		}
	}
}
