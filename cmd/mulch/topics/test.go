package topics

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

var testCmd = &cobra.Command{
	Use:   "test <vm-name>",
	Short: "Server test call",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// call := globalAPI.NewCall("POST", "/test/"+args[0], map[string]string{})
		// call.Do()

		call := globalAPI.NewCall("GET", "/sshpair", map[string]string{})
		call.JSONCallback = testCmdPairCB
		call.Do()

	},
}

func testCmdPairCB(reader io.Reader, headers http.Header) {

	_, privKeyFile, err := WriteSSHPair(reader)
	if err != nil {
		log.Fatal(err.Error())
	}

	user := "app@mini"
	remote := "localhost"
	port := ":8022"

	// get host public key
	hostKey := getHostKey("[" + remote + "]" + port)

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			publicKeyAuthFromPubFile(privKeyFile),
		},
		// HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}

	// connect
	conn, err := ssh.Dial("tcp", remote+port, config)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// create new SFTP client
	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("connected")
	defer client.Close()

}

func publicKeyAuthFromPubFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

func getHostKey(host string) ssh.PublicKey {
	// parse OpenSSH known_hosts file
	// ssh or use ssh-keyscan to get initial key
	file, err := os.Open(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var hostKey ssh.PublicKey
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 3 {
			continue
		}
		if fields[0] == host {
			var err error
			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(scanner.Bytes())
			if err != nil {
				log.Fatalf("error parsing %q: %v", fields[2], err)
			}
			break
		}
	}

	if hostKey == nil {
		log.Fatalf("no hostkey found for %s", host)
	}

	return hostKey
}

func init() {
	rootCmd.AddCommand(testCmd)
}
