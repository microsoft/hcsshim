//go:build linux
// +build linux

package kmsg

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
)

var (
	// ErrInvalidFormat indicates the kmsg entry failed to parse.
	ErrInvalidFormat = errors.New("invalid kmsg format")
)

// LogLevel represents the severity/priority of a log entry in the kernels
// ring buffer.
// https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/linux/kern_levels.h?id=HEAD
type LogLevel uint8

func (logLevel LogLevel) String() string {
	return levels[logLevel]
}

const (
	Emerg LogLevel = iota
	Alert
	Crit
	Err
	Warning
	Notice
	Info
	Debug
)

var levels = [...]string{
	"Emerg",
	"Alert",
	"Crit",
	"Err",
	"Warning",
	"Notice",
	"Info",
	"Debug",
}

// Entry is a single log entry in kmsg.
type Entry struct {
	Priority           LogLevel
	Facility           uint8
	Seq                uint64
	TimeSinceBootMicro uint64
	Flags              string
	Message            string
}

func (ke *Entry) logFormat() logrus.Fields {
	return logrus.Fields{
		"priority":           ke.Priority.String(),
		"facility":           ke.Facility,
		"seq":                ke.Seq,
		"timesincebootmicro": ke.TimeSinceBootMicro,
		"flags":              ke.Flags,
		"message":            ke.Message,
	}
}

// Parse takes a single kmsg log entry string and returns a struct representing
// the components of the log entry.
func parse(s string) (*Entry, error) {
	fields := strings.SplitN(s, ";", 2)
	if len(fields) < 2 {
		return nil, ErrInvalidFormat
	}
	prefixFields := strings.SplitN(fields[0], ",", 5)
	if len(prefixFields) < 4 {
		return nil, ErrInvalidFormat
	}
	syslog, err := strconv.ParseUint(prefixFields[0], 10, 16)
	if err != nil {
		return nil, ErrInvalidFormat
	}
	seq, err := strconv.ParseUint(prefixFields[1], 10, 64)
	if err != nil {
		return nil, ErrInvalidFormat
	}
	timestamp, err := strconv.ParseUint(prefixFields[2], 10, 64)
	if err != nil {
		return nil, ErrInvalidFormat
	}

	// TODO: validate that the message ends in with a \n and parse key/value pairs:
	//  - "The human readable text string starts directly after the ';' and is terminated by a '\n'"
	//  - "A line starting with ' ', is a continuation line, adding key/value pairs to the log message"
	//
	// See: https://docs.kernel.org/admin-guide/abi-testing.html#abi-dev-kmsg
	msg := strings.TrimSpace(fields[1])

	return &Entry{
		Priority:           LogLevel(syslog & 0x7),
		Facility:           uint8(syslog >> 3),
		Seq:                seq,
		TimeSinceBootMicro: timestamp,
		Flags:              prefixFields[3],
		Message:            msg,
	}, nil
}

// ReadForever reads from /dev/kmsg forever unless /dev/kmsg cannot be opened.
// Every entry with priority <= 'logLevel' will be logged.
func ReadForever(logLevel LogLevel) {
	// TODO:
	//  - handle continuation fragments:
	//    "/dev/kmsg users are recommended to implement fragment handling."
	//    See: https://docs.kernel.org/admin-guide/abi-testing.html#abi-dev-kmsg
	//  - use `dmesg --follow --json` to avoid string parsing, buffer allocation, and fragment handling
	//  - check entry sequence numbers are sequential and warn if messages were skipped
	file, err := os.Open("/dev/kmsg")
	if err != nil {
		logrus.WithError(err).Error("failed to open /dev/kmsg")
		return
	}
	defer file.Close()
	// Buffer size is controlled by LOG_BUF_SHIFT (and LOG_CPU_MAX_BUF_SHIFT) kernel parameters.
	// Use the default shift value of 17, since records larger than 128 KiB should be unlikely and
	// allocating the maximum (LOG_BUF_LEN_MAX) 2 GiB is overkill.
	//
	// See:
	//  - https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/init/Kconfig?id=11028ab62899e4191e074ee364c712b77823a9c4#n807
	//  - https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/kernel/printk/printk.c?id=11028ab62899e4191e074ee364c712b77823a9c4#n503
	const bufSize = 1 << 17
	// Reuse buffer for entries
	buf := make([]byte, bufSize)
	for {
		n, err := file.Read(buf)
		if err != nil {
			// "In case messages get overwritten in the circular buffer while
			// the device is kept open, the next read() will return -EPIPE,
			// and the seek position be updated to the next available record.
			// Subsequent reads() will return available records again."
			if err == syscall.EPIPE { //nolint:errorlint
				logrus.Warn("kmsg entry overwritten; skipping entry")
				continue
			}
			logrus.WithError(err).Error("kmsg read failure")
			return
		}
		line := string(buf[:n])
		entry, err := parse(line)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"line":          line,
			}).Error("failed to parse kmsg entry")
		} else {
			if entry.Priority <= logLevel {
				logrus.WithFields(entry.logFormat()).Info("kmsg read")
			}
		}
	}
}
