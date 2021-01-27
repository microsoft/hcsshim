package main

import (
	"context"

	"github.com/Microsoft/hcsshim/internal/clone"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

// saveAsTemplate saves the UVM and container inside it as a template and also stores the
// relevant information in the registry so that clones can be created from this template.
// Every cloned uvm gets its own NIC and we do not want to create clones of a template
// which still has a NIC attached to it. So remove the NICs attached to the template uvm
// before saving it.
// Similar to the NIC scenario we do not want to create clones from a template with an
// active GCS connection so close the GCS connection too.
func saveAsTemplate(ctx context.Context, templateTask *hcsTask) (err error) {
	var utc *uvm.UVMTemplateConfig
	var templateConfig *clone.TemplateConfig

	if err = templateTask.host.RemoveAllNICs(ctx); err != nil {
		return err
	}

	if err = templateTask.host.CloseGCSConnection(); err != nil {
		return err
	}

	utc, err = templateTask.host.GenerateTemplateConfig()
	if err != nil {
		return err
	}

	templateConfig = &clone.TemplateConfig{
		TemplateUVMID:         utc.UVMID,
		TemplateUVMResources:  utc.Resources,
		TemplateUVMCreateOpts: utc.CreateOpts,
		TemplateContainerID:   templateTask.id,
		TemplateContainerSpec: *templateTask.taskSpec,
	}

	if err = clone.SaveTemplateConfig(ctx, templateConfig); err != nil {
		return err
	}

	if err = templateTask.host.SaveAsTemplate(ctx); err != nil {
		return err
	}
	return nil
}
