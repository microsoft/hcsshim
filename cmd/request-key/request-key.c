// runinns.c
// Build:  gcc -O2 -Wall -Wextra -o /run/runinns runinns.c
// Usage:  runinns [args...]
// Behavior: finds PID by env "azure_file_share_with_mi_mouter=true",
//           joins mnt/uts/ipc/net, then execs: /sbin/request-key -v [args...]
// Logs: every line goes to stderr and /run/request-key.log (best-effort)

#define _GNU_SOURCE
#include <sched.h>
#include <fcntl.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <time.h>
#include <sys/wait.h>
#include <stdarg.h>
#include <dirent.h>
#include <ctype.h>
#include <limits.h>
#include <poll.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <syslog.h>

static const char *TARGET_ENV_NAME  = "azure_file_share_krb_upcall_handler";
static const char *TARGET_ENV_VALUE = "true";
static const char *REQUEST_KEY_BIN  = "/sbin/request-key";

/* ---------- simple file logger (best-effort) ---------- */
static void ts(char *buf, size_t n) {
    struct timespec tp; clock_gettime(CLOCK_REALTIME, &tp);
    struct tm tm; gmtime_r(&tp.tv_sec, &tm);
    strftime(buf, n, "%Y-%m-%dT%H:%M:%SZ", &tm);
}

static void vlog(const char *level, const char *fmt, ...) {
    char msg[1600];
    va_list ap; va_start(ap, fmt);
    vsnprintf(msg, sizeof(msg), fmt, ap);
    va_end(ap);

    char t[32]; ts(t, sizeof t);
    char full[1800];
    snprintf(full, sizeof(full), "%s uvm-request-key[%d] %s: %s",
             t, getpid(), level, msg);

    // syslog only
    syslog(LOG_INFO, "%s", full);
}

/* ----------------------------------------------------- */

static int open_ns_fd(const char *pidstr, const char *nsname) {
    char path[PATH_MAX];
    int n = snprintf(path, sizeof(path), "/proc/%s/ns/%s", pidstr, nsname);
    if (n < 0 || n >= (int)sizeof(path)) {
        errno = ENAMETOOLONG;
        vlog("error", "ns path too long for pid=%s ns=%s", pidstr, nsname);
        return -1;
    }
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) {
        int e = errno;
        vlog("error", "open %s failed: %s", path, strerror(e));
        errno = e;
        return -1;
    }
    vlog("info", "open OK -> %s (fd=%d)", path, fd);
    return fd;
}

static int setns_fd(int fd, const char *label) {
    if (setns(fd, 0) == 0) {
        vlog("info", "setns OK -> %s", label);
        return 0;
    }
    int e = errno;
    vlog("error", "setns FAIL -> %s : %s", label, strerror(e));
    errno = e;
    return -1;
}

/* ---------- find PID by env NAME=VALUE exact match ---------- */
static int is_all_digits(const char *s) {
    if (!s || !*s) return 0;
    for (const unsigned char *p=(const unsigned char*)s; *p; ++p)
        if (!isdigit(*p)) return 0;
    return 1;
}

static int read_file_all(const char *path, char **buf, size_t *len) {
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) return errno ? errno : 1;
    size_t cap = 4096;
    char *tmp = malloc(cap);
    if (!tmp) { close(fd); return ENOMEM; }
    size_t n = 0;
    for (;;) {
        ssize_t r = read(fd, tmp + n, cap - n);
        if (r < 0) { int e = errno; free(tmp); close(fd); return e; }
        if (r == 0) break;
        n += (size_t)r;
        if (n == cap) {
            size_t nc = cap * 2;
            char *nb = realloc(tmp, nc);
            if (!nb) { free(tmp); close(fd); return ENOMEM; }
            tmp = nb; cap = nc;
        }
    }
    close(fd);
    *buf = tmp; *len = n;
    return 0;
}

static int is_zombie_process(const char *pidstr) {
    char path[PATH_MAX];
    snprintf(path, sizeof(path), "/proc/%s/stat", pidstr);
    FILE *f = fopen(path, "r");
    if (!f) return 0; // treat as not zombie if can't open
    char buf[256];
    if (!fgets(buf, sizeof(buf), f)) { fclose(f); return 0; }
    fclose(f);
    // stat format: pid (comm) state ...
    // Find the closing ')' after comm
    char *p = strchr(buf, ')');
    if (!p || !p[1]) return 0;
    p++; // move past ')'
    while (*p == ' ') p++; // skip spaces
    if (*p == 'Z') return 1; // 'Z' means zombie
    return 0;
}

