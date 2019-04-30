package svm

// import (
// 	"bytes"
// 	"encoding/hex"
// 	"fmt"
// 	"io"
// 	"strings"
// 	"syscall"
// 	"time"

// 	"github.com/Microsoft/hcsshim"
// 	"github.com/sirupsen/logrus"
// )

// // RunProcess is a simple wrapper for running a process in a service VM.
// // It returns the exit code and the combined stdout/stderr.
// func (i *instance) RunProcess(id string, args []string, stdin string) (int, string, error) {
// 	// Keep a global service VM running - effectively a no-op
// 	if i.mode == ModeGlobal {
// 		id = globalID
// 	}

// 	// Write operation. Must hold the lock. TODO Not sure we do....
// 	//i.Lock()
// 	//defer i.Unlock()

// 	// Nothing to do if no service VMs or not found
// 	if i.serviceVMs == nil {
// 		return 0, "", ErrNotFound
// 	}
// 	svmItem, exists := i.serviceVMs[id]
// 	if !exists {
// 		return 0, "", ErrNotFound
// 	}

// 	env := make(map[string]string)
// 	env["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:"
// 	commandLine := strings.Join(args, " ")
// 	processConfig := &hcsshim.ProcessConfig{
// 		EmulateConsole:    false,
// 		CreateStdInPipe:   len(stdin) > 0,
// 		CreateStdOutPipe:  true,
// 		CreateStdErrPipe:  true,
// 		CreateInUtilityVm: true,
// 		WorkingDirectory:  "/bin",
// 		Environment:       env,
// 		CommandLine:       commandLine,
// 	}
// 	p, err := svmItem.serviceVM.utilityVM.ComputeSystem().CreateProcess(processConfig)
// 	if err != nil {
// 		return 0, "", err
// 	}
// 	defer p.Close()

// 	pIn, pOut, pErr, err := p.Stdio()
// 	if err != nil {
// 		p.Kill()
// 		return 0, "", err
// 	}

// 	if len(stdin) > 0 {
// 		if _, err = copyWithTimeout(
// 			pIn,
// 			strings.NewReader(stdin),
// 			0,
// 			60, // TODO
// 			fmt.Sprintf("send to stdin of %s", commandLine)); err != nil {
// 			return 0, "", err
// 		}
// 		// Don't need stdin now we've sent everything. This signals GCS that we are finished sending data.
// 		if err := p.CloseStdin(); err != nil && !hcsshim.IsNotExist(err) && !hcsshim.IsAlreadyClosed(err) {
// 			// This error will occur if the compute system is currently shutting down
// 			if perr, ok := err.(*hcsshim.ProcessError); ok && perr.Err != hcsshim.ErrVmcomputeOperationInvalidState {
// 				return 0, "", err
// 			}
// 		}
// 	}

// 	p.WaitTimeout(60 * time.Second) // TODO
// 	exitCode, err := p.ExitCode()
// 	if err != nil {
// 		return 0, "", err
// 	}

// 	// Copy stderr back if non-zero exit code
// 	if exitCode != 0 {
// 		errbuf := &bytes.Buffer{}
// 		if _, err := copyWithTimeout(errbuf,
// 			pErr,
// 			0,
// 			60, // TODO
// 			fmt.Sprintf("RunProcess: copy back from %s", commandLine)); err != nil {
// 			return exitCode, "", err
// 		}
// 		return exitCode, errbuf.String(), nil
// 	}

// 	outbuf := &bytes.Buffer{}
// 	if _, err := copyWithTimeout(outbuf,
// 		pOut,
// 		0,
// 		60, // TODO
// 		fmt.Sprintf("copy stdout back from %s", commandLine)); err != nil {
// 		return exitCode, "", err
// 	}

// 	return exitCode, outbuf.String(), nil
// }

// // copyWithTimeout is a wrapper for io.Copy using a timeout duration
// func copyWithTimeout(dst io.Writer, src io.Reader, size int64, timeoutSeconds int, operation string) (int64, error) {
// 	logrus.Debugf("copywithtimeout: size %d: timeout %d: (%s)", size, timeoutSeconds, operation)

// 	type resultType struct {
// 		err   error
// 		bytes int64
// 	}

// 	done := make(chan resultType, 1)
// 	go func() {
// 		result := resultType{}
// 		if logrus.GetLevel() < logrus.DebugLevel || logDataFromUVM == 0 {
// 			result.bytes, result.err = io.Copy(dst, src)
// 		} else {
// 			// In advanced debug mode where we log (hexdump format) what is copied
// 			// up to the number of bytes defined by environment variable
// 			// OPENGCS_LOG_DATA_FROM_UVM
// 			var buf bytes.Buffer
// 			tee := io.TeeReader(src, &buf)
// 			result.bytes, result.err = io.Copy(dst, tee)
// 			if result.err == nil {
// 				size := result.bytes
// 				if size > logDataFromUVM {
// 					size = logDataFromUVM
// 				}
// 				if size > 0 {
// 					bytes := make([]byte, size)
// 					if _, err := buf.Read(bytes); err == nil {
// 						logrus.Debugf(fmt.Sprintf("copyWithTimeout\n%s", hex.Dump(bytes)))
// 					}
// 				}
// 			}
// 		}
// 		done <- result
// 	}()

// 	var result resultType
// 	timedout := time.After(time.Duration(timeoutSeconds) * time.Second)

// 	select {
// 	case <-timedout:
// 		return 0, fmt.Errorf("copyWithTimeout: timed out (%s)", operation)
// 	case result = <-done:
// 		if result.err != nil && result.err != io.EOF {
// 			// See https://github.com/golang/go/blob/f3f29d1dea525f48995c1693c609f5e67c046893/src/os/exec/exec_windows.go for a clue as to why we are doing this :)
// 			if se, ok := result.err.(syscall.Errno); ok {
// 				const (
// 					errNoData     = syscall.Errno(232)
// 					errBrokenPipe = syscall.Errno(109)
// 				)
// 				if se == errNoData || se == errBrokenPipe {
// 					logrus.Debugf("copyWithTimeout: hit NoData or BrokenPipe: %d: %s", se, operation)
// 					return result.bytes, nil
// 				}
// 			}
// 			return 0, fmt.Errorf("copyWithTimeout: error reading: '%s' after %d bytes (%s)", result.err, result.bytes, operation)
// 		}
// 	}
// 	logrus.Debugf("copyWithTimeout: success - copied %d bytes (%s)", result.bytes, operation)
// 	return result.bytes, nil
// }
