#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/resource.h>
#include <sys/prctl.h>
#include <string.h>
#include <errno.h>
#include <fcntl.h>
#include <sys/ioctl.h>
#include <net/if.h>
#include <linux/capability.h>
#include <seccomp.h>
#include <dirent.h>

#define STACK_SIZE (1024 * 1024)

static int write_userns_map(const char *path, const char *content) {
    int fd = open(path, O_WRONLY);
    if (fd < 0) {
        perror(path);
        return -1;
    }
    ssize_t len = strlen(content);
    if (write(fd, content, len) != len) {
        perror("write map");
        close(fd);
        return -1;
    }
    close(fd);
    return 0;
}

static int setup_userns(uid_t host_uid, gid_t host_gid) {
    char buf[64];

    if (write_userns_map("/proc/self/setgroups", "deny") != 0) {
        return -1;
    }

    snprintf(buf, sizeof(buf), "0 %d 1\n", (int)host_gid);
    if (write_userns_map("/proc/self/gid_map", buf) != 0) {
        return -1;
    }

    snprintf(buf, sizeof(buf), "0 %d 1\n", (int)host_uid);
    if (write_userns_map("/proc/self/uid_map", buf) != 0) {
        return -1;
    }

    return 0;
}

struct child_args {
    const char *rootfsPath;
    uid_t host_uid;
    gid_t host_gid;
};

static void mount_pseudo_fs(const char *base) {
    char mountPath[4096];

    snprintf(mountPath, sizeof(mountPath), "%s/proc", base);
    if (mount("proc", mountPath, "proc", MS_NOSUID | MS_NOEXEC | MS_NODEV, NULL) != 0) {
        if (errno != EBUSY) {
            perror("mount /proc");
        }
    }

    snprintf(mountPath, sizeof(mountPath), "%s/dev", base);
    if (mount("devtmpfs", mountPath, "devtmpfs", MS_NOSUID | MS_NOEXEC, NULL) != 0) {
        if (errno != EBUSY) {
            perror("mount /dev (devtmpfs)");
        }
    }

    snprintf(mountPath, sizeof(mountPath), "%s/dev/pts", base);
    mkdir(mountPath, 0755);
    if (mount("devpts", mountPath, "devpts", MS_NOSUID | MS_NOEXEC, "newinstance,ptmxmode=0666,mode=0620") != 0) {
        if (errno != EBUSY) {
            perror("mount /dev/pts");
        }
    }
}

static int switch_root(const char *rootfsPath) {
    char chrootPath[4096];
    char oldRoot[4096];
    snprintf(chrootPath, sizeof(chrootPath), "%s/rootfs", rootfsPath);
    snprintf(oldRoot, sizeof(oldRoot), "%s/.pivot_old", chrootPath);

    if (mount(chrootPath, chrootPath, "bind", MS_BIND | MS_REC, "") != 0) {
        perror("mount bind");
        return -1;
    }

    if (mkdir(oldRoot, 0700) != 0 && errno != EEXIST) {
        perror("mkdir pivot_old");
        return -1;
    }

    mount_pseudo_fs(chrootPath);

    if (chdir(chrootPath) != 0) {
        perror("chdir rootfs");
        return -1;
    }

    if (syscall(SYS_pivot_root, chrootPath, oldRoot) != 0) {
        perror("pivot_root");
        return -1;
    }

    if (chdir("/") != 0) {
        perror("chdir /");
        return -1;
    }

    if (umount2("/.pivot_old", MNT_DETACH) != 0) {
        perror("umount pivot_old");
    }

    if (rmdir("/.pivot_old") != 0 && errno != ENOENT) {
        perror("rmdir pivot_old");
    }

    return 0;
}

static int setup_loopback(void) {
    int sock = socket(AF_INET, SOCK_DGRAM, 0);
    if (sock < 0) {
        perror("socket");
        return -1;
    }

    struct ifreq ifr;
    memset(&ifr, 0, sizeof(ifr));
    strncpy(ifr.ifr_name, "lo", IFNAMSIZ - 1);
    ifr.ifr_flags = IFF_UP | IFF_LOOPBACK | IFF_RUNNING;

    if (ioctl(sock, SIOCSIFFLAGS, &ifr) != 0) {
        perror("ioctl SIOCSIFFLAGS");
        close(sock);
        return -1;
    }

    close(sock);
    return 0;
}

