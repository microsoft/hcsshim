package uvm

import (
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"golang.org/x/sys/windows"
)

// UTC has everything set to 0's. Just need to fill in the pointer fields and string identifiers.
var utcTimezone = &hcsschema.TimeZoneInformation{
	StandardName: "Coordinated Universal Time",
	DaylightName: "Coordinated Universal Time",
	StandardDate: &hcsschema.SystemTime{},
	DaylightDate: &hcsschema.SystemTime{},
}

// getTimezone returns the hosts timezone in an HCS TimeZoneInformation structure and an error if there
// is one.
func getTimezone() (*hcsschema.TimeZoneInformation, error) {
	var tz windows.Timezoneinformation
	_, err := windows.GetTimeZoneInformation(&tz)
	if err != nil {
		return nil, fmt.Errorf("failed to get time zone information: %w", err)
	}
	return tziToHCSSchema(&tz), nil
}

// TZIToHCSSchema converts a windows.TimeZoneInformation (TIME_ZONE_INFORMATION) to the hcs schema equivalent.
func tziToHCSSchema(tzi *windows.Timezoneinformation) *hcsschema.TimeZoneInformation {
	return &hcsschema.TimeZoneInformation{
		Bias:         tzi.Bias,
		StandardName: windows.UTF16ToString(tzi.StandardName[:]),
		StandardDate: &hcsschema.SystemTime{
			Year:         int32(tzi.StandardDate.Year),
			Month:        int32(tzi.StandardDate.Month),
			DayOfWeek:    int32(tzi.StandardDate.DayOfWeek),
			Day:          int32(tzi.StandardDate.Day),
			Hour:         int32(tzi.StandardDate.Hour),
			Second:       int32(tzi.StandardDate.Second),
			Minute:       int32(tzi.StandardDate.Minute),
			Milliseconds: int32(tzi.StandardDate.Milliseconds),
		},
		StandardBias: tzi.StandardBias,
		DaylightName: windows.UTF16ToString(tzi.DaylightName[:]),
		DaylightDate: &hcsschema.SystemTime{
			Year:         int32(tzi.DaylightDate.Year),
			Month:        int32(tzi.DaylightDate.Month),
			DayOfWeek:    int32(tzi.DaylightDate.DayOfWeek),
			Day:          int32(tzi.DaylightDate.Day),
			Hour:         int32(tzi.DaylightDate.Hour),
			Second:       int32(tzi.DaylightDate.Second),
			Minute:       int32(tzi.DaylightDate.Minute),
			Milliseconds: int32(tzi.DaylightDate.Milliseconds),
		},
		DaylightBias: tzi.DaylightBias,
	}
}
