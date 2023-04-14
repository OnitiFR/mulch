package server

import (
	"fmt"

	"libvirt.org/go/libvirt"
)

func GetConsoleStream(vmName *VMName, app *App) (*libvirt.Stream, error) {
	vmNameID := vmName.ID()

	connect, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, err
	}

	name, err := ParseVMName(vmNameID)
	if err != nil {
		return nil, err
	}

	domainName := name.LibvirtDomainName(app)
	domain, err := app.Libvirt.GetDomainByName(domainName)

	if err != nil {
		return nil, err
	}

	stream, err := connect.NewStream(0)
	if err != nil {
		return nil, err
	}

	err = domain.OpenConsole("", stream, 0)
	if err != nil {
		stream.Abort()
		return nil, fmt.Errorf("can't open console: %s", err)
	}

	// cr.app.Log.Infof("console %s opened", domainName)

	return stream, nil
}
