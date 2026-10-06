package securitypolicy

import (
	"strings"
	"testing"
)

func TestCreateWindowsContainerPolicy(t *testing.T) {
	container, err := CreateWindowsContainerPolicy(
		[]string{"cmd.exe", "/c", "echo test"},
		[]string{"layer-1", "layer-2"},
		[]string{"merged-cim"},
		[]EnvRuleConfig{{Strategy: EnvVarRuleString, Rule: "PATH=C:\\Windows", Required: true}},
		"C:\\",
		nil,
		nil,
		true,
		"ContainerUser",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got := container.Command.Elements["0"]; got != "cmd.exe" {
		t.Fatalf("unexpected command: %q", got)
	}
	if got := container.Layers.Elements["1"]; got != "layer-2" {
		t.Fatalf("unexpected layer: %q", got)
	}
	if got := container.MountedCim; len(got) != 1 || got[0] != "merged-cim" {
		t.Fatalf("unexpected mounted CIM: %v", got)
	}
	if container.WorkingDir != "C:\\" || container.User != "ContainerUser" || !container.AllowStdioAccess {
		t.Fatalf("unexpected Windows container fields: %+v", container)
	}
}

func TestCreateWindowsContainerPolicyRejectsInvalidEnvRegex(t *testing.T) {
	_, err := CreateWindowsContainerPolicy(nil, nil, nil, []EnvRuleConfig{{
		Strategy: EnvVarRuleRegex,
		Rule:     "[",
	}}, "", nil, nil, false, "")
	if err == nil {
		t.Fatal("expected invalid environment regex to fail")
	}
}

func TestMarshalWindowsPolicy(t *testing.T) {
	container, err := CreateWindowsContainerPolicy(
		[]string{"cmd.exe"},
		[]string{"layer-hash"},
		[]string{"merged-cim-hash"},
		nil,
		"C:\\",
		nil,
		nil,
		false,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	policy, err := MarshalWindowsPolicy("rego", false, []*WindowsContainer{container}, nil, nil, false, false, false, false, false, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"layer-hash", "merged-cim-hash", "cmd.exe"} {
		if !strings.Contains(policy, expected) {
			t.Errorf("policy does not contain %q", expected)
		}
	}
}

func TestMarshalWindowsPolicyRejectsJSON(t *testing.T) {
	_, err := MarshalWindowsPolicy("json", false, nil, nil, nil, false, false, false, false, false, false, false, false, false)
	if err == nil {
		t.Fatal("expected JSON marshalling to be rejected for Windows policies")
	}
}

func TestMarshalPolicyRejectsJSON(t *testing.T) {
	_, err := MarshalPolicy("json", false, nil, nil, nil, false, false, false, false, false, false, false, false, false)
	if err == nil {
		t.Fatal("expected JSON marshalling to be rejected")
	}
}

func TestMarshalWindowsPolicyEscapesBackslashes(t *testing.T) {
	container, err := CreateWindowsContainerPolicy(
		[]string{"C:\\app\\run.exe"},
		[]string{"layer-hash"},
		[]string{"merged-cim-hash"},
		nil,
		"C:\\",
		nil,
		nil,
		false,
		"NT AUTHORITY\\SYSTEM",
	)
	if err != nil {
		t.Fatal(err)
	}

	policy, err := MarshalWindowsPolicy("rego", false, []*WindowsContainer{container}, nil, nil, false, false, false, false, false, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// An unescaped backslash before a quote (`"C:\"`) would break Rego parsing.
	if strings.Contains(policy, `"C:\"`) {
		t.Fatalf("working_dir backslash not escaped:\n%s", policy)
	}
	for _, want := range []string{`"working_dir": "C:\\"`, `"C:\\app\\run.exe"`, `"user": "NT AUTHORITY\\SYSTEM"`} {
		if !strings.Contains(policy, want) {
			t.Errorf("policy missing %q", want)
		}
	}
}

func TestMarshalWindowsFragment(t *testing.T) {
	container, err := CreateWindowsContainerPolicy(
		[]string{"cmd.exe"},
		[]string{"layer-hash"},
		[]string{"merged-cim-hash"},
		nil,
		"C:\\",
		nil,
		nil,
		false,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	fragment, err := MarshalWindowsFragment("contoso.example", "1", []*WindowsContainer{container}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"package contoso.example", "svn := \"1\"", "layer-hash", "merged-cim-hash", "cmd.exe"} {
		if !strings.Contains(fragment, expected) {
			t.Errorf("fragment does not contain %q", expected)
		}
	}
}
