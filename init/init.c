#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <getopt.h>
#include <linux/random.h> // RNDADDENTROPY
#include <net/if.h>
#include <netinet/ip.h>
#include <signal.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/sysmacros.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#ifdef MODULES
#include <ftw.h>
#include <libkmod.h>
#include <sys/utsname.h>
#endif

#include "../vsockexec/vsock.h"

#ifdef DEBUG
#ifdef USE_TCP
static const int tcpmode = 1;
#else
static const int tcpmode;
#endif
// vsockexec opens vsock connections for the specified stdio descriptors and
// then execs the specified process.

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
#endif

#define DEFAULT_PATH_ENV "PATH=/sbin:/usr/sbin:/bin:/usr/bin"
#define OPEN_FDS 15

const char* const default_envp[] = {
    DEFAULT_PATH_ENV,
    NULL,
};

#ifdef MODULES
// global kmod k_ctx so we can access it in the file tree traversal
struct kmod_ctx* k_ctx;

// possible extensions for the kernel modules files
const char* kmod_ext = ".ko";
const char* kmod_xz_ext = ".ko.xz";
#endif

// When nothing is passed, default to the LCOWv1 behavior.
const char* const default_argv[] = {"/bin/gcs", "-loglevel", "debug", "-logfile=/run/gcs/gcs.log"};
const char* const default_shell = "/bin/sh";
const char* const lib_modules = "/lib/modules";

struct Mount {
    const char *source, *target, *type;
    unsigned long flags;
    const void* data;
};

struct Mkdir {
    const char* path;
    mode_t mode;
};

struct Mknod {
    const char* path;
    mode_t mode;
    int major, minor;
};

struct Symlink {
    const char *linkpath, *target;
};

enum OpType {
    OpMount,
    OpMkdir,
    OpMknod,
    OpSymlink,
};

struct InitOp {
    enum OpType op;
    union {
        struct Mount mount;
        struct Mkdir mkdir;
        struct Mknod mknod;
        struct Symlink symlink;
    };
};

const struct InitOp ops[] = {
    // mount /proc (which should already exist)
    {OpMount, .mount = {"proc", "/proc", "proc", MS_NODEV | MS_NOSUID | MS_NOEXEC}},

    // add symlinks in /dev (which is already mounted)
    {OpSymlink, .symlink = {"/dev/fd", "/proc/self/fd"}},
    {OpSymlink, .symlink = {"/dev/stdin", "/proc/self/fd/0"}},
    {OpSymlink, .symlink = {"/dev/stdout", "/proc/self/fd/1"}},
    {OpSymlink, .symlink = {"/dev/stderr", "/proc/self/fd/2"}},

    // mount tmpfs on /run and /tmp (which should already exist)
    {OpMount, .mount = {"tmpfs", "/run", "tmpfs", MS_NODEV | MS_NOSUID | MS_NOEXEC, "mode=0755"}},
    {OpMount, .mount = {"tmpfs", "/tmp", "tmpfs", MS_NODEV | MS_NOSUID | MS_NOEXEC}},

    // mount shm and devpts
    {OpMkdir, .mkdir = {"/dev/shm", 0755}},
    {OpMount, .mount = {"shm", "/dev/shm", "tmpfs", MS_NODEV | MS_NOSUID | MS_NOEXEC}},
    {OpMkdir, .mkdir = {"/dev/pts", 0755}},
    {OpMount, .mount = {"devpts", "/dev/pts", "devpts", MS_NOSUID | MS_NOEXEC}},

    // mount /sys (which should already exist)
    {OpMount, .mount = {"sysfs", "/sys", "sysfs", MS_NODEV | MS_NOSUID | MS_NOEXEC}},
    {OpMount, .mount = {"cgroup_root", "/sys/fs/cgroup", "tmpfs", MS_NODEV | MS_NOSUID | MS_NOEXEC, "mode=0755"}},
};