static void drop_capabilities(void) {
    struct __user_cap_header_struct hdr = { .version = _LINUX_CAPABILITY_VERSION_3, .pid = 0 };
    struct __user_cap_data_struct data[2] = {{0}};

    if (syscall(SYS_capset, &hdr, data) != 0) {
        perror("capset");
    }
}

static void set_resource_limits(void) {
    struct rlimit rlim;

    rlim.rlim_cur = rlim.rlim_max = 1024;
    setrlimit(RLIMIT_NOFILE, &rlim);

    rlim.rlim_cur = rlim.rlim_max = 128;
    setrlimit(RLIMIT_NPROC, &rlim);

    rlim.rlim_cur = rlim.rlim_max = 0;
    setrlimit(RLIMIT_CORE, &rlim);

    rlim.rlim_cur = rlim.rlim_max = 10485760;
    setrlimit(RLIMIT_FSIZE, &rlim);
}

static void setup_seccomp(void) {
    const char *mode = getenv("QO_SECCOMP");
    if (mode == NULL || strcmp(mode, "off") == 0) {
        return;
    }

    scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
    if (ctx == NULL) {
        fprintf(stderr, "Failed to initialize seccomp\n");
        return;
    }

    uint32_t action = SCMP_ACT_LOG;
    if (strcmp(mode, "enforce") == 0) {
        action = SCMP_ACT_KILL_PROCESS;
    }

    seccomp_rule_add(ctx, action, SCMP_SYS(reboot), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(mount), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(umount2), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(pivot_root), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(unshare), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(setns), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(init_module), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(finit_module), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(delete_module), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(kexec_load), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(personality), 0);
    seccomp_rule_add(ctx, action, SCMP_SYS(ptrace), 0);

    if (seccomp_load(ctx) != 0) {
        fprintf(stderr, "Failed to load seccomp filter\n");
    }

    seccomp_release(ctx);
}

static int spawn_shell(const char *rootfsPath) {
    pid_t shell_pid = fork();
    if (shell_pid < 0) {
        perror("fork");
        return -1;
    }

    if (shell_pid == 0) {
        if (setsid() < 0) {
            perror("setsid");
        }
        if (ioctl(0, TIOCSCTTY, 1) < 0) {
            perror("ioctl TIOCSCTTY");
        }

        const char *student_name = getenv("QO_STUDENT_NAME");
        if (student_name) {
            char ps1[256];
            snprintf(ps1, sizeof(ps1), "root@%s:~# ", student_name);
            setenv("PS1", ps1, 1);
        }

        if (chdir("/root") != 0) {
            perror("chdir /root");
            _exit(1);
        }

        FILE *bashrc = fopen("/root/.bashrc", "a");
        if (bashrc) {
            fprintf(bashrc, "\n");
            fprintf(bashrc, "__qo_challenge() {\n");
            fprintf(bashrc, "    local action=\"$1\"\n");
            fprintf(bashrc, "    if [ \"$action\" = \"go\" ]; then\n");
            fprintf(bashrc, "        local ans\n");
            fprintf(bashrc, "        read -p \"Your answer: \" ans\n");
            fprintf(bashrc, "        action=\"go:${ans}\"\n");
            fprintf(bashrc, "    fi\n");
            fprintf(bashrc, "    local req=\"/tmp/.qo-challenge-req\"\n");
            fprintf(bashrc, "    local resp=\"/tmp/.qo-challenge-resp\"\n");
            fprintf(bashrc, "    local tmpReq=\"/tmp/.qo-challenge-req.tmp\"\n");
            fprintf(bashrc, "    printf '%%s' \"$action\" > \"$tmpReq\"\n");
            fprintf(bashrc, "    mv -f \"$tmpReq\" \"$req\"\n");
            fprintf(bashrc, "    echo \"__qo_debug action=$action\" > /tmp/.qo-challenge-debug\n");
            fprintf(bashrc, "    local i=1\n");
            fprintf(bashrc, "    while [ $i -le 50 ]; do\n");
            fprintf(bashrc, "        if [ -s \"$resp\" ]; then\n");
            fprintf(bashrc, "            cat \"$resp\"\n");
            fprintf(bashrc, "            > \"$resp\"\n");
            fprintf(bashrc, "            return\n");
            fprintf(bashrc, "        fi\n");
            fprintf(bashrc, "        sleep 0.1\n");
            fprintf(bashrc, "        i=$((i+1))\n");
            fprintf(bashrc, "    done\n");
            fprintf(bashrc, "    echo \"Error: server not responding\"\n");
            fprintf(bashrc, "}\n");
            fprintf(bashrc, "alias quest='__qo_challenge quest'\n");
            fprintf(bashrc, "alias hint='__qo_challenge hint'\n");
            fprintf(bashrc, "alias go='__qo_challenge go'\n");
            fprintf(bashrc, "alias map='__qo_challenge map'\n");
            fprintf(bashrc, "alias status='__qo_challenge status'\n");
            fprintf(bashrc, "alias logo='__qo_challenge logo'\n");
            fprintf(bashrc, "alias help='__qo_challenge help'\n");
            fprintf(bashrc, "alias clear='printf \"\\033[2J\\033[H\"'\n");
            fclose(bashrc);
        }

        execl("/bin/bash", "/bin/bash", "-i", NULL);
        perror("execl");
        _exit(1);
    }

    int status = 0;
    pid_t pid;
    while ((pid = waitpid(-1, &status, 0)) > 0) {
        if (pid == shell_pid) {
            if (WIFEXITED(status)) {
                return WEXITSTATUS(status);
            } else if (WIFSIGNALED(status)) {
                return 128 + WTERMSIG(status);
            }
            return 1;
        }
    }

    if (pid < 0 && errno == ECHILD) {
        return 1;
    }

    return 1;
}

