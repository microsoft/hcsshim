package main

// vsockspew is a very simple listener which spews the output received
// from a vsock connection of a utility VM.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"

	"github.com/linuxkit/virtsock/pkg/hvsock"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	_ERROR_CONNECTION_ABORTED syscall.Errno = 1236
	linuxLogVsockPort                       = 109
)

func main() {

	app := cli.NewApp()
	app.Name = "vsockspew"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Usage: `Runtime ID of the utility VM`,
		},
	}

	app.Action = func(c *cli.Context) error {
		vmID, err := hvsock.GUIDFromString(c.String("id"))
		if err != nil {
			return fmt.Errorf("failed to generate GUID from runtime ID %s: %s", c.String("id"), err)
		}

		serviceID, _ := hvsock.GUIDFromString("00000000-facb-11e6-bd58-64006a7986d3")
		binary.LittleEndian.PutUint32(serviceID[0:4], linuxLogVsockPort)

		// Open a listener
		var listener net.Listener
		listener, err = hvsock.Listen(hvsock.Addr{VMID: vmID, ServiceID: serviceID})
		if err != nil {
			return err
		}

		// Accept a connection
		var conn net.Conn
		conn, err = listener.Accept()
		listener.Close()
		if err != nil {
			return err
		}

		j := json.NewDecoder(conn)
		logger := logrus.StandardLogger()
		for {
			e := logrus.Entry{Logger: logger}
			err = j.Decode(&e.Data)
			if err == io.EOF || err == _ERROR_CONNECTION_ABORTED {
				break
			}
			if err != nil {
				// Something went wrong. Read the rest of the data as a single
				// string and log it at once -- it's probably a GCS panic stack.
				logrus.Error("gcs log read: ", err)
				rest, _ := ioutil.ReadAll(io.MultiReader(j.Buffered(), conn))
				if len(rest) != 0 {
					logrus.Error("gcs stderr: ", string(rest))
				}
				break
			}
			msg := e.Data["msg"]
			delete(e.Data, "msg")
			lvl := e.Data["level"]
			delete(e.Data, "level")
			e.Data["vm.time"] = e.Data["time"]
			delete(e.Data, "time")
			switch lvl {
			case "debug":
				e.Debug(msg)
			case "info":
				e.Info(msg)
			case "warning":
				e.Warning(msg)
			case "error", "fatal":
				e.Error(msg)
			default:
				e.Info(msg)
			}
		}

		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
		os.Exit(-1)
	}
}
