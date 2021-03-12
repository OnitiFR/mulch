package controllers

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// GetStateZipController return a ZIP file with VMs states and config
func GetStateZipController(req *server.Request) {
	req.Response.Header().Set("Content-Type", "application/octet-stream")

	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)

	csvBuffer := new(bytes.Buffer)
	csvWriter := csv.NewWriter(csvBuffer)
	csvWriter.Comma = ';'

	var csvLines = [][]string{
		{"Name", "Revision", "Active", "Locked", "State", "Author"},
	}

	vmNames := req.App.VMDB.GetNames()
	for _, vmName := range vmNames {
		vm, err := req.App.VMDB.GetByName(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		domain, err := req.App.Libvirt.GetDomainByName(vmName.LibvirtDomainName(req.App))
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		if domain == nil {
			msg := fmt.Sprintf("VM %s: does not exists in libvirt", vmName)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}
		defer domain.Free()

		state, _, err := domain.GetState()
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		active, err := req.App.VMDB.IsVMActive(vmName)
		if err != nil {
			msg := fmt.Sprintf("VM %s: %s", vmName, err)
			req.App.Log.Error(msg)
			http.Error(req.Response, msg, 500)
			return
		}

		csvLines = append(csvLines, []string{
			vmName.Name,
			strconv.Itoa(vmName.Revision),
			strconv.FormatBool(active),
			strconv.FormatBool(vm.Locked),
			server.LibvirtDomainStateToString(state),
			vm.AuthorKey,
		})

		filename := fmt.Sprintf("%s.toml", vmName.ID())
		zipFile, err := zipWriter.Create(filename)
		if err != nil {
			req.App.Log.Error(err.Error())
			http.Error(req.Response, err.Error(), 500)
		}
		_, err = zipFile.Write([]byte(vm.Config.FileContent))
		if err != nil {
			req.App.Log.Error(err.Error())
			http.Error(req.Response, err.Error(), 500)
		}
	}

	// write to CSV buffer
	csvWriter.WriteAll(csvLines)
	if err := csvWriter.Error(); err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}

	// add CSV file to the ZIP
	zipFile, err := zipWriter.Create("_states.csv")
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
	_, err = zipFile.Write(csvBuffer.Bytes())
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}

	// flush the ZIP
	err = zipWriter.Close()
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}

	req.Response.Write(zipBuffer.Bytes())
	req.App.Log.Trace("client downloaded ZIP state")
}
