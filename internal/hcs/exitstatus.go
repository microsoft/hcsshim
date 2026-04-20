//go:build windows

package hcs

import (
	"encoding/json"
)

// systemExitStatus mirrors the HCS external schema for
// HcsEventSystemExited's EventData payload. The server (vmcompute.exe) serializes
// Schema::Responses::System::SystemExitStatus into JSON; the shim parses it back
// here. We care about Status (HRESULT) and the new ExitType added in schema 2.18
// (string rendering of the NotificationType enum: "Reboot", "GracefulExit", ...).
// Other fields on the wire (e.g. Attribution) are ignored intentionally.
type systemExitStatus struct {
	Status   int32  `json:"Status"`
	ExitType string `json:"ExitType,omitempty"`
}

// parseExitType reads a SystemExitStatus JSON document and returns the ExitType
// string. Empty input returns ("", nil) so non-exited notifications that carry
// no payload are benign. Malformed JSON returns ("", err). A well-formed document
// without the ExitType field returns ("", nil) — that's how older HCS builds
// serialize the struct.
func parseExitType(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	var st systemExitStatus
	if err := json.Unmarshal([]byte(s), &st); err != nil {
		return "", err
	}
	return st.ExitType, nil
}
