package topics

import (
	"github.com/spf13/cobra"
)

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backups management",
	Long: `Manage VM backups.

Example of how to access data inside a backup:

 * using NBD: (modprobe nbd)
  - qemu-nbd -c /dev/nbd0 <my-backup.qcow2>
  - mount /dev/nbd0 </mnt/disk>
  - …profit…
  - umount </mnt/disk> && qemu-nbd -c /dev/nbd0

 * using guestmount / libguestfs:
  - guestmount -a <my-backup.qcow2> -m /dev/sda </mnt/disk>
  - use guestunmount to unmount
`,
}

func init() {
	rootCmd.AddCommand(backupCmd)
}
