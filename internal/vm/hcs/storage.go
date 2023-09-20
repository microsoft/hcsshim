//go:build windows

package hcs

func (uvmb *utilityVMBuilder) SetStorageQos(iopsMaximum int64, bandwidthMaximum int64) error {
	uvmb.doc.VirtualMachine.StorageQoS.BandwidthMaximum = uint64(bandwidthMaximum)
	uvmb.doc.VirtualMachine.StorageQoS.IOPSMaximum = uint64(iopsMaximum)
	return nil
}
