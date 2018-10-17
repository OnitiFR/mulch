package topics

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"syscall"

	"github.com/Xfennec/mulch/common"
	"github.com/spf13/cobra"
)

var sshCmdVM string

//  sshCmd represents the "ssh" command
var sshCmd = &cobra.Command{
	Use:   "ssh <vm-name>",
	Short: "Open a SSH session",
	Long: `Open a SSH shell session to the VM.
See 'vm list' for VM Names.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := CreateSSHMulchDir()
		if err != nil {
			log.Fatal(err.Error())
		}

		sshCmdVM = args[0]

		call := globalAPI.NewCall("GET", "/sshpair", map[string]string{})
		call.JSONCallback = sshCB
		call.Do()
	},
}

func sshCB(reader io.Reader) {
	var data common.SSHPair
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}
	// save files using current server name
	privFilePath := GetSSHPath(mulchSubDir + sshKeyPrefix + globalConfig.Server.Name)
	pubFilePath := privFilePath + ".pub"

	err = ioutil.WriteFile(privFilePath, []byte(data.Private), 0600)
	if err != nil {
		log.Fatal(err.Error())
	}

	err = ioutil.WriteFile(pubFilePath, []byte(data.Public), 0644)
	if err != nil {
		log.Fatal(err.Error())
	}

	hostname, err := GetSSHHost()
	if err != nil {
		log.Fatal(err.Error())
	}

	// should get that dynamically from the VM
	// (and it would allow to validate VM name early!)
	user := "admin"

	// launch 'ssh' command
	args := []string{
		"ssh",
		"-i", privFilePath,
		"-p", strconv.Itoa(sshPort),
		user+"@"+sshCmdVM+"@"+hostname,
	}

	err = syscall.Exec("/usr/bin/ssh", args, os.Environ())
	log.Fatal(err.Error())
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
