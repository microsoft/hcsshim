package signals

import (
	"errors"
	"strconv"
	"strings"
)

var (
	// ErrInvalidSignal is the standard error for an invalid signal for a given
	// flavor of container WCOW/LCOW.
	ErrInvalidSignal = errors.New("invalid signal value")
)

// ShouldKill returns `true` if `signal` should terminate the process.
func ShouldKill(signal uint32) bool {
	return signal == sigKill || signal == sigTerm
}

// ValidateSigstr validates that `sigstr` is an acceptable signal for WCOW/LCOW
// based on `signalsSupported`.
//
// `sigstr` may either be the text name or integer value of the signal.
//
// If `signalsSupported==false` we verify that only SIGTERM/SIGKILL and CTRLC
// are sent. All other signals are not supported on downlevel platforms.
//
// By default WCOW orchestrators may still use Linux SIGTERM and SIGKILL
// semantics which will be properly translated to CTRLC, CTRLSHUTDOWN.
func ValidateSigstr(sigstr string, signalsSupported, isLcow bool) (int, error) {
	// All flavors including legacy default to SIGTERM on LCOW CtrlC on Windows
	if sigstr == "" {
		if isLcow {
			return sigTerm, nil
		}
		return ctrlC, nil
	}

	signal, err := strconv.Atoi(sigstr)
	if err == nil {
		return Validate(signal, signalsSupported, isLcow)
	}

	sigstr = strings.ToUpper(sigstr)
	if !signalsSupported {
		// If signals arent supported we just validate that its a known signal.
		// We already return 0 since we only supported a platform Kill() at that
		// time.
		if isLcow {
			switch sigstr {
			case "TERM", "KILL":
				return 0, nil
			default:
				return 0, ErrInvalidSignal
			}
		}
		switch sigstr {
		// Docker sends a UNIX term in the supported Windows Signal map.
		case "TERM", "CTRLC", "KILL":
			return 0, nil
		default:
			return 0, ErrInvalidSignal
		}
	} else {
		if !isLcow {
			// Docker sends the UNIX signal name or value. Convert them to the
			// correct Windows signals.
			switch sigstr {
			case "TERM":
				return ctrlC, nil
			case "KILL":
				return ctrlShutdown, nil
			}
		}
	}

	var sigmap map[string]int
	if isLcow {
		sigmap = signalMapLcow
	} else {
		sigmap = signalMapWindows
	}

	// Match signal string name
	for k, v := range sigmap {
		if sigstr == k {
			return v, nil
		}
	}
	return 0, ErrInvalidSignal
}

// Validate validates that `signal` is an acceptable signal for WCOW/LCOW based
// on `signalsSupported`.
//
// If `signalsSupported==false` we verify that only SIGTERM/SIGKILL and CTRLC
// are sent. All other signals are not supported on downlevel platforms.
//
// By default WCOW orechestrators may still use Linux SIGTERM and SIGKILL
// semantics which will be properly translated to CTRLC, CTRLSHUTDOWN.
func Validate(signal int, signalsSupported, isLcow bool) (int, error) {
	if !signalsSupported {
		// If signals arent supported we just validate that its a known signal.
		// We already return 0 since we only supported a platform Kill() at that
		// time.
		if isLcow {
			switch signal {
			case sigTerm, sigKill:
				return 0, nil
			default:
				return 0, ErrInvalidSignal
			}
		}
		switch signal {
		// Docker sends a UNIX term in the supported Windows Signal map.
		case sigTerm, sigKill, 0:
			return 0, nil
		default:
			return 0, ErrInvalidSignal
		}
	} else {
		if !isLcow {
			// Docker sends the UNIX signal name or value. Convert them to the
			// correct Windows signals.
			switch signal {
			case sigTerm:
				return ctrlC, nil
			case sigKill:
				return ctrlShutdown, nil
			}
		}
	}

	var sigmap map[string]int
	if isLcow {
		sigmap = signalMapLcow
	} else {
		sigmap = signalMapWindows
	}

	// Match signal by value
	for _, v := range sigmap {
		if signal == v {
			return signal, nil
		}
	}
	return 0, ErrInvalidSignal
}
