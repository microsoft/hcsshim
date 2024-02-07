package computecore

type HCSResourceType uint32

//go:generate go run golang.org/x/tools/cmd/stringer -type=HCSResourceType -trimprefix=ResourceType resource_type.go

const (
	ResourceTypeNone      = HCSResourceType(0)
	ResourceTypeFile      = HCSResourceType(1)
	ResourceTypeJob       = HCSResourceType(2)
	ResourceTypeComObject = HCSResourceType(3)
	ResourceTypeSocket    = HCSResourceType(4)
)

type HCSResourceUri string

const (
	HCSMemoryJobUri = HCSResourceUri("hcs:/VirtualMachine/VmmemJob")
	HCSCpuJobUri    = HCSResourceUri("hcs:/VirtualMachine/WorkerJob")
)

type HCSJobResource struct {
	Name string
	Uri  HCSResourceUri
}

func NewMemoryPoolResource(name string) HCSJobResource {
	return HCSJobResource{
		Name: name,
		Uri:  HCSMemoryJobUri,
	}
}

func NewCPUPoolResource(name string) HCSJobResource {
	return HCSJobResource{
		Name: name,
		Uri:  HCSCpuJobUri,
	}
}
