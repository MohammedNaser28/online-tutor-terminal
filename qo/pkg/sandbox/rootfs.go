package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ahmedYasserM/qo/pkg/logger"
)

const maxConcurrentSessions = 8

// checkConcurrencyCap checks if we're at the maximum concurrent sessions
func checkConcurrencyCap() error {
	lockFile := filepath.Join("/tmp", "qo-sessions.lock")
	file, err := os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open concurrency lock: %w", err)
	}
	defer file.Close()

	// Try to get an exclusive lock
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("concurrent sessions limit reached")
	}

	// Read current session count
	countBytes, err := os.ReadFile(lockFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read session count: %w", err)
	}

	count := 0
	if len(countBytes) > 0 {
		count, err = strconv.Atoi(string(countBytes))
		if err != nil {
			return fmt.Errorf("invalid session count in lock file: %w", err)
		}
	}

	// Check if we're at capacity
	if count >= maxConcurrentSessions {
		return fmt.Errorf("concurrent sessions limit reached")
	}

	// Increment count
	count++

	// Write updated count
	if err := os.WriteFile(lockFile, []byte(strconv.Itoa(count)), 0644); err != nil {
		return fmt.Errorf("failed to write session count: %w", err)
	}

	return nil
}

// releaseConcurrencyCap releases the concurrency cap
func releaseConcurrencyCap() {
	lockFile := filepath.Join("/tmp", "qo-sessions.lock")
	file, err := os.OpenFile(lockFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to open concurrency lock for release: %v", err))
		return
	}
	defer file.Close()

	// Release the lock
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		logger.Warn(fmt.Sprintf("Failed to release concurrency lock: %v", err))
		return
	}

	// Decrement count
	countBytes, err := os.ReadFile(lockFile)
	if err == nil && len(countBytes) > 0 {
		sessionCount, err := strconv.Atoi(string(countBytes))
		if err == nil && sessionCount > 0 {
			sessionCount--
			if err := os.WriteFile(lockFile, []byte(strconv.Itoa(sessionCount)), 0644); err != nil {
				logger.Warn(fmt.Sprintf("Failed to decrement session count: %v", err))
			}
		}
	}

	// Clean up lock file if empty
	sessionCount, _ := strconv.Atoi(string(countBytes))
	if sessionCount <= 0 {
		os.Remove(lockFile)
	}
}

//go:embed rootfs.tar.gz
var embeddedRootfs []byte

const sessionsDir = "/tmp/qo-sessions"
const defaultUser string = "ahmed"

// GenerateSessionPath creates a unique per-session path with random suffix
func GenerateSessionPath(studentID string) (string, error) {
	rand.Seed(time.Now().UnixNano())
	suffix := fmt.Sprintf("%04x", rand.Int31())
	sessionID := fmt.Sprintf("%s-%s", studentID, suffix)
	sessionPath := filepath.Join(sessionsDir, sessionID)
	return sessionPath, nil
}

// setupUserNamespaceMapping sets up UID/GID mapping for the new user namespace
func setupUserNamespaceMapping(pid int) error {
	uidMapPath := fmt.Sprintf("/proc/%d/uid_map", pid)
	gidMapPath := fmt.Sprintf("/proc/%d/gid_map", pid)
	setgroupsPath := fmt.Sprintf("/proc/%d/setgroups", pid)

	// Get current host UID/GID to ensure files in rootfs are writable
	hostUID := os.Getuid()
	hostGID := os.Getgid()

	// Write setgroups: deny before gid_map
	if err := os.WriteFile(setgroupsPath, []byte("deny"), 0644); err != nil {
		return fmt.Errorf("failed to write setgroups: %w", err)
	}

	// Map student UID to UID 0 inside namespace, and to the current host UID outside
	// This ensures the student can write to files in the rootfs (which are owned by hostUID)
	if err := os.WriteFile(uidMapPath, []byte(fmt.Sprintf("0 %d 1", hostUID)), 0644); err != nil {
		return fmt.Errorf("failed to write uid_map: %w", err)
	}

	// Map student GID to GID 0 inside namespace, and to the current host GID outside
	if err := os.WriteFile(gidMapPath, []byte(fmt.Sprintf("0 %d 1", hostGID)), 0644); err != nil {
		return fmt.Errorf("failed to write gid_map: %w", err)
	}

	return nil
}

