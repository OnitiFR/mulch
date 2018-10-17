package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"

	"github.com/Xfennec/mulch/common"
	"github.com/spf13/cobra"
)

var sshCmdVm string

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

		sshCmdVm = args[0]

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

	// launch 'ssh' command
	cmd := exec.Command("ssh", "-i", privFilePath, "-p", strconv.Itoa(sshPort), "admin@"+sshCmdVm+"@"+globalConfig.Server.URL)
	fmt.Println(cmd)
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
