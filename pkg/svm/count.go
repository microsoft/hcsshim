package svm

// Count returns the number of service VMs in this instance
func (i *instance) Count() int {
	i.Lock()
	defer i.Unlock()
	if i.serviceVMs == nil {
		return 0
	}
	return len(i.serviceVMs)
}