/*
rootfs VHDs are mounted as read-only, which can cause issues for binaries running in the
uVM (e.g., syslogd, (GPU) drivers) that expect to be able to write to /etc/
(e.g., syslogd is configured by /etc/syslog.conf) or /var/ (e.g., syslogd (typically) writes to /var/log/messages).

Make /var and /etc writable by creating an overlay with a tmpfs-backer upper (and work) directories.

Use /run for overlay directories since that shouldn't be as volatile as /tmp.
/run is already tmpfs backed, but create a new (smaller) tmpfs mount to prevent contestion
with container-specific files under /run/gcs/c/ (e.g., the container config file and overlay work directory).

Note: tmpfs is backed by virtual memory and can be swapped out, but the uVM is, itself, virtual memory
backed on the host.
Hence limiting the total size of tmpfs mounts will prevent the virtual machine's worker
thread on the host from growing egregiously.

See:
- https://refspecs.linuxfoundation.org/FHS_3.0/fhs/ch03s07.html
- https://refspecs.linuxfoundation.org/FHS_3.0/fhs/ch05.html
- https://refspecs.linuxfoundation.org/FHS_3.0/fhs/ch05s10.html
- https://refspecs.linuxfoundation.org/FHS_3.0/fhs/ch03s15.html
*/
#define OVERLAY_PATH "/run/over"
#define VAR_OVERLAY_PATH OVERLAY_PATH "/var"
#define ETC_OVERLAY_PATH OVERLAY_PATH "/etc"

const struct InitOp overlay_ops[] = {
    // /run should already exist
    {OpMkdir, .mkdir = {OVERLAY_PATH, 0755}},
    {OpMount, .mount = {"tmpfs", OVERLAY_PATH, "tmpfs", MS_NODEV | MS_NOSUID | MS_NOEXEC, "size=40\%,mode=0755"}},

    // /etc
    {OpMkdir, .mkdir = {ETC_OVERLAY_PATH, 0755}},
    {OpMkdir, .mkdir = {(ETC_OVERLAY_PATH "/upper"), 0755}},
    {OpMkdir, .mkdir = {(ETC_OVERLAY_PATH "/work"), 0755}},
    {OpMount, .mount = {"overlay", "/etc", "overlay", MS_NODEV | MS_NOSUID | MS_NOEXEC,
                        "lowerdir=/etc,upperdir=" ETC_OVERLAY_PATH "/upper,workdir=" ETC_OVERLAY_PATH "/work"}},

    // /var
    {OpMkdir, .mkdir = {VAR_OVERLAY_PATH, 0755}},
    {OpMkdir, .mkdir = {VAR_OVERLAY_PATH "/upper", 0755}},
    {OpMkdir, .mkdir = {VAR_OVERLAY_PATH "/work", 0755}},
    {OpMount, .mount = {"overlay", "/var", "overlay", MS_NODEV | MS_NOSUID, // allow execs from the /var
                        "lowerdir=/var,upperdir=" VAR_OVERLAY_PATH "/upper,workdir=" VAR_OVERLAY_PATH "/work"}},
};

void warn(const char* msg) {
    int error = errno;
    perror(msg);
    errno = error;
}

void warn2(const char* msg1, const char* msg2) {
    int error = errno;
    fputs(msg1, stderr);
    fputs(": ", stderr);
    errno = error;
    warn(msg2);
}

_Noreturn void dien() {
#ifdef DEBUG
    printf("dien errno = %d", errno);
#endif
    exit(errno);
}

_Noreturn void die(const char* msg) {
    warn(msg);
    dien();
}

_Noreturn void die2(const char* msg1, const char* msg2) {
    warn2(msg1, msg2);
    dien();
}

void init_rlimit() {
    // Set the hard limit for number of open fds much larger. The kernel sets
    // a limit of 4096 for historical reasons, and this limit is too low for
    // some software. According to the systemd developers, there is no downside
    // to a large hard limit in modern Linux kernels.
    //
    // Retain the small soft limit of 1024 for appcompat.
    struct rlimit rlim = {
        .rlim_cur = 1024,
        .rlim_max = 1024 * 1024,
    };
    if (setrlimit(RLIMIT_NOFILE, &rlim) < 0) {
        die("setrlimit(RLIMIT_NOFILE)");
    }
}

