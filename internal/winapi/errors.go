package winapi

//sys RtlNtStatusToDosError(status uint32) (winerr error) = ntdll.RtlNtStatusToDosError

const (
	STATUS_REPARSE_POINT_ENCOUNTERED = 0xC000050B
	ERROR_NO_MORE_ITEMS              = 0x103
)

func NTSuccess(status uint32) bool {
	return status == 0
}
