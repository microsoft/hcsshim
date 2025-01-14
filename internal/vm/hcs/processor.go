//go:build windows

package hcs

func (uvmb *utilityVMBuilder) SetProcessorCount(count uint32) error {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.Count = count
	return nil
}