static int find_pid_by_env_exact(const char *envname, const char *envval, long *out_pid) {
    DIR *d = opendir("/proc");
    if (!d) return errno ? errno : 1;

    long best = 0;
    struct dirent *de;
    size_t name_len = strlen(envname);
    size_t val_len  = strlen(envval);

    while ((de = readdir(d)) != NULL) {
        if (!is_all_digits(de->d_name)) continue;

        // Skip zombie processes
        if (is_zombie_process(de->d_name)) continue;

        char path[PATH_MAX];
        int n = snprintf(path, sizeof(path), "/proc/%s/environ", de->d_name);
        if (n < 0 || n >= (int)sizeof(path)) continue;

        char *buf = NULL; size_t len = 0;
        int er = read_file_all(path, &buf, &len);
        if (er != 0) { continue; }

        int found = 0;
        /* /proc/<pid>/environ is NUL-separated "KEY=VALUE" entries */
        size_t i = 0;
        while (i < len) {
            size_t j = i;
            while (j < len && buf[j] != '\0') j++;
            if (j > i) {
                const char *entry = buf + i;
                const char *eq = memchr(entry, '=', (size_t)(j - i));
                if (eq) {
                    size_t klen = (size_t)(eq - entry);
                    size_t vlen = (size_t)(j - i) - klen - 1; /* exclude '=' */
                    if (klen == name_len && vlen == val_len &&
                        memcmp(entry, envname, name_len) == 0 &&
                        memcmp(eq + 1, envval, val_len) == 0) {
                        found = 1; /* exact NAME=VALUE match */
                        break;
                    }
                }
            }
            i = j + 1;
        }
        free(buf);

        if (found) {
            long pid = strtol(de->d_name, NULL, 10);
            if (pid > best) best = pid; /* prefer highest (likely newest) */
        }
    }
    closedir(d);
    if (best > 0) { *out_pid = best; return 0; }
    return ESRCH;
}

int main(int argc, char **argv) {
    openlog("uvm-request-key", LOG_PID | LOG_CONS, LOG_USER);
    atexit(closelog);

    vlog("info", "UVM request-key");

    long pidval = 0;
    int er = find_pid_by_env_exact(TARGET_ENV_NAME, TARGET_ENV_VALUE, &pidval);
    if (er != 0) {
        vlog("error", "no process found with env %s=%s (err=%d: %s)",
             TARGET_ENV_NAME, TARGET_ENV_VALUE, er, strerror(er));
        return er ? er : 1;
    }

    char pidstr[32];
    snprintf(pidstr, sizeof(pidstr), "%ld", pidval);
    vlog("info", "targetPID=%s exec=%s -v (forwarding %d args)",
         pidstr, REQUEST_KEY_BIN, (argc > 1) ? (argc - 1) : 0);

    // PRE-OPEN needed ns FDs
    int fd_mnt = open_ns_fd(pidstr, "mnt"); if (fd_mnt < 0) return errno ? errno : 1;
    int fd_uts = open_ns_fd(pidstr, "uts"); if (fd_uts < 0) { close(fd_mnt); return errno ? errno : 1; }
    int fd_ipc = open_ns_fd(pidstr, "ipc"); if (fd_ipc < 0) { close(fd_mnt); close(fd_uts); return errno ? errno : 1; }
    int fd_net = open_ns_fd(pidstr, "net"); if (fd_net < 0) { close(fd_mnt); close(fd_uts); close(fd_ipc); return errno ? errno : 1; }

    // JOIN (log both successes and failures)
    if (setns_fd(fd_mnt, "mnt")) { close(fd_mnt); close(fd_uts); close(fd_ipc); close(fd_net); return errno ? errno : 1; }
    if (setns_fd(fd_uts, "uts")) { close(fd_mnt); close(fd_uts); close(fd_ipc); close(fd_net); return errno ? errno : 1; }
    if (setns_fd(fd_ipc, "ipc")) { close(fd_mnt); close(fd_uts); close(fd_ipc); close(fd_net); return errno ? errno : 1; }
    if (setns_fd(fd_net, "net")) { close(fd_mnt); close(fd_uts); close(fd_ipc); close(fd_net); return errno ? errno : 1; }

    vlog("info", "namespaces joined: mnt,uts,ipc,net for pid=%s", pidstr);

    close(fd_mnt); close(fd_uts); close(fd_ipc); close(fd_net);

    // Build argv: /sbin/request-key -v [args...]
    size_t extra = (argc > 1) ? (size_t)(argc - 1) : 0;
    size_t total = 2 + extra + 1;
    char **cmdv = (char **)calloc(total, sizeof(char *));
    if (!cmdv) { vlog("error", "calloc failed building argv"); return ENOMEM; }
    cmdv[0] = (char *)REQUEST_KEY_BIN;
    cmdv[1] = (char *)"-v";
    for (size_t i = 0; i < extra; ++i) cmdv[2 + i] = argv[1 + (int)i];
    cmdv[2 + extra] = NULL;

    pid_t child = fork();
    if (child < 0) {
        int e = errno; vlog("error", "fork failed: %s", strerror(e));
        free(cmdv);
        return e ? e : 1;
    }
    if (child == 0) {
        execv(cmdv[0], cmdv);
        int e = errno;
        vlog("error", "execv failed: %s", strerror(e));
        _exit(e ? e : 127);
    }

    free(cmdv);

    int status = 0;
    if (waitpid(child, &status, 0) < 0) {
        int e = errno; vlog("error", "waitpid failed: %s", strerror(e));
        return e ? e : 1;
    }

    if (WIFEXITED(status)) {
        int rc = WEXITSTATUS(status);
        vlog("info", "child exited rc=%d", rc);
        return rc;
    } else if (WIFSIGNALED(status)) {
        int sig = WTERMSIG(status);
        int rc = 128 + sig;
        vlog("warn", "child killed by signal %d -> rc=%d", sig, rc);
        return rc;
    }
    vlog("warn", "child ended unexpectedly");
    return 1;
}
