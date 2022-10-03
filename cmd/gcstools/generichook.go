//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"text/template"

	"github.com/Microsoft/hcsshim/cmd/gcstools/generichook"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func runGenericHook() error {
	state, err := loadHookState(os.Stdin)
	if err != nil {
		return err
	}

	var (
		tctx = newTemplateContext(state)
		args = []string(os.Args[1:])
		env  = os.Environ()
	)

	parsedArgs, err := render(args, tctx)
	if err != nil {
		return err
	}
	parsedEnv, err := render(env, tctx)
	if err != nil {
		return err
	}

	hookCmd := exec.Command(parsedArgs[0], parsedArgs[1:]...)
	hookCmd.Env = parsedEnv

	out, err := hookCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run nvidia cli tool with: %v, %v", string(out), err)
	}

	return nil
}

func logDebugFile(debugFilePath string) {
	contents, err := os.ReadFile(debugFilePath)
	if err != nil {
		logrus.Errorf("failed to read debug file at %s: %v", debugFilePath, err)
		return
	}
	numBytesInContents := len(contents)

	// since we forward logs on windows to etw, limit log size to 8KB to avoid issues
	maxLogSize := 8000
	startBytes := 0
	i := 0
	for startBytes < numBytesInContents {
		bytesLeft := len(contents[startBytes:])
		chunkSize := maxLogSize
		if bytesLeft < maxLogSize {
			chunkSize = bytesLeft
		}
		stopBytes := startBytes + chunkSize
		output := string(contents[startBytes:stopBytes])
		logrus.WithField("output", output).Infof("%s debug part %d", debugFilePath, i)
		i += 1
		startBytes += chunkSize
	}
}

func genericHookMain() {
	if err := runGenericHook(); err != nil {
		logrus.Errorf("error in generic hook: %s", err)
		debugFileToRead := os.Getenv(generichook.LogDebugFileEnvKey)
		if debugFileToRead != "" {
			logDebugFile(debugFileToRead)
		}
		os.Exit(1)
	}
	os.Exit(0)
}

// the below functions are based on containerd oci-hook.go and are used for
// injecting runtime values into an oci hook's command
func loadHookState(r io.Reader) (*specs.State, error) {
	var s *specs.State
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}

func newTemplateContext(state *specs.State) *templateContext {
	t := &templateContext{
		state: state,
	}
	t.funcs = template.FuncMap{
		"id":         t.id,
		"pid":        t.pid,
		"annotation": t.annotation,
	}
	return t
}

type templateContext struct {
	state *specs.State
	funcs template.FuncMap
}

func (t *templateContext) id() string {
	return t.state.ID
}

func (t *templateContext) pid() int {
	return t.state.Pid
}

func (t *templateContext) annotation(k string) string {
	return t.state.Annotations[k]
}

func render(templateList []string, tctx *templateContext) ([]string, error) {
	buf := bytes.NewBuffer(nil)
	for i, s := range templateList {
		buf.Reset()

		t, err := template.New("generic-hook").Funcs(tctx.funcs).Parse(s)
		if err != nil {
			return nil, err
		}
		if err := t.Execute(buf, tctx); err != nil {
			return nil, err
		}
		templateList[i] = buf.String()
	}
	buf.Reset()
	return templateList, nil
}
