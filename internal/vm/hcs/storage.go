package hcs

func (uvmb *utilityVMBuilder) SetStorageQos(iopsMaximum int64, bandwidthMaximum int64) error {
	uvmb.doc.VirtualMachine.StorageQoS.BandwidthMaximum = int32(bandwidthMaximum)
	uvmb.doc.VirtualMachine.StorageQoS.IopsMaximum = int32(iopsMaximum)
	return nil
}
