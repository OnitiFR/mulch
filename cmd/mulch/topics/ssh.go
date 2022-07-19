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
var sshCmdAsAdmin bool
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

		sshCmdAsAdmin, _ = cmd.Flags().GetBool("admin")

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

func sshCmdInfoCB(reader io.Reader, _ http.Header) {
	var data common.APIVMInfos
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if !data.Up {
		log.Fatal(fmt.Errorf("error, VM is not running"))
	}

	sshCmdVM = &data
	call := client.GlobalAPI.NewCall("GET", "/sshpair", map[string]string{})
	call.JSONCallback = sshCmdPairCB
	call.Do()

}

func sshCmdPairCB(reader io.Reader, _ http.Header) {
	_, privFilePath, err := client.WriteSSHPair(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	hostname, err := client.GetSSHHost()
	if err != nil {
		log.Fatal(err.Error())
	}

	user := sshCmdVM.AppUser
	if sshCmdUser != "" {
		user = sshCmdUser
	}

	if sshCmdAsAdmin {
		if sshCmdUser != "" {
			log.Fatal(fmt.Errorf("cannot use --admin and --user simultaneously"))
		}
		user = sshCmdVM.SuperUser
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
		"-p", strconv.Itoa(client.GlobalConfig.Server.SSHPort),
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
	sshCmd.Flags().BoolP("admin", "a", false, "login as admin")
	sshCmd.Flags().StringVarP(&sshCmdUser, "user", "u", "", "login user (default: app user)")
	sshCmd.Flags().StringP("revision", "r", "", "revision number")
}
