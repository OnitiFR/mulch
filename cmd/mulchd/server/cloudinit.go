package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"time"
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
	expanded := StringExpandVariables(string(data), variables)

	return []byte(expanded), nil
}

func cloudInitExtraEnv(envMap map[string]string) string {
	res := ""
	for key, val := range envMap {
		res = res + fmt.Sprintf("export %s=\"%s\"; ", key, val)
	}
	return res
}

// CloudInitCreate will create (and upload ?) the CloudInit image
func CloudInitCreate(volumeName string, vm *VM, app *App, log *Log) error {
	volTemplate := app.Config.configPath + "/templates/volume.xml"
	userDataTemplate := app.Config.configPath + "/templates/ci-user-data.yml"

	phURL := "http://" + app.Libvirt.NetworkXML.IPs[0].Address + app.Config.Listen + "/phone"
	SSHPub, err := ioutil.ReadFile(app.Config.MulchSSHPublicKey)
	if err != nil {
		return err
	}

	// 1 - create cidata file contents
	metaData := cloudInitMetaData(vm.SecretUUID, vm.Config.Hostname)

	userDataVariables := make(map[string]interface{})
	userDataVariables["_SSH_PUBKEY"] = SSHPub
	userDataVariables["_PHONE_HOME_URL"] = phURL
	userDataVariables["_PACKAGE_UPGRADE"] = vm.Config.InitUpgrade
	userDataVariables["_MULCH_SUPER_USER"] = app.Config.MulchSuperUser
	userDataVariables["_TIMEZONE"] = vm.Config.Timezone
	userDataVariables["_APP_USER"] = vm.Config.AppUser
	userDataVariables["_VM_NAME"] = vm.Config.Name
	userDataVariables["_KEY_DESC"] = vm.AuthorKey
	userDataVariables["_MULCH_VERSION"] = Version
	userDataVariables["_VM_INIT_DATE"] = time.Now().Format(time.RFC3339)
	userDataVariables["__EXTRA_ENV"] = cloudInitExtraEnv(vm.Config.Env)

	userData, err := cloudInitUserData(userDataTemplate, userDataVariables)
	if err != nil {
		return err
	}

	// 2 - build image
	contents := []CIFFile{
		CIFFile{Filename: "meta-data", Content: metaData},
		CIFFile{Filename: "user-data", Content: userData},
	}
	tmpfile, err := ioutil.TempFile("", "mulch-ci-image")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name())

	// tmpfile will be closed by CloudInitFatCreateImage, no matter what
	err = CloudInitFatCreateImage(tmpfile, 256*1024, contents)
	if err != nil {
		return err
	}

	// 3 - upload imaage to storage pool
	err = app.Libvirt.UploadFileToLibvirt(
		app.Libvirt.Pools.CloudInit,
		app.Libvirt.Pools.CloudInitXML,
		volTemplate,
		tmpfile.Name(),
		volumeName,
		log)

	if err != nil {
		return err
	}

	return nil
}