void init_dev() {
    if (mount("dev", "/dev", "devtmpfs", MS_NOSUID | MS_NOEXEC, NULL) < 0) {
#ifdef DEBUG
        printf("mount - errno %d\n", errno);
#endif
        warn2("mount", "/dev");
        // /dev will be already mounted if devtmpfs.mount = 1 on the kernel
        // command line or CONFIG_DEVTMPFS_MOUNT is set. Do not consider this
        // an error.
        if (errno != EBUSY) {
            dien();
        }
    }
}

void init_fs(const struct InitOp* ops, size_t count) {
    for (size_t i = 0; i < count; i++) {
        switch (ops[i].op) {
        case OpMount: {
            const struct Mount* m = &ops[i].mount;
#ifdef DEBUG
            printf("OpMount src %s target %s type %s flags %lu data %p\n", m->source, m->target, m->type, m->flags,
                   m->data);
#endif
            if (mount(m->source, m->target, m->type, m->flags, m->data) < 0) {
                die2("mount", m->target);
            }
            break;
        }
        case OpMkdir: {
            const struct Mkdir* m = &ops[i].mkdir;
#ifdef DEBUG
            printf("OpMkdir path %s mode %d\n", m->path, m->mode);
#endif
            if (mkdir(m->path, m->mode) < 0) {
                warn2("mkdir", m->path);
                if (errno != EEXIST) {
                    dien();
                }
            }
            break;
        }
        case OpMknod: {
            const struct Mknod* n = &ops[i].mknod;
#ifdef DEBUG
            printf("OpMknod path %s mode %d major %d minor %d\n", n->path, n->mode, n->major, n->minor);
#endif
            if (mknod(n->path, n->mode, makedev(n->major, n->minor)) < 0) {
                warn2("mknod", n->path);
                if (errno != EEXIST) {
                    dien();
                }
            }
            break;
        }
        case OpSymlink: {
            const struct Symlink* sl = &ops[i].symlink;
#ifdef DEBUG
            printf("OpSymlink targeg %s link %s\n", sl->target, sl->linkpath);
#endif
            if (symlink(sl->target, sl->linkpath) < 0) {
                warn2("symlink", sl->linkpath);
                if (errno != EEXIST) {
                    dien();
                }
            }
            break;
        }
        }
    }
}

void init_cgroups() {
    const char* fpath = "/proc/cgroups";
    FILE* f = fopen(fpath, "r");
    if (f == NULL) {
        die2("fopen", fpath);
    }
    // Skip the first line.
    for (;;) {
        char c = fgetc(f);
        if (c == EOF || c == '\n') {
            break;
        }
    }
    for (;;) {
        static const char base_path[] = "/sys/fs/cgroup/";
        char path[sizeof(base_path) - 1 + 64];
        char* name = path + sizeof(base_path) - 1;
        int hier, groups, enabled;
        int r = fscanf(f, "%64s %d %d %d\n", name, &hier, &groups, &enabled);
        if (r == EOF) {
            break;
        }
        if (r != 4) {
            errno = errno ?: EINVAL;
            die2("fscanf", fpath);
        }
        if (enabled) {
            memcpy(path, base_path, sizeof(base_path) - 1);
            if (mkdir(path, 0755) < 0) {
                die2("mkdir", path);
            }
            if (mount(name, path, "cgroup", MS_NODEV | MS_NOSUID | MS_NOEXEC, name) < 0) {
                die2("mount", path);
            }
        }
    }
    fclose(f);
}

void init_network(const char* iface, int domain) {
    int s = socket(domain, SOCK_DGRAM, IPPROTO_IP);
    if (s < 0) {
        if (errno == EAFNOSUPPORT) {
            return;
        }
        die("socket");
    }

    struct ifreq request = {0};
    strncpy(request.ifr_name, iface, sizeof(request.ifr_name));
    if (ioctl(s, SIOCGIFFLAGS, &request) < 0) {
        die2("ioctl(SIOCGIFFLAGS)", iface);
    }

    request.ifr_flags |= IFF_UP | IFF_RUNNING;
    if (ioctl(s, SIOCSIFFLAGS, &request) < 0) {
        die2("ioctl(SIOCSIFFLAGS)", iface);
    }

    close(s);
}

