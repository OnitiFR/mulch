package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

type sshConfigCmdDataStruct struct {
	hostname    string
	vmList      *common.APIVMListEntries
	privKeyPath string
	all         bool
}

var sshConfigCmdData sshConfigCmdDataStruct

//  sshConfigCmd represents the "ssh-config" command
var sshConfigCmd = &cobra.Command{
	Use:   "ssh-config",
	Short: "Update local SSH config",
	Long: `Create or update your local SSH config with aliases, allowing you to
use usual ssh/scp/sftp/… commands directly with your VMs without any
aditionnal configuration:

ssh vm-mulch
scp file vm-mulch:
…

Name follows the format: {VM name}-{server name/alias}
You'll be connected as application user.

If --all is used, two aliases are available per VM:
 - myvm-mulch (application user)
 - myvm-admin-mulch (admin user)
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := client.CreateSSHMulchDir()
		if err != nil {
			log.Fatal(err.Error())
		}

		hostname, err := client.GetSSHHost()
		if err != nil {
			log.Fatal(err.Error())
		}
		sshConfigCmdData.hostname = hostname
		sshConfigCmdData.all, _ = cmd.Flags().GetBool("all")

		call := client.GlobalAPI.NewCall("GET", "/sshpair", map[string]string{})
		call.JSONCallback = sshConfigCmdPairCB
		call.Do()
	},
}

func sshConfigCmdPairCB(reader io.Reader, headers http.Header) {
	var data common.APISSHPair
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}
	// save files using current server name
	privFilePath := client.GetSSHPath(client.MulchSSHSubDir + client.SSHKeyPrefix + client.GlobalConfig.Server.Name)
	pubFilePath := privFilePath + ".pub"
	sshConfigCmdData.privKeyPath = privFilePath

	err = ioutil.WriteFile(privFilePath, []byte(data.Private), 0600)
	if err != nil {
		log.Fatal(err.Error())
	}

	err = ioutil.WriteFile(pubFilePath, []byte(data.Public), 0644)
	if err != nil {
		log.Fatal(err.Error())
	}

	call := client.GlobalAPI.NewCall("GET", "/vm", map[string]string{})
	call.JSONCallback = sshConfigCmdVMListCB
	call.Do()
}

func sshConfigCmdVMListCB(reader io.Reader, headers http.Header) {
	var data common.APIVMListEntries
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(data) == 0 {
		fmt.Printf("Currently, no VM exists. You may use 'mulch vm create'.\n")
		return
	}

	sshConfigCmdData.vmList = &data

	err = sshConfigCmdGenerate(&sshConfigCmdData)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func sshConfigCmdGenerate(conf *sshConfigCmdDataStruct) error {
	const includeString = "Include mulch/aliases_*.conf"
	configPath := client.GetSSHPath("config")
	sampleContent := `# Generated once by mulch client, feel free to edit.

Include mulch/aliases_*.conf

# Insert your usual hosts here:
# Host foobar
#	HostName foo.bar.tld
#	User foo
#	IdentityFile /home/foo/.ssh/id_foo

# do not spread our ssh keys to other hosts
Host *
    IdentitiesOnly yes
`
	filename := client.GetSSHPath(client.MulchSSHSubDir + "aliases_" + client.GlobalConfig.Server.Name + ".conf")
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("# Generated by 'mulch ssh-config' command, do not modify manually.\n# Run the command again to refresh.\n\n")

	serverName := client.GlobalConfig.Server.Name
	if client.GlobalConfig.Server.Alias != "" {
		serverName = client.GlobalConfig.Server.Alias

	}

	fmt.Println("Generated aliases:")
	for _, vm := range *conf.vmList {

		revision := ""

		if !vm.Active {
			if !conf.all {
				continue
			}
			revision = fmt.Sprintf("-r%d", vm.Revision)
		}

		aliasName := vm.Name + revision + "-" + serverName
		fmt.Printf("  %s\n", aliasName)
		file.WriteString(fmt.Sprintf("Host %s\n", aliasName))
		file.WriteString(fmt.Sprintf("    HostName %s\n", conf.hostname))
		file.WriteString(fmt.Sprintf("    IdentityFile %s\n", conf.privKeyPath))
		file.WriteString(fmt.Sprintf("    Port %d\n", client.GlobalConfig.Server.SSHPort))
		file.WriteString(fmt.Sprintf("    User %s@%s\n", vm.AppUser, vm.Name+revision))
		file.WriteString("\n")

		if conf.all {
			appAliasName := vm.Name + "-admin-" + serverName
			fmt.Printf("  %s\n", appAliasName)
			file.WriteString(fmt.Sprintf("Host %s\n", appAliasName))
			file.WriteString(fmt.Sprintf("    HostName %s\n", conf.hostname))
			file.WriteString(fmt.Sprintf("    IdentityFile %s\n", conf.privKeyPath))
			file.WriteString(fmt.Sprintf("    Port %d\n", client.GlobalConfig.Server.SSHPort))
			file.WriteString(fmt.Sprintf("    User %s@%s\n", vm.SuperUser, vm.Name+revision))
			file.WriteString("\n")
		}

		file.WriteString("\n")
	}

	includeIsHere, _ := common.FileContains(configPath, includeString)
	if !includeIsHere {
		if !common.PathExist(configPath) {
			err := ioutil.WriteFile(configPath, []byte(sampleContent), 0600)
			if err != nil {
				log.Fatal(err.Error())
			}
		} else {
			fmt.Printf(`
Warning: in order to use aliases, you should add the following line
*at the top* of your SSH config file '%s':
---
%s
---
`, configPath, includeString)
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(sshConfigCmd)
	sshConfigCmd.Flags().BoolP("all", "a", false, "generate all possible aliases (admin user, inactive VMs)")
}
