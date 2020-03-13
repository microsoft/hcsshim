package uvm

import (
	"context"
	"os"
	// "io"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/copyfile"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/pkg/security"
)

var logFile *os.File
var err error

var hcsSaveOptions = "{\"SaveType\": \"AsTemplate\"}"

func debuglog(msg string) {
	if logFile == nil {
		logFile, err = os.OpenFile("C:\\Users\\Amit\\Documents\\debuglog.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0755)
		if err != nil {
			return
		}
	}
	logFile.WriteString(msg)
}

func createCloneConfigDoc(uvm *UtilityVM) (*hcsschema.ComputeSystem) {
	cloneDoc := *uvm.configDoc
	cloneDoc.VirtualMachine.RestoreState = &hcsschema.RestoreState {}
	cloneDoc.VirtualMachine.RestoreState.TemplateSystemId = uvm.ID()
	return &cloneDoc
}

func (uvm *UtilityVM) Clone(ctx context.Context) (*UtilityVM, error) {
	err := uvm.hcsSystem.Pause(ctx)
	if err != nil {
		debuglog("1." + err.Error() + "\n")
		return nil, err
	}

	err = uvm.hcsSystem.Save(ctx, hcsSaveOptions)
	if err != nil {
		debuglog("2." + err.Error() + "\n")
		return nil, err
	}

	props, err := uvm.hcsSystem.PropertiesV2(ctx)
	if err != nil {
		debuglog("2.1." + err.Error() + "\n")
		return nil, err
	}
	debuglog("properties: " + props.State)

	cloneDoc := createCloneConfigDoc(uvm)

	srcVhdPath := uvm.configDoc.VirtualMachine.Devices.Scsi["0"].Attachments["0"].Path
	dstVhdPath := "C:\\Users\\Amit\\Documents\\sandbox.vhdx"

	// copy the VHDX of source VM
	err = copyfile.CopyFile(ctx, srcVhdPath, dstVhdPath, true)
	if err != nil {
		debuglog("3." + err.Error() + "\n")
		return nil, err
	}

	// replace the VHD path in config
	vhdAttachement := cloneDoc.VirtualMachine.Devices.Scsi["0"].Attachments["0"]
	vhdAttachement.Path = dstVhdPath
	cloneDoc.VirtualMachine.Devices.Scsi["0"].Attachments["0"] = vhdAttachement
	// Guest connection will be done externally
	cloneDoc.VirtualMachine.GuestConnection = &hcsschema.GuestConnection{}


	err = security.GrantVmGroupAccess(dstVhdPath)
	if err != nil {
		debuglog("4." + err.Error() + "\n")
		return nil, err
	}

	g, err := guid.NewV4()
	if err != nil {
		return nil, err
	}

	opts := NewDefaultOptionsWCOW(g.String(), uvm.owner)

	cloneUvm := &UtilityVM{
		id:                  g.String(),
		owner:               uvm.owner,
		operatingSystem:     "windows",
		scsiControllerCount: 1,
		vsmbDirShares:       make(map[string]*vsmbShare),
		vsmbFileShares:      make(map[string]*vsmbShare),
	}
	defer func() {
		if err != nil {
			cloneUvm.Close()
		}
	}()

	// To maintain compatability with Docker we need to automatically downgrade
	// a user CPU count if the setting is not possible.
	cloneUvm.normalizeProcessorCount(ctx, opts.ProcessorCount)
	cloneUvm.scsiLocations[0][0].hostPath = dstVhdPath


	err = cloneUvm.create(ctx, cloneDoc)
	if err != nil {
		debuglog("5." + err.Error() + "\n")
		return nil, err
	}

	err = cloneUvm.hcsSystem.Start(ctx)
	if err != nil {
		debuglog("6." + err.Error() + "\n")
		return nil, err
	}

	return cloneUvm, nil
}
