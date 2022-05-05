package constants

const (
	PlatformWindows    = "windows"
	PlatformLinux      = "linux"
	SnapshotterWindows = "windows"
	SnapshotterLinux   = "windows-lcow"
)

func SnapshotterFromPlatform(platform string) string {
	if platform == PlatformWindows {
		return SnapshotterWindows
	}
	return SnapshotterLinux
}
