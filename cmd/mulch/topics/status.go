package topics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusFlagJSON bool

// statusCmd represents the "status" command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get informations about Mulchd host",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		statusFlagJSON, _ = cmd.Flags().GetBool("json")

		call := client.GlobalAPI.NewCall("GET", "/status", map[string]string{})
		call.JSONCallback = statusDisplay
		call.Do()
	},
}

func statusDisplay(reader io.Reader, headers http.Header) {
	if statusFlagJSON {
		io.Copy(os.Stdout, reader)
		return
	}

	var data common.APIStatus
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		log.Fatal(err.Error())
	}

	referenceTime := time.Now()
	if headers.Get("Date") != "" {
		date, err := http.ParseTime(headers.Get("Date"))
		if err == nil {
			referenceTime = date
		}
	}

	em := color.New(color.Bold, color.FgMagenta).SprintfFunc()
	var s string

	fmt.Printf("Started since: %s (%s)\n", referenceTime.Sub(data.StartTime), data.StartTime)

	fmt.Println("---")

	fmt.Printf("VMs: %d\n", data.VMs)
	if data.VMs > 0 {
		fmt.Printf("VMs running: %d (%d%%)\n", data.ActiveVMs, data.ActiveVMs*100/data.VMs)
	}

	fmt.Println("---")

	fmt.Printf("Host CPUs: %d\n", data.HostCPUs)
	if data.VMs > 0 {
		fmt.Printf("VM CPUs: %d (%d%%)\n", data.VMCPUs, data.VMCPUs*100/data.HostCPUs)
		s = em("%d%% of host CPUs", data.VMActiveCPUs*100/data.HostCPUs)
		fmt.Printf("VM active CPUs: %d (%s, %d%% of VM CPUs)\n", data.VMActiveCPUs, s, data.VMActiveCPUs*100/data.VMCPUs)
	}

	fmt.Println("---")

	fmt.Printf("Host memory: %s\n", (datasize.ByteSize(data.HostMemoryTotalMB) * datasize.MB).HR())
	if data.VMs > 0 {
		fmt.Printf("VM memory: %s (%d%%)\n", (datasize.ByteSize(data.VMMemMB) * datasize.MB).HR(), data.VMMemMB*100/data.HostMemoryTotalMB)
		s = em("%d%% of host memory", data.VMActiveMemMB*100/data.HostMemoryTotalMB)
		fmt.Printf("VM active memory: %s (%s, %d%% of VM memory)\n", (datasize.ByteSize(data.VMActiveMemMB) * datasize.MB).HR(), s, data.VMActiveMemMB*100/data.VMMemMB)
	}

	fmt.Println("---")

	// support for mulchd < 1.37.6
	if data.TotalStorageMB == 0 || data.TotalBackupMB == 0 {
		color.Red("WARNING: mulchd < 1.37.6 detected, only basic disk stats available")
		fmt.Printf("Host free disk storage: %s\n", (datasize.ByteSize(data.FreeStorageMB) * datasize.MB).HR())
		fmt.Printf("Host free backup storage: %s\n", (datasize.ByteSize(data.FreeBackupMB) * datasize.MB).HR())
		fmt.Printf("Provisioned VM storage: %s\n", (datasize.ByteSize(data.ProvisionedDisksMB) * datasize.MB).HR())
		fmt.Printf("Allocated VM storage: %s\n", (datasize.ByteSize(data.AllocatedDisksMB) * datasize.MB).HR())
	} else {
		s = em("%s free", (datasize.ByteSize(data.FreeStorageMB) * datasize.MB).HR())
		s2 := em("%d%% used", 100-(data.FreeStorageMB*100/data.TotalStorageMB))
		fmt.Printf("Host disk storage: %s available, %s (%s)\n", (datasize.ByteSize(data.TotalStorageMB) * datasize.MB).HR(), s, s2)
		fmt.Printf("Backup storage: %s available, %s free (%d%% used)\n", (datasize.ByteSize(data.TotalBackupMB) * datasize.MB).HR(), (datasize.ByteSize(data.FreeBackupMB) * datasize.MB).HR(), 100-(data.FreeBackupMB*100/data.TotalBackupMB))
		if data.VMs > 0 {
			s = em("%d%% of host storage", data.ProvisionedDisksMB*100/data.TotalStorageMB)
			fmt.Printf("Provisioned VM storage: %s (%s)\n", (datasize.ByteSize(data.ProvisionedDisksMB) * datasize.MB).HR(), s)
			fmt.Printf("Allocated VM storage: %s (%d%% of storage, %d%% of provisioned)\n", (datasize.ByteSize(data.AllocatedDisksMB) * datasize.MB).HR(), data.AllocatedDisksMB*100/data.TotalStorageMB, data.AllocatedDisksMB*100/data.ProvisionedDisksMB)
		}
	}

	fmt.Println("---")

	fmt.Printf("Origins: %d\n", len(data.Origins))
	for _, origin := range data.Origins {
		fmt.Printf("  %s: %s (%s)\n", origin.Name, origin.Path, origin.Type)
	}

	fmt.Printf("SSHConnections: %d\n", len(data.SSHConnections))
	for _, conn := range data.SSHConnections {
		since := referenceTime.Sub(conn.StartTime)
		fmt.Printf(" - from %s@%s to %s@%s (%s)\n",
			conn.FromUser,
			conn.FromIP,
			conn.ToUser,
			conn.ToVMName,
			since)
	}

	fmt.Printf("Operations: %d\n", len(data.Operations))
	for _, op := range data.Operations {
		since := referenceTime.Sub(op.StartTime)
		fmt.Printf(" - from %s: %s %s %s (%s)\n",
			op.Origin,
			op.Action,
			op.Ressource,
			op.RessourceName,
			since,
		)
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolP("json", "j", false, "show raw JSON response")
}