static int child(void *arg) {
    struct child_args *ca = (struct child_args *)arg;

    if (getenv("QO_NO_USERNS") == NULL && setup_userns(ca->host_uid, ca->host_gid) != 0) {
        fprintf(stderr, "qo-init: user namespace mapping failed, requesting fallback\n");
        return 42;
    }

    if (mount(NULL, "/", NULL, MS_REC | MS_PRIVATE, NULL) != 0) {
        perror("mount private");
        return 1;
    }

    if (setup_loopback() != 0) {
        fprintf(stderr, "Failed to setup loopback interface\n");
        return 1;
    }

    if (switch_root(ca->rootfsPath) != 0) {
        fprintf(stderr, "pivot_root failed, falling back to chroot\n");
        char chrootPath[4096];
        snprintf(chrootPath, sizeof(chrootPath), "%s/rootfs", ca->rootfsPath);
        if (chroot(chrootPath) != 0) {
            perror("chroot fallback");
            return 1;
        }
        mount_pseudo_fs("/");
    }

    prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0);
    drop_capabilities();
    set_resource_limits();
    setup_seccomp();

    return spawn_shell(ca->rootfsPath);
}

static int run_child(const char *rootfsPath, unsigned long clone_flags) {
    char *stack = malloc(STACK_SIZE);
    if (!stack) {
        perror("malloc");
        return 42;
    }

    struct child_args ca = {
        .rootfsPath = rootfsPath,
        .host_uid = getuid(),
        .host_gid = getgid(),
    };

    pid_t pid = clone(child, stack + STACK_SIZE,
                      clone_flags | SIGCHLD,
                      &ca);
    if (pid == -1) {
        perror("clone");
        free(stack);
        return 42;
    }

    int status = 0;
    waitpid(pid, &status, 0);
    free(stack);

    if (WIFEXITED(status)) {
        return WEXITSTATUS(status);
    }
    if (WIFSIGNALED(status)) {
        return 128 + WTERMSIG(status);
    }
    return 1;
}

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <rootfs-path>\n", argv[0]);
        return 1;
    }

    unsigned long flags = CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS |
                          CLONE_NEWNET | CLONE_NEWIPC | CLONE_NEWCGROUP;

    int ret = run_child(argv[1], flags | CLONE_NEWUSER);
    if (ret == 42) {
        fprintf(stderr, "qo-init: user namespace unavailable, continuing without it\n");
        setenv("QO_NO_USERNS", "1", 1);
        ret = run_child(argv[1], flags);
    }

    return ret;
}