// setupCgroupV2 creates a cgroup for the session and sets resource limits
func setupCgroupV2(sessionID string, pid int) error {
	cgroupPath := filepath.Join("/sys/fs/cgroup", sessionsDir, sessionID)

	// Create the cgroup directory
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup directory: %w", err)
	}

	// Enable required controllers via subtree_control
	controllers := []string{"memory", "pids", "cpu"}
	for _, controller := range controllers {
		controlPath := filepath.Join(cgroupPath, "cgroup.subtree_control")
		if err := os.WriteFile(controlPath, []byte("+"+controller), 0644); err != nil {
			logger.Warn(fmt.Sprintf("Failed to enable %s controller: %v", controller, err))
			// Don't fail if controller enablement fails - some controllers may not be available
		}
	}

	// Set memory limit (default 512M)
	memoryPath := filepath.Join(cgroupPath, "memory.max")
	if err := os.WriteFile(memoryPath, []byte("512M"), 0644); err != nil {
		return fmt.Errorf("failed to set memory limit: %w", err)
	}

	// Set PIDs limit (default 200)
	pidsPath := filepath.Join(cgroupPath, "pids.max")
	if err := os.WriteFile(pidsPath, []byte("200"), 0644); err != nil {
		return fmt.Errorf("failed to set PIDs limit: %w", err)
	}

	// Set CPU weight (default 1000, which is the default weight)
	cpuPath := filepath.Join(cgroupPath, "cpu.max")
	if err := os.WriteFile(cpuPath, []byte(fmt.Sprintf("%d %d", 1000*1000, 1000*1000)), 0644); err != nil {
		return fmt.Errorf("failed to set CPU weight: %w", err)
	}

	// Move the child process into the cgroup
	procsPath := filepath.Join(cgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsPath, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		return fmt.Errorf("failed to move process into cgroup: %w", err)
	}

	return nil
}

// cleanupSession performs cleanup after the session ends
func cleanupSession(rootfsPath string, sessionID string) {
	// Unmount /proc
	if err := syscall.Unmount(rootfsPath+"/proc", 0); err != nil && !os.IsNotExist(err) {
		logger.Warn(fmt.Sprintf("Failed to unmount /proc: %v", err))
	}

	// Remove the per-session rootfs directory
	if err := os.RemoveAll(rootfsPath); err != nil {
		logger.Warn(fmt.Sprintf("Failed to remove rootfs directory: %v", err))
	}

	// Remove the cgroup
	cgroupPath := filepath.Join("/sys/fs/cgroup", sessionsDir, sessionID)
	if err := os.RemoveAll(cgroupPath); err != nil && !os.IsNotExist(err) {
		logger.Warn(fmt.Sprintf("Failed to remove cgroup: %v", err))
	}

	logger.Info("Session cleaned up successfully")
}

// PathExists checks if a file or directory exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// ExtractRootfs extracts the tar-archived rootfs folder to the specified path
func ExtractRootfs(rootfsPath string) error {
	if pathExists(rootfsPath) {
		_ = syscall.Unmount(filepath.Join(rootfsPath, "proc"), syscall.MNT_FORCE) // force unmount of /proc to handle possible previous exits using external kill signal

		if err := os.RemoveAll(rootfsPath); err != nil {
			return err
		}
	}

	gzReader, err := gzip.NewReader(io.NopCloser(bytes.NewReader(embeddedRootfs)))
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // done
		}
		if err != nil {
			return err
		}

		destPath := filepath.Join(rootfsPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}

			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}

			if err := os.Symlink(header.Linkname, destPath); err != nil {
				return err
			}

		case tar.TypeChar:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			if err := syscall.Mknod(destPath, syscall.S_IFCHR|uint32(os.FileMode(header.Mode)&0777), int(mkdev(uint64(header.Devmajor), uint64(header.Devminor)))); err != nil {
				return err
			}
		}
	}

	binDir := filepath.Join(rootfsPath, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	applets := []string{"sleep", "kill", "pkill", "killall", "stat", "passwd", "chpasswd", "adduser", "addgroup", "deluser", "delgroup", "clear", "reset", "du", "df", "free", "uptime", "time", "cut", "tr", "od", "base64", "bc", "expr", "factor", "seq"}
	for _, applet := range applets {
		linkPath := filepath.Join(binDir, applet)
		if _, err := os.Stat(linkPath); os.IsNotExist(err) {
			os.Symlink("busybox", linkPath)
		}
	}

	return nil
}

