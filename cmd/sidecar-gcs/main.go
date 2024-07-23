//go:build windows

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/gcs"
)

func acceptAndClose(l net.Listener) (conn net.Conn, err error) {
	conn, err = l.Accept()
	l.Close()

	return conn, err
}

func main() {
	ctx := context.Background()
	file, err := os.OpenFile("hello.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Error opening file: %v", err)
		return
	}
	defer file.Close()

	d := &winio.HvsockDialer{
		Deadline:  time.Now().Add(10 * time.Minute),
		Retries:   1000,
		RetryWait: time.Second,
	}

	//DEFINE_GUID(HV_GUID_PARENT, 0xa42e7cda, 0xd03f, 0x480c, 0x9c, 0xc2, 0xa4, 0xde, 0x20, 0xab, 0xb8, 0x78);
	addr := &winio.HvsockAddr{
		VMID: guid.GUID{
			Data1: 0xa42e7cda,
			Data2: 0xd03f,
			Data3: 0x480c,
			Data4: [8]uint8{0x9c, 0xc2, 0xa4, 0xde, 0x20, 0xab, 0xb8, 0x78},
		},
		ServiceID: gcs.SidecarGcsHvsockServiceID,
	}

	_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - started dialing\n")
	if err != nil {
		fmt.Printf("Error writing to file: %v", err)
		return
	}

	// Dial the HV socket for the sidecar gcs
	conn, err := d.Dial(ctx, addr)
	if err != nil {
		_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - Error dialing address: " + err.Error() + "\n")
		if err != nil {
			fmt.Printf("Error writing to file: %v", err)
			return
		}
		return
	}

	go func() {
		_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - Connection established\n")
		if err != nil {
			fmt.Printf("Error writing to file: %v", err)
			return
		}
		for {
			_, err := conn.Write([]byte(time.Now().Format("2006-01-02 15:04:05") + " - Hello, world!\n"))
			if err != nil {
				_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - Error writing to connection: " + err.Error() + "\n")
				if err != nil {
					fmt.Printf("Error writing to file: %v", err)
					return
				}
				return
			}

			time.Sleep(5 * time.Second)
		}
	}()

	// _, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - Started listening for the GCS\n")
	// if err != nil {
	// 	fmt.Printf("Error writing to file: %v", err)
	// 	return
	// }

	// start GCS listener
	// 0xe0e16197, 0xdd56, 0x4a10, 0x91, 0x95, 0x5e, 0xe7, 0xa1, 0x55, 0xa8, 0x38
	l, err := winio.ListenHvsock(&winio.HvsockAddr{
		VMID: guid.GUID{
			Data1: 0xe0e16197,
			Data2: 0xdd56,
			Data3: 0x4a10,
			Data4: [8]uint8{0x91, 0x95, 0x5e, 0xe7, 0xa1, 0x55, 0xa8, 0x38},
		},
		ServiceID: gcs.SidecarGuestHvsockServiceID,
	})
	if err != nil {
		_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - Error opening gcs listener: " + err.Error() + "\n")
		if err != nil {
			fmt.Printf("Error writing to file: %v", err)
			return
		}
		return
	}

	if l != nil {
		_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - GCS listener opened\n")
		if err != nil {
			fmt.Printf("Error writing to file: %v", err)
			return
		}

		// Accept the connection
		gcsConn, err := acceptAndClose(l)
		if err != nil {
			_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - Error with accept: " + err.Error() + "\n")
			if err != nil {
				fmt.Printf("Error writing to file: %v", err)
				return
			}
			return
		}

		_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - GCS connection established\n")
		if err != nil {
			fmt.Printf("Error writing to file: %v", err)
			return
		}

		l = nil

		go func() {
			logfile, err := os.OpenFile("gcsLog.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - error reading \n")
				if err != nil {
					return
				}
				return
			}
			defer logfile.Close()

			gcsbuffer := make([]byte, 1024)
			for {
				_, err := gcsConn.Read(gcsbuffer)
				if err != nil {
					_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - error reading from client\n")
					if err != nil {
						return
					}
					return
				}
				time.Sleep(5 * time.Second)

				if _, err := logfile.Write(gcsbuffer); err != nil {
					_, err = file.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - error writing to logfile\n")
					if err != nil {
						return
					}
					return
				}
			}
		}()

		for {
			fmt.Println("Running sidecar")
		}
	}
}

/*
Next steps:
1) run sidecar as a service, inbox component cannot start outbox componenet
2) Sidecar will be a container layer in the image
*/

//sidecar is client for shim, gcs is client for sidecar
