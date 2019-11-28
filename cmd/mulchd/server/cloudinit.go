package server

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
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

func cloudInitExtraEnv(envMap map[string]string) string {
	res := ""
	for key, val := range envMap {
		res = res + fmt.Sprintf("export %s=\"%s\"; ", key, val)
	}
	return res
}

// CloudInitDataGen will return CloudInit meta-data and user-data
func CloudInitDataGen(vm *VM, vmName *VMName, app *App) (string, string, error) {
	userDataTemplate := app.Config.GetTemplateFilepath("ci-user-data.yml")

	phURL := "http://" + app.Libvirt.NetworkXML.IPs[0].Address + ":" + strconv.Itoa(AppInternalServerPost) + "/phone"

	sshKeyPair := app.SSHPairDB.GetByName(SSHSuperUserPair)
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
	userDataVariables["_PHONE_HOME_URL"] = phURL
	userDataVariables["_PACKAGE_UPGRADE"] = vm.Config.InitUpgrade
	userDataVariables["_MULCH_SUPER_USER"] = app.Config.MulchSuperUser
	userDataVariables["_TIMEZONE"] = vm.Config.Timezone
	userDataVariables["_APP_USER"] = vm.Config.AppUser
	userDataVariables["_VM_NAME"] = vmName.Name
	userDataVariables["_VM_REVISION"] = vmName.Revision
	userDataVariables["_KEY_DESC"] = vm.AuthorKey
	userDataVariables["_MULCH_VERSION"] = Version
	userDataVariables["_VM_INIT_DATE"] = vm.InitDate.Format(time.RFC3339)
	userDataVariables["_DOMAINS"] = strings.Join(domains, ",")
	userDataVariables["_DOMAIN_FIRST"] = firstDomain
	userDataVariables["__EXTRA_ENV"] = cloudInitExtraEnv(vm.Config.Env)

	userData, err := cloudInitUserData(userDataTemplate, userDataVariables)
	if err != nil {
		return "", "", err
	}

	return string(metaData), string(userData), nil
}