// inject boot-time entropy after reading it from a vsock port
void init_entropy(int port) {
    int s = openvsock(VMADDR_CID_HOST, port);
    if (s < 0) {
        die("openvsock entropy");
    }

    int e = open("/dev/random", O_RDWR);
    if (e < 0) {
        die("open /dev/random");
    }

    struct {
        int entropy_count;
        int buf_size;
        char buf[4096];
    } buf;

    for (;;) {
        ssize_t n = read(s, buf.buf, sizeof(buf.buf));
        if (n < 0) {
            die("read entropy");
        }

        if (n == 0) {
            break;
        }

        buf.entropy_count = n * 8; // in bits
        buf.buf_size = n;          // in bytes
        if (ioctl(e, RNDADDENTROPY, &buf) < 0) {
            die("ioctl(RNDADDENTROPY)");
        }
    }

    close(s);
    close(e);
}

// dmesg is a helper function for printing to dmesg. We cannot assume that the
// image has syslogd or similar running for now, so we cannot just use syslog(3).
//
// /dev/kmsg exports the structured data in the following line format:
// "<level>,<sequnum>,<timestamp>,<contflag>[,additional_values, ... ];<message text>\n"
void dmesg(const unsigned int level, const char* msg) {
    int fd_kmsg = open("/dev/kmsg", O_WRONLY);
    if (fd_kmsg == -1) {
        // failed to open the kmsg device
        warn("error opening /dev/kmsg");
        return;
    }
    FILE* f_kmsg = fdopen(fd_kmsg, "w");
    if (f_kmsg == NULL) {
        warn("error getting /dev/kmsg file");
        close(fd_kmsg);
        return;
    }
    fprintf(f_kmsg, "<%u>%s", level, msg);
    fflush(f_kmsg);
    int close_ret = fclose(f_kmsg); // closes the underlying fd_kmsg as well
    if (close_ret != 0) {
        warn("error closing /dev/kmsg");
    }
}

// see https://man7.org/linux/man-pages/man2/syslog.2.html for definitions of levels
void dmesgErr(const char* msg) { dmesg(3, msg); }
void dmesgWarn(const char* msg) { dmesg(4, msg); }
void dmesgInfo(const char* msg) { dmesg(6, msg); }

pid_t launch(int argc, char** argv) {
    int pid = fork();
    if (pid != 0) {
        if (pid < 0) {
            die("fork");
        }

        return pid;
    }

    // Unblock signals before execing.
    sigset_t set;
    sigfillset(&set);
    sigprocmask(SIG_UNBLOCK, &set, 0);

    // Create a session and process group.
    setsid();
    setpgid(0, 0);

    // Terminate the arguments and exec.
    char** argvn = alloca(sizeof(argv[0]) * (argc + 1));
    memcpy(argvn, argv, sizeof(argv[0]) * argc);
    argvn[argc] = NULL;
    if (putenv(DEFAULT_PATH_ENV)) { // Specify the PATH used for execvpe
        die("putenv");
    }
    // CodeQL [SM01925] designed to initialize Linux guest and then exec the command-line arguments (either ./vsockexec or ./cmd/gcs)
    execvpe(argvn[0], argvn, (char**)default_envp);
    die2("execvpe", argvn[0]);
}

int reap_until(pid_t until_pid) {
    for (;;) {
        int status;
        pid_t pid = wait(&status);
        if (pid < 0) {
            die("wait");
        }

        if (pid == until_pid) {
            // The initial child process died. Pass through the exit status.
            if (WIFEXITED(status)) {
                if (WEXITSTATUS(status) != 0) {
                    fputs("child exited with error\n", stderr);
                }
                return WEXITSTATUS(status);
            }
            fputs("child exited by signal: ", stderr);
            fputs(strsignal(WTERMSIG(status)), stderr);
            fputs("\n", stderr);
            return 128 + WTERMSIG(status);
        }
    }
}

