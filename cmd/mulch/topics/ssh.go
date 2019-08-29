package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

var sshCmdVM *common.APIVMInfos
var sshCmdUser string
var sshCmdWithRevision bool

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

		revision, _ := cmd.Flags().GetString("revision")
		sshCmdWithRevision = false
		if revision != "" {
			sshCmdWithRevision = true
		}
		call := globalAPI.NewCall("GET", "/vm/infos/"+args[0], map[string]string{
			"revision": revision,
		})
		call.JSONCallback = sshCmdInfoCB
		call.Do()
	},
}

func sshCmdInfoCB(reader io.Reader, headers http.Header) {
	var data common.APIVMInfos
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if data.Up == false {
		log.Fatal(fmt.Errorf("error, VM is not running"))
	}

	sshCmdVM = &data
	call := globalAPI.NewCall("GET", "/sshpair", map[string]string{})
	call.JSONCallback = sshCmdPairCB
	call.Do()

}

func sshCmdPairCB(reader io.Reader, headers http.Header) {
	var data common.APISSHPair
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

	user := sshCmdVM.SuperUser
	if sshCmdUser != "" {
		user = sshCmdUser
	}

	// legacy (no revision) destination
	destination := user + "@" + sshCmdVM.Name + "@" + hostname
	if sshCmdWithRevision {
		destination = user + "@" + sshCmdVM.Name + "-" + strconv.Itoa(sshCmdVM.Revision) + "@" + hostname
	}

	// launch 'ssh' command
	args := []string{
		"ssh",
		"-i", privFilePath,
		"-p", strconv.Itoa(sshPort),
		destination,
	}

	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		log.Fatalf("ssh command not found: %s", err)
	}

	err = syscall.Exec(sshPath, args, os.Environ())
	log.Fatal(err.Error())
}

func init() {
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().StringVarP(&sshCmdUser, "user", "u", "", "login user (default: super user)")
	sshCmd.Flags().StringP("revision", "r", "", "revision number")
}
