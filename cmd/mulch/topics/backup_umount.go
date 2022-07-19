package topics

import (
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/OnitiFR/mulch/common"
	"github.com/spf13/cobra"
)

// backupuUmountCmd represents the "backup umount" command
var backupUmountCmd = &cobra.Command{
	Use:   "umount <mount-point>",
	Short: "Unmount a backup image",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		mountPoint := args[0]

		guestunmountPath, err := exec.LookPath("guestunmount")
		if err != nil {
			log.Fatalf("guestunmount command not found: %s", err)
		}

		if !common.PathExist(mountPoint) {
			log.Fatalf("mount point '%s' does not exists", mountPoint)
		}

		// launch 'ssh' command
		cmdArgs := []string{
			"guestunmount",
			mountPoint,
		}

		err = syscall.Exec(guestunmountPath, cmdArgs, os.Environ())
		log.Fatal(err.Error())
	},
}

func init() {
	backupCmd.AddCommand(backupUmountCmd)
}