#ifdef MODULES
// load_module gets the module from the absolute path to the module and then
// inserts into the kernel.
int load_module(struct kmod_ctx* ctx, const char* module_path) {
    struct kmod_module* mod = NULL;
    int err;

#ifdef DEBUG
    printf("loading module: %s\n", module_path);
#endif

    err = kmod_module_new_from_path(ctx, module_path, &mod);
    if (err < 0) {
        return err;
    }

    err = kmod_module_probe_insert_module(mod, 0, NULL, NULL, NULL, NULL);
    if (err < 0) {
        kmod_module_unref(mod);
        return err;
    }

    kmod_module_unref(mod);
    return 0;
}

// has_extension is a helper function for checking if string `fpath` has extension `ext`
bool has_extension(const char* fpath, const char* ext) {
    size_t fpath_length = strlen(fpath);
    size_t ext_length = strlen(ext);

    if (fpath_length < ext_length) {
        return false;
    }

    return (strncmp(fpath + (fpath_length - ext_length), ext, ext_length) == 0);
}

// parse_tree_entry is called by ftw for each directory and file in the file tree.
// If this entry is a file and has a .ko file extension, attempt to load into kernel.
int parse_tree_entry(const char* fpath, const struct stat* sb, int typeflag) {
    int result;

    if (typeflag != FTW_F) {
        // do nothing if this isn't a file
        return 0;
    }

    // Kernel module files either end with a .ko extension or a .ko.xz extension.
    // Files ending in .ko.xz are compressed kernel modules while .ko files are
    // uncompressed kernel modules.
    if (!has_extension(fpath, kmod_ext) && !has_extension(fpath, kmod_xz_ext)) {
        return 0;
    }

    // print warning if we fail to load the module, but don't fail fn so
    // we keep trying to load the rest of the modules.
    result = load_module(k_ctx, fpath);
    if (result != 0) {
        warn2("failed to load module", fpath);
    }
    dmesgInfo(fpath);
    return 0;
}

// load_all_modules finds the modules in the image and loads them using kmod,
// which accounts for ordering requirements.
void load_all_modules() {
    int max_path = 256;
    char modules_dir[max_path];
    struct utsname uname_data;
    int ret;

    // get information on the running kernel
    ret = uname(&uname_data);
    if (ret != 0) {
        die("failed to get kernel information");
    }

    // create the absolute path of the modules directory this looks
    // like /lib/modules/<uname.release>
    ret = snprintf(modules_dir, max_path, "%s/%s", lib_modules, uname_data.release);
    if (ret < 0) {
        die("failed to create the modules directory path");
    } else if (ret > max_path) {
        die("modules directory buffer larger than expected");
    }

    if (k_ctx == NULL) {
        k_ctx = kmod_new(NULL, NULL);
        if (k_ctx == NULL) {
            die("failed to create kmod context");
        }
    }

    kmod_load_resources(k_ctx);
    ret = ftw(modules_dir, parse_tree_entry, OPEN_FDS);
    if (ret != 0) {
        // Don't fail on error from walking the file tree and loading modules right now.
        // ftw may return an error if the modules directory doesn't exist, which
        // may be the case for some images. Additionally, we don't currently support
        // using a denylist when loading modules, so we may try to load modules
        // that cannot be loaded until later, such as nvidia modules which fail to
        // load if no device is present.
        warn("error adding modules");
    }

    kmod_unref(k_ctx);
}
#endif

#ifdef DEBUG
int debug_main(int argc, char** argv) {
    unsigned int ports[3] = {2056, 2056, 2056};
    int sockets[3] = {-1, -1, -1};

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

    return 0;
}
#endif

