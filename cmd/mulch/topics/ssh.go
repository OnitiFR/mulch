package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
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
		err := client.CreateSSHMulchDir()
		if err != nil {
			log.Fatal(err.Error())
		}

		revision, _ := cmd.Flags().GetString("revision")
		sshCmdWithRevision = false
		if revision != "" {
			sshCmdWithRevision = true
		}
		call := client.GlobalAPI.NewCall("GET", "/vm/infos/"+args[0], map[string]string{
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
	call := client.GlobalAPI.NewCall("GET", "/sshpair", map[string]string{})
	call.JSONCallback = sshCmdPairCB
	call.Do()

}

func sshCmdPairCB(reader io.Reader, headers http.Header) {
	_, privFilePath, err := client.WriteSSHPair(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	hostname, err := client.GetSSHHost()
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
		destination = user + "@" + sshCmdVM.Name + "-r" + strconv.Itoa(sshCmdVM.Revision) + "@" + hostname
	}

	// launch 'ssh' command
	args := []string{
		"ssh",
		"-i", privFilePath,
		"-p", strconv.Itoa(client.SSHPort),
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
