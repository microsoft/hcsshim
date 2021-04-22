package hcs

func (uvmb *utilityVMBuilder) SetProcessorCount(count uint32) error {
	uvmb.doc.VirtualMachine.ComputeTopology.Processor.Count = int32(count)
	return nil
}
