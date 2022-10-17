package server

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/common"
)

func cloudInitMetaData(id string, hostname string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "instance-id: %s\n", id)
	fmt.Fprintf(&buf, "local-hostname: %s\n", hostname)
	return buf.Bytes()
}

func cloudInitUserData(templateFile string, variables map[string]interface{}) ([]byte, error) {
	data, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}
	expanded := common.StringExpandVariables(string(data), variables)

	return []byte(expanded), nil
}

func cloudInitGenExports(envMap map[string]interface{}) string {
	res := ""
	for key, val := range envMap {
		res = res + fmt.Sprintf("export %s=\"%s\"; ", key, common.InterfaceValueToString(val))
	}
	return res
}

// CloudInitDataGen will return CloudInit meta-data and user-data
func CloudInitDataGen(vm *VM, vmName *VMName, app *App) (string, string, error) {
	userDataTemplate := app.Config.GetTemplateFilepath("ci-user-data.yml")

	mulchIP := app.Libvirt.NetworkXML.IPs[0].Address

	homeURL := "http://" + mulchIP + ":" + strconv.Itoa(app.Config.InternalServerPort)
	phURL := homeURL + "/phone"

	sshKeyPair := app.SSHPairDB.GetByName(vm.MulchSuperUserSSHKey)
	if sshKeyPair == nil {
		return "", "", errors.New("can't find SSH super user key pair")
	}

	var domains []string
	var firstDomain string
	for index, domain := range vm.Config.Domains {
		if index == 0 {
			firstDomain = domain.Name
		}
		domains = append(domains, domain.Name)
	}

	// 1 - create cidata file contents
	metaData := cloudInitMetaData(vm.SecretUUID, vm.Config.Hostname)

	// DO NOT FORGET TO UPDATE ci-user-data.yml TEMPLATE TOO!
	userDataVariables := make(map[string]interface{})
	userDataVariables["_SSH_PUBKEY"] = sshKeyPair.Public
	userDataVariables["_PACKAGE_UPGRADE"] = vm.Config.InitUpgrade
	userDataVariables["_HOME_URL"] = homeURL
	userDataVariables["_PHONE_HOME_URL"] = phURL
	userDataVariables["_TIMEZONE"] = vm.Config.Timezone
	userDataVariables["_MULCH_SUPER_USER"] = app.Config.MulchSuperUser
	userDataVariables["_APP_USER"] = vm.Config.AppUser

	mainEnv := make(map[string]interface{})
	mainEnv["_MULCH_SUPER_USER"] = app.Config.MulchSuperUser
	mainEnv["_BACKUP"] = "/mnt/backup"
	mainEnv["_APP_USER"] = vm.Config.AppUser
	mainEnv["_VM_NAME"] = vmName.Name
	mainEnv["_VM_REVISION"] = vmName.Revision
	mainEnv["_KEY_DESC"] = vm.AuthorKey
	mainEnv["_MULCH_VERSION"] = Version
	mainEnv["_VM_INIT_DATE"] = vm.InitDate.Format(time.RFC3339)
	mainEnv["_DOMAINS"] = strings.Join(domains, ",")
	mainEnv["_DOMAIN_FIRST"] = firstDomain
	mainEnv["_MULCH_PROXY_IP"] = mulchIP
	mainEnv["_PORT1"] = VMPortBaseForward + 0
	mainEnv["_PORT2"] = VMPortBaseForward + 1
	mainEnv["_PORT3"] = VMPortBaseForward + 2
	mainEnv["_PORT4"] = VMPortBaseForward + 3
	mainEnv["_PORT5"] = VMPortBaseForward + 4
	for _, port := range vm.Config.Ports {
		if port.Direction == VMPortDirectionImport {
			name := fmt.Sprintf("_%d_%s", port.Port, "TCP")
			_, exists := mainEnv[name]
			if exists {
				// name conflict, force the user to use _PORTx
				mainEnv[name] = "CONFLICT"
				continue
			}
			mainEnv[name] = VMPortBaseForward + uint16(port.Index)
		}
	}

	// build extra env map
	extraMap := make(map[string]string)

	for key, val := range vm.Config.Env {
		extraMap[key] = val
	}

	for _, keyPath := range vm.Config.Secrets {
		secret, err := app.SecretsDB.Get(keyPath)
		if err != nil {
			app.Log.Errorf("error with secret '%s' for %s: %s", keyPath, vmName, err)
			continue
		}
		key := filepath.Base(keyPath)

		extraMap[key] = secret.Value
	}

	userDataVariables["__MAIN_ENV"] = cloudInitGenExports(mainEnv)
	userDataVariables["__EXTRA_ENV"] = cloudInitGenExports(common.MapStringToInterface(extraMap))

	userData, err := cloudInitUserData(userDataTemplate, userDataVariables)
	if err != nil {
		return "", "", err
	}

	return string(metaData), string(userData), nil
}
