package topics

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// backupMountCmd represents the "backup mount" command
var backupMountCmd = &cobra.Command{
	Use:   "mount <disk-name.qcow2> <mount-point>",
	Short: "Mount a backup image",
	Long: `Mount a backup image on a mount as a user.

The command 'guestmount' must be installed on your system. Usual package
names are : guestmount, libguestfs, libguestfs-tools.
(libguestfs is based on libvirt, so be prepared for a few dependencies…)

Warning: use 'mulch backup umount' command, not the system's 'umount'.
	`,
	Args: cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		backupFile := args[0]
		mountPoint := args[1]

		guestmountPath, err := exec.LookPath("guestmount")
		if err != nil {
			log.Fatalf("guestmount command not found: %s (see help: mulch backup mount -h)", err)
		}

		if !common.PathExist(backupFile) {
			log.Fatalf("backup file '%s' does not exists", backupFile)
		}

		if !common.PathExist(mountPoint) {
			log.Fatalf("mount point '%s' does not exists", mountPoint)
		}

		cmdArgs := []string{
			"guestmount",
			"-a", backupFile,
			"-m", "/dev/sda",
			mountPoint,
		}

		cmd := exec.Command(guestmountPath, cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		err = cmd.Run()
		if err != nil {
			fmt.Println(err.Error())
			fmt.Println("---")
			fmt.Println("mulch hint: on some systems, access to kernel images is denied for users.")
			fmt.Println("If the error is related to this issue, you can reset access as a workaround:")
			fmt.Println("  sudo chmod 644", "/boot/vmlinuz-$(uname -r)")
			fmt.Println("This is supposedly (and arguably) a dangerous fix, but see by yourself:")
			fmt.Println("https://bugs.launchpad.net/fuel/+bug/1467579")
			os.Exit(1)
		}
	},
}

func init() {
	backupCmd.AddCommand(backupMountCmd)
}