// start_services is a helper function to start different services that are
// expected to be running in the guest on boot. These processes run as
// linux daemons.
//
// Future work: Support collecting logs for these services and handle
// log rotation as needed.
void start_services() {
    // While execvpe will already search the path for the executable, it does
    // so after forking. We can avoid that unnecessary fork by stating the
    // binary beforehand.
    char* persistenced_name = "/bin/nvidia-persistenced";
    struct stat persistenced_stat;
    if (stat(persistenced_name, &persistenced_stat) == -1) {
        dmesgWarn("nvidia-persistenced not present, skipping ");
    } else {
        dmesgInfo("start nvidia-persistenced daemon");
        pid_t persistenced_pid = launch(1, &persistenced_name);
        if (persistenced_pid < 0) {
            // do not return early if we fail to start this, since it's possible that
            // this service doesn't exist on the system, which is a valid scenario
            dmesgWarn("failed to start nvidia-persistenced daemon");
        }
    }

    char* fm_name = "/bin/nv-fabricmanager";
    struct stat fabric_stat;
    if (stat(fm_name, &fabric_stat) == -1) {
        dmesgWarn("nv-fabricmanager not present, skipping ");
    } else {
        dmesgInfo("start nv-fabricmanager daemon");
        char* command[] = {fm_name, "-c", "/usr/share/nvidia/nvswitch/fabricmanager.cfg"};
        pid_t fm_pid = launch(3, command);
        if (fm_pid < 0) {
            // do not return early if we fail to start this, since it's possible that
            // this service doesn't exist on the system, which is a valid scenario
            dmesgWarn("failed to start nv-fabricmanager daemon");
        }
    }
}

int main(int argc, char** argv) {
#ifdef DEBUG
    if (debug_main(argc, argv) != 0) {
        dmesgWarn("failed to connect debug sockets");
    }
    printf("Running init\n");
#endif
    char* debug_shell = NULL;
    int entropy_port = 0;
    bool overlay_mount = false;
    if (argc <= 1) {
        argv = (char**)default_argv;
        argc = sizeof(default_argv) / sizeof(default_argv[0]);
        optind = 0;
        debug_shell = (char*)default_shell;
    } else {
        for (int opt; (opt = getopt(argc, argv, "+d:e:w")) >= 0;) {
            switch (opt) {
            case 'd': // [d]ebug
                debug_shell = optarg;
                break;

            case 'e': // [e]ntropy port
                entropy_port = atoi(optarg);
#ifdef DEBUG
                printf("entropy port %d\n", entropy_port);
#endif
                if (entropy_port == 0) {
                    fputs("invalid entropy port\n", stderr);
                    exit(1);
                }

                break;

            case 'w': // [w]ritable overlay mounts
                overlay_mount = true;
                break;

            default:
                exit(1);
            }
        }
    }

    char** child_argv = argv + optind;
    int child_argc = argc - optind;

    // Block all signals in init. SIGCHLD will still cause wait() to return.
    sigset_t set;
#ifdef DEBUG
    printf("sigfillset(&set)\n");
#endif
    sigfillset(&set);

#ifdef DEBUG
    printf("sigfillset\n");
#endif
    sigprocmask(SIG_BLOCK, &set, 0);

#ifdef DEBUG
    printf("init_rlimit\n");
#endif
    init_rlimit();

#ifdef DEBUG
    printf("init_dev\n");
#endif
    init_dev();

#ifdef DEBUG
    printf("init_fs\n");
#endif
    init_fs(ops, sizeof(ops) / sizeof(ops[0]));

    if (overlay_mount) {
#ifdef DEBUG
        printf("init_fs for overlay mounts\n");
#endif
        init_fs(overlay_ops, sizeof(overlay_ops) / sizeof(overlay_ops[0]));
    }

#ifdef DEBUG
    printf("init_cgroups\n");
#endif
    init_cgroups();

#ifdef DEBUG
    printf("init_network\n");
#endif
    init_network("lo", AF_INET);
    init_network("lo", AF_INET6);
    if (entropy_port != 0) {
        init_entropy(entropy_port);
    }

#ifdef MODULES
#ifdef DEBUG
    printf("loading modules\n");
#endif
    load_all_modules();
#endif

    start_services();

    pid_t pid = launch(child_argc, child_argv);
    if (debug_shell != NULL) {
        // The debug shell takes over as the primary child.
        pid = launch(1, &debug_shell);
    }

    if (pid < 0) {
        die("failed launching process");
    }

    // Reap until the initial child process dies.
    return reap_until(pid);
}
