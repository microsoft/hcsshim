package hcsschema

import (
	"fmt"
	"strings"
)

func (ev *ErrorEvent) String() string {
	b := strings.Builder{}

	// [strings.Builder.WriteString] always returns nil
	_, _ = b.WriteString("[Event Detail: " + ev.Message)
	if ev.StackTrace != "" {
		_, _ = b.WriteString(" Stack Trace: " + ev.StackTrace)
	}
	if ev.Provider != nil {
		_, _ = b.WriteString(" Provider: " + ev.Provider.String())
	}
	if ev.EventID != 0 {
		_, _ = b.WriteString(fmt.Sprintf(" EventID: %d", ev.EventID))
	}
	if ev.Flags != 0 {
		_, _ = b.WriteString(fmt.Sprintf(" flags: %d", ev.Flags))
	}
	if ev.Source != "" {
		_, _ = b.WriteString(" Source: " + ev.Source)
	}
	for i, d := range ev.Data {
		if i == 0 {
			_, _ = b.WriteString(" EventData:")
		}
		_, _ = b.WriteString(" ")
		if d.Type_ != nil {
			_, _ = b.WriteString(string(*d.Type_))
		}
		_, _ = b.WriteString("(" + d.Value + ")")
	}
	_, _ = b.WriteString("]")
	return b.String()
}
