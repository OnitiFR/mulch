package topics

import (
	"log"
	"os"
	"os/exec"
	"syscall"

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

		// launch 'ssh' command
		cmdArgs := []string{
			"guestmount",
			"-a", backupFile,
			"-m", "/dev/sda",
			mountPoint,
		}

		err = syscall.Exec(guestmountPath, cmdArgs, os.Environ())
		log.Fatal(err.Error())
	},
}

func init() {
	backupCmd.AddCommand(backupMountCmd)
}
