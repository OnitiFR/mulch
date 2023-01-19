package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/OnitiFR/mulch/common"
)

func cloudInitMetaData(id string, hostname string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "instance-id: %s\n", id)
	fmt.Fprintf(&buf, "local-hostname: %s\n", hostname)
	return buf.Bytes()
}

func cloudInitUserData(templateFile string, variables map[string]interface{}) ([]byte, error) {
	data, err := os.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}
	expanded := common.StringExpandVariables(string(data), variables)

	return []byte(expanded), nil
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

	userData, err := cloudInitUserData(userDataTemplate, userDataVariables)
	if err != nil {
		return "", "", err
	}

	return string(metaData), string(userData), nil
}
