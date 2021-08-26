package exec

import "testing"

// Very rudimentary test that the os/exec fork works.
func TestExec(t *testing.T) {
	cmd := Command("ping", "127.0.0.1")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
