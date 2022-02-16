package log

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const TimeFormat = time.RFC3339Nano

func FormatTime(t time.Time) string {
	return t.Format(TimeFormat)
}

// FormatIO formats net.Conn and other types that have an `Addr()` or `Name()`.
//
// See FormatEnabled for more information.
func FormatIO(ctx context.Context, l logrus.Level, v interface{}) string {
	if !IsLevelEnabled(ctx, l) {
		return ""
	}

	m := make(map[string]string)
	m["type"] = reflect.TypeOf(v).String()

	switch t := v.(type) {
	case net.Conn:
		m["local_address"] = formatAddr(t.LocalAddr())
		m["remote_address"] = formatAddr(t.RemoteAddr())
	case interface{ Addr() net.Addr }:
		m["address"] = formatAddr(t.Addr())
	default:
		return Format(ctx, t)
	}

	return Format(ctx, m)
}

func formatAddr(a net.Addr) string {
	return a.Network() + "://" + a.String()
}

// FormatAny formats an object into a JSON string, but only if the logger is
// enabled for the level specified. This avoids evaluating an expensive JSON
// conversion unnecessarily.
//
// See Format() for more details.
func FormatEnabled(ctx context.Context, l logrus.Level, v interface{}) string {
	if !IsLevelEnabled(ctx, l) {
		return ""
	}

	return Format(ctx, v)
}

// Format formats an object into a JSON string, without any indendtation or
// HTML escapes.
//
// Use the FormatEnabled to check before conversion that the logging level is enabled
func Format(ctx context.Context, v interface{}) string {
	buff := &bytes.Buffer{}

	if err := encode(buff, v); err != nil {
		G(ctx).WithError(err).Warning("could not JSON encode %T for logging", v)

		return ""
	}

	return strings.TrimSpace(buff.String())
}

// used by scrubber
func encode(buf *bytes.Buffer, v interface{}) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "")

	if err := enc.Encode(v); err != nil {
		return err
	}

	return nil
}
