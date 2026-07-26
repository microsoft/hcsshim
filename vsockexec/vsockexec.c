// vsockexec opens vsock connections for the specified stdio descriptors and
// then execs the specified process.
//
// With -r ("reconnect mode") it does NOT hand the connection to the process.
// Instead it keeps running as a middleman (a "relay") between the process and
// the host, so that a host connection which is missing at startup or which
// drops later never reaches the process. See run_relay below for the details.

#include "vsock.h"
#include <errno.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

#ifdef USE_TCP
static const int tcpmode = 1;
#else
static const int tcpmode;
#endif

static int opentcp(unsigned short port) {
    int s = socket(AF_INET, SOCK_STREAM, 0);
    if (s < 0) {
        return -1;
    }

    struct sockaddr_in addr = {0};
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    if (connect(s, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        return -1;
    }

    return s;
}

// sleep_ms pauses for the given number of milliseconds. We use it to wait a
// short moment between connection attempts instead of retrying back-to-back and
// burning CPU.
static void sleep_ms(long ms) {
    struct timespec ts = {ms / 1000, (ms % 1000) * 1000000L};
    nanosleep(&ts, NULL);
}

// dial_retry opens a connection to the host on the given port and never gives
// up. If the host side isn't listening yet (it may come up a little later) or
// has gone away (for example the VM was moved to a different host), it just
// keeps trying until it succeeds. This is what lets the guest wait for the host
// instead of failing, which is important because failing here would take the
// whole VM down.
static int dial_retry(unsigned int port) {
    for (;;) {
        // Try to connect once. Normal builds use vsock to reach the host; the
        // tcp branch only exists for local testing.
        int s = tcpmode ? opentcp(port) : openvsock(VMADDR_CID_HOST, port);
        if (s >= 0) {
            return s; // success: we now have a live connection to the host
        }
        // Couldn't connect yet. Wait a moment and then try again.
        sleep_ms(100);
    }
}

// run_relay implements "reconnect mode" (-r).
//
// Normally vsockexec connects a socket to the host and hands that socket
// straight to the program as its stdout/stderr. The problem: if that host
// connection breaks, the program's writes fail and the program dies. Since the
// program is a child of PID 1 inside the VM, a dying program makes PID 1 exit,
// which makes the Linux kernel panic and the VM crash.
//
// run_relay avoids that by sitting in the MIDDLE and never letting the program
// touch the host connection directly:
//
//     program  --writes-->  [ pipe ]  --run_relay reads-->  [ host connection ]
//
// The program only ever writes into a pipe, which never breaks. run_relay reads
// from that pipe and forwards the data to the host. If the host connection is
// missing or drops, only run_relay notices; it quietly reconnects and keeps
// going, so the program never sees a problem and stays alive.
//
// This mode is intentionally scoped to that one job: forward a single GCS log
// stream to a single host listener. It ignores stdin (-i) and merges stdout and
// stderr onto one output port instead of acting as a general stdio forwarder --
// a reconnecting input stream or a second output port would each need their own
// buffering and reconnect logic that the logging use case never needs. The
// default (non -r) path still forwards -i/-o/-e independently.
static int run_relay(unsigned int ports[3], char** child_argv) {
    // In reconnect mode we forward the program's OUTPUT (stdout and/or stderr)
    // to one host port. Use whichever output port was requested.
    unsigned int port = ports[2] != 0 ? ports[2] : ports[1];
    if (port == 0) {
        // Nothing to forward: reconnect mode is only meaningful with -o or -e.
        fprintf(stderr, "vsockexec: -r requires -o or -e\n");
        return 1;
    }

    // Writing to a connection whose other end has gone away normally kills this
    // process with a SIGPIPE signal. Tell the system to ignore that signal, so
    // a dropped connection instead shows up as an ordinary write error that we
    // can catch below and recover from by reconnecting.
    signal(SIGPIPE, SIG_IGN);

    // Create a pipe: a simple one-way channel. Anything written to p[1] (the
    // write end) can be read back from p[0] (the read end). The program will
    // write into p[1]; we (the relay) will read from p[0].
    int p[2];
    if (pipe(p) < 0) {
        perror("pipe");
        return 1;
    }

    // Split into two processes. The child will turn into the program; the
    // parent stays behind and becomes the relay/middleman.
    pid_t pid = fork();
    if (pid < 0) {
        perror("fork");
        return 1;
    }
    if (pid == 0) {
        // ---------------- Child: becomes the program (e.g. GCS) -------------
        signal(SIGPIPE, SIG_DFL); // restore normal signal behavior for the program
        close(p[0]);              // the program only writes, so close the read end
        // Redirect the program's stdout/stderr into the pipe instead of a real
        // socket. After this, everything the program prints goes into the pipe.
        if (ports[1] != 0) {
            dup2(p[1], 1); // stdout -> pipe
        }
        if (ports[2] != 0) {
            dup2(p[1], 2); // stderr -> pipe
        }
        close(p[1]); // the fds above are now our copies; close the original
        // Replace this child process with the requested program and run it.
        // CodeQL [SM01925] designed to forward stdio over VSOCK and then exec the command-line arguments (always ./cmd/gcs)
        execvp(child_argv[0], child_argv);
        // We only reach here if the program failed to start.
        fprintf(stderr, "execvp: %s: %s\n", child_argv[0], strerror(errno));
        _exit(127);
    }

    // -------------------- Parent: the relay/middleman ----------------------
    close(p[1]); // the relay only reads, so close the write end

    // Open the connection to the host, waiting for the host if it isn't ready.
    int sock = dial_retry(port);

    // Main loop: keep copying whatever the program writes into the pipe out to
    // the host connection, for as long as the program is running.
    char buf[65536];
    for (;;) {
        // Read the next chunk of the program's output from the pipe.
        ssize_t n = read(p[0], buf, sizeof(buf));
        if (n < 0) {
            if (errno == EINTR) {
                continue; // a signal interrupted the read; simply try again
            }
            break; // unexpected error reading the pipe; stop
        }
        if (n == 0) {
            break; // the pipe reached end-of-file: the program has exited
        }
        // Send those n bytes to the host. Keep sending until every byte is out,
        // reconnecting if the connection breaks in the middle of sending.
        for (ssize_t off = 0; off < n;) {
            ssize_t w = write(sock, buf + off, (size_t)(n - off));
            if (w < 0) {
                if (errno == EINTR) {
                    continue; // interrupted by a signal; retry the write
                }
                // The connection dropped (for example the host went away). Drop
                // the dead socket, get a fresh connection, and resend the bytes
                // that had not been sent yet (off marks how far we got).
                close(sock);
                sock = dial_retry(port);
                continue;
            }
            off += w; // w bytes were sent; move past them and continue
        }
    }
    close(sock);

    // The program has finished. Wait for it and then exit with the SAME status
    // it did, so that whoever launched us (PID 1) sees the program's real exit
    // code and behaves exactly as if it had run the program directly.
    int status = 0;
    while (waitpid(pid, &status, 0) < 0 && errno == EINTR) {
        // a signal interrupted the wait; keep waiting
    }
    if (WIFEXITED(status)) {
        return WEXITSTATUS(status); // program exited normally: pass on its code
    }
    if (WIFSIGNALED(status)) {
        return 128 + WTERMSIG(status); // program was killed by a signal
    }
    return 1;
}

_Noreturn static void usage(const char* argv0) {
    fprintf(stderr, "%s [-r] [-i port] [-o port] [-e port] -- program [args...]\n", argv0);
    exit(1);
}

int main(int argc, char** argv) {
    unsigned int ports[3] = {0};
    int sockets[3] = {-1, -1, -1};
    int reconnect = 0;
    int c;
    while ((c = getopt(argc, argv, "+i:o:e:r")) != -1) {
        switch (c) {
        case 'i':
            ports[0] = strtoul(optarg, NULL, 10);
            break;

        case 'o':
            ports[1] = strtoul(optarg, NULL, 10);
            break;

        case 'e':
            ports[2] = strtoul(optarg, NULL, 10);
            break;

        case 'r':
            // -r turns on reconnect mode (resilient forwarding via run_relay).
            reconnect = 1;
            break;

        default:
            usage(argv[0]);
        }
    }

    if (optind == argc) {
        fprintf(stderr, "%s: missing program argument\n", argv[0]);
        usage(argv[0]);
    }

    if (reconnect) {
        // Resilient path: wait for the host if it isn't listening yet and
        // reconnect if the connection later drops. The plain connect-and-exec
        // path below (used when -r is absent) does neither.
        return run_relay(ports, argv + optind);
    }

    for (int i = 0; i < 3; i++) {
        if (ports[i] != 0) {
            int j;
            for (j = 0; j < i; j++) {
                if (ports[i] == ports[j]) {
                    int s = dup(sockets[j]);
                    if (s < 0) {
                        perror("dup");
                        return 1;
                    }
                    sockets[i] = s;
                    break;
                }
            }

            if (j == i) {
                int s = tcpmode ? opentcp(ports[i]) : openvsock(VMADDR_CID_HOST, ports[i]);
                if (s < 0) {
                    fprintf(stderr, "connect: port %u: %s", ports[i], strerror(errno));
                    return 1;
                }
                sockets[i] = s;
            }
        }
    }

    for (int i = 0; i < 3; i++) {
        if (sockets[i] >= 0) {
            dup2(sockets[i], i);
            close(sockets[i]);
        }
    }

    // CodeQL [SM01925] designed to forward stdio over VSOCK and then exec the command-line arguments (always ./cmd/gcs)
    execvp(argv[optind], argv + optind);
    fprintf(stderr, "execvp: %s: %s\n", argv[optind], strerror(errno));
    return 1;
}
