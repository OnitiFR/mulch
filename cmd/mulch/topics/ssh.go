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

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

var sshCmdVM *common.APIVMInfos
var sshCmdUser string
var sshCmdAsAdmin bool
var sshCmdWithRevision bool
var sshCmdRemoteCommand []string

// sshCmd represents the "ssh" command
var sshCmd = &cobra.Command{
	Use:   "ssh <vm-name> [-- command...]",
	Short: "Open a SSH session",
	Long: `Open a SSH shell session to the VM, or execute a command on it.

See 'vm list' for VM Names.

Examples:
  mulch ssh myvm
  mulch ssh myvm -a
  mulch ssh myvm -- cat hello.txt
  mulch ssh myvm -a -- ls -la /etc
  mulch ssh myvm -- 'cat > file.txt' < local_file.txt
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := client.CreateSSHMulchDir()
		if err != nil {
			log.Fatal(err.Error())
		}

		sshCmdAsAdmin, _ = cmd.Flags().GetBool("admin")

		// remote command is everything after "--"
		argsBeforeDash := len(args)
		if cmd.ArgsLenAtDash() != -1 {
			argsBeforeDash = cmd.ArgsLenAtDash()
		}
		if argsBeforeDash != 1 {
			log.Fatal("usage: mulch ssh <vm-name> [-- command...]")
		}
		sshCmdRemoteCommand = args[1:]

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
		"-i", privFilePath,
		"-p", strconv.Itoa(client.GlobalConfig.Server.SSHPort),
		"-A",
		destination,
	}
	if len(sshCmdRemoteCommand) > 0 {
		// prevent ssh from parsing the remote command as its own options
		args = append(args, "--")
		args = append(args, sshCmdRemoteCommand...)
	}

	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		log.Fatalf("ssh command not found: %s", err)
	}

	cmd := exec.Command(sshPath, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		// forward remote command exit code (ssh errors are already shown on stderr)
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatal(err.Error())
	}
}

func init() {
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().BoolP("admin", "a", false, "login as admin")
	sshCmd.Flags().StringVarP(&sshCmdUser, "user", "u", "", "login user (default: app user)")
	sshCmd.Flags().StringP("revision", "r", "", "revision number")
}
