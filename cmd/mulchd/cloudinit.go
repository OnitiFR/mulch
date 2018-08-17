package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
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

// CloudInitCreate will create (and upload ?) the CloudInit image
func CloudInitCreate(volumeName string, template string, app *App, log *Log) error {
	// 1 - create cidata file contents

	metaData := cloudInitMetaData("testhost", "mulch-deadbeef")

	phURL := "http://" + app.Libvirt.NetworkXML.IPs[0].Address + app.Config.Listen + "/phone"

	userDataVariables := make(map[string]interface{})
	userDataVariables["_SSH_PUBKEY"] = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCyJP5W0uB1M2AGSLesePNnBmPjUzu5ruJ1AswAWfghBwvqeJUOfj1lY1P/fqKxnS/K/KBrVu3f/QwvidB2JAWlRgux9iTY+PdJIbiNxKqJhCOrpJX+CrmhOrr4d+XK10VvtLDie7ArTapkOYUHmG5tB0/Iv9lYSvnee+cNK7HIem3UL3WilOxQ+UzR98jwsm1J6BDOsh0FcpLBkFYM8tF3/f/4F0oJ0OSVyUmUdlfHpuccQf4f4mZnHpltZdjc4/C6Auxf/uJcS3mW/Qz9F1I/0LLaHjQnp76zDmLfOdaQCBNthF7RlOXr9dAam91m41oz8jUQZU2Ydg7VtlRrZj8bAJtqxvq/9XLluvU7qYgFAXWoX25NV/X1gbmDv30KmNJ35EqUz/0nNdQp2a6bF+Hc1gnAKj4Jn8e0kVLoNV5XJoIQcJ8PY9FNG7YNTQpp3gqMNOQGLZnVwbE+DCC9+Psl9XIXWscwXNeIpi30IdnVMWYmabZgoXpLVl5+eTzVCy+e8kGuPadBh5o2TPj6sTzGmdPE2BymHPDJpxcgoAnLmtNp+jdGDJgcNrLY9zBjMmqfpCX8Y66qamESys79aKhbGSp6w+29U9sD37kkEMC5NydfCAmpclAJUOIX/Ya6DYqEEqO39l2v2qRj+LdS47abuk+lfThCiLhAlVH1JnE9SQ== test.xfennec@JulienDev"
	userDataVariables["_PHONE_HOME_URL"] = phURL

	userDataTemplate := app.Config.configPath + "/templates/ci-user-data.yml"
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
		template,
		tmpfile.Name(),
		volumeName,
		log)

	if err != nil {
		return err
	}

	return nil
}