func mkdev(major, minor uint64) uint64 {
	return (major & 0xfff) << 8 | (minor & 0xff) | (minor & 0xfff00) << 12
}

func createDevices(rootfsPath string) {
	devices := []struct {
		path   string
		major  int
		minor  int
	}{
		{"/dev/null", 1, 3},
		{"/dev/zero", 1, 5},
		{"/dev/random", 1, 8},
		{"/dev/urandom", 1, 9},
	}

	for _, dev := range devices {
		devPath := filepath.Join(rootfsPath, dev.path)
		if err := os.MkdirAll(filepath.Dir(devPath), 0755); err != nil {
			continue
		}

		// Try bind-mount from host (works when parent ns shares mounts)
		if _, err := os.Stat(dev.path); err == nil {
			if _, err := os.Stat(devPath); os.IsNotExist(err) {
				os.WriteFile(devPath, []byte{}, 0666)
			}
			if err := syscall.Mount(dev.path, devPath, "", syscall.MS_BIND, ""); err == nil {
				continue
			}
		}

		// Try mknod
		devNum := (dev.major << 8) | dev.minor
		if err := syscall.Mknod(devPath, syscall.S_IFCHR|0666, devNum); err == nil {
			continue
		}

		// Fallback: regular file with open permissions
		os.WriteFile(devPath, []byte{}, 0666)
	}
}

func StartSandBox(rootfsPath string, duration time.Duration) error {

	createDevices(rootfsPath)

	if len(os.Args) > 0 && os.Args[0] == "init" {
		// Release concurrency cap when child process starts
		releaseConcurrencyCap()
		if err := syscall.Chroot(rootfsPath); err != nil {
			return err
		}

		if err := os.Chdir("/tmp"); err != nil {
			return err
		}

		if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
			return err
		}

		logger.Info("You are now inside the isolated enviornemnt.")

		cmd := exec.Command("/bin/bash")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()

		return err
	}

	// Check concurrency cap before starting child process
	if err := checkConcurrencyCap(); err != nil {
		return err
	}

	cmd := exec.Command("/proc/self/exe")
	cmd.Args = []string{"init", rootfsPath}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
		Setpgid:    true,
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Set up UID/GID mapping after fork but before child execs
	if err := setupUserNamespaceMapping(cmd.Process.Pid); err != nil {
		logger.Error(fmt.Errorf("failed to setup user namespace mapping: %w", err))
		cmd.Process.Kill()
		return err
	}

	// Set up cgroup v2 resource limits
	sessionID := filepath.Base(rootfsPath)
	if err := setupCgroupV2(sessionID, cmd.Process.Pid); err != nil {
		logger.Error(fmt.Errorf("failed to setup cgroup v2: %w", err))
		cmd.Process.Kill()
		return err
	}

	// Start duration timer if set
	if duration > 0 {
		go func() {
			time.Sleep(duration)
			logger.Warn(fmt.Sprintf("Duration elapsed, terminating session"))
			// Try SIGTERM first
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			time.Sleep(5 * time.Second)
			// SIGKILL after grace period
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}()
	}

	// Setup cleanup on exit
	cleanupDone := make(chan bool, 1)
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Warn(fmt.Sprintf("Session exited with error: %v", err))
		}
		cleanupDone <- true
	}()

	// Wait for cleanup to complete
	<-cleanupDone

	// Perform cleanup
	cleanupSession(rootfsPath, sessionID)

	return nil
}

