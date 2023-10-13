package hcsschema

// schema reference has [InterruptModerationMode] as a string enum:
// https://learn.microsoft.com/en-us/virtualization/api/hcs/schemareference#interruptmoderationmode
//
// however, Schema.VirtualMachines.Resources.Network.mars shows it as a int enum

func (x InterruptModerationMode) Int64() (int64, error) {
	return enumLookup(map[InterruptModerationMode]int64{
		InterruptModerationMode_DEFAULT_: 0,
		InterruptModerationMode_ADAPTIVE: 1,
		InterruptModerationMode_OFF:      2,
		InterruptModerationMode_LOW:      100,
		InterruptModerationMode_MEDIUM:   200,
		InterruptModerationMode_HIGH:     300,
	}, x)
}

func NewInterruptModerationModeFromInt64(x int64) (InterruptModerationMode, error) {
	return enumLookup(map[int64]InterruptModerationMode{
		0:   InterruptModerationMode_DEFAULT_,
		1:   InterruptModerationMode_ADAPTIVE,
		2:   InterruptModerationMode_OFF,
		100: InterruptModerationMode_LOW,
		200: InterruptModerationMode_MEDIUM,
		300: InterruptModerationMode_HIGH,
	}, x)
}
