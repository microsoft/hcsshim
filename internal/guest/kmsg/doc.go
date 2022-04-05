// Package kmsg contains support for parsing Linux kernel log entries read from
// /dev/kmsg. These are the same log entries that can be read via the `dmesg`
// command. Each read from /dev/kmsg is guaranteed to return a single log entry,
// so no line-splitting is required.
//
// More information can be found here:
// https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
package kmsg
