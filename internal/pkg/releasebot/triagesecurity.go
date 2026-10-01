package releasebot

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// verifySpool checks, from the broker, the exact layout setup.sh prepare makes, and refuses anything
// else: a root only its owner writes, shared in/ and out/ (setgid, group rwx), the broker's own work/.
func verifySpool(dir string) error {
	root, err := spoolEntry(dir, "")
	if err != nil {
		return err
	}
	if !root.IsDir() || root.Mode().Perm()&0o022 != 0 || fileUID(root) != spoolRootUID {
		return fmt.Errorf("spool %s must be a directory only root writes; run setup.sh prepare", dir)
	}
	want := map[string]os.FileMode{spoolIn: 0o770 | os.ModeSetgid, spoolOut: 0o770 | os.ModeSetgid, spoolWork: 0o700}
	for _, d := range []string{spoolIn, spoolOut, spoolWork} {
		info, lstatErr := spoolEntry(dir, d)
		if lstatErr != nil {
			return lstatErr
		}
		mode := info.Mode() & (os.ModePerm | os.ModeSetgid)
		owner := spoolRootUID
		if d == spoolWork {
			owner = os.Geteuid()
		}
		owned := fileUID(info) == owner && fileGID(info) == fileGID(root)
		if !info.IsDir() || mode != want[d] || !owned {
			return fmt.Errorf("spool %s/%s is not a %v directory with the expected owner; run setup.sh prepare",
				dir, d, want[d])
		}
	}

	return nil
}

func spoolEntry(dir, name string) (os.FileInfo, error) {
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("spool: %w", err)
	}

	return info, nil
}

// spoolRootUID owns the spool root, in/ and out/: root, so the broker cannot swap them either
// (tests, which cannot chown to root, set it to their own uid).
var spoolRootUID = 0

func fileUID(info os.FileInfo) int {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}

	return -1
}

func fileGID(info os.FileInfo) int {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Gid)
	}

	return -1
}

// lockExclusive retries for a moment: brokerRunning takes the lock shared for an instant, and a
// broker starting right then must not mistake that probe for another broker.
func lockExclusive(f *os.File) error {
	var err error
	for range 20 {
		if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return err
}

// VerifyPrivate checks the MCP config, which holds the Jenkins credential: a regular file and its
// directory, both the broker's own and closed to everyone else (no links).
func VerifyPrivate(mcpConfig string) error {
	dir, err := os.Lstat(filepath.Dir(mcpConfig))
	if err != nil {
		return fmt.Errorf("MCP config: %w", err)
	}
	if !dir.IsDir() || dir.Mode().Perm() != 0o700 || fileUID(dir) != os.Geteuid() {
		return fmt.Errorf("%s must be a 0700 directory owned by the broker", filepath.Dir(mcpConfig))
	}
	info, err := os.Lstat(mcpConfig)
	if err != nil {
		return fmt.Errorf("MCP config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || fileUID(info) != os.Geteuid() {
		return fmt.Errorf("MCP config %s must be a regular 0600 file owned by the broker", mcpConfig)
	}

	return nil
}

// AcquireBrokerLock checks the spool layout, then takes the lock prepare created in it: a regular
// 0640 file owned by the broker (the bot probes it), never followed through a link nor re-moded.
func AcquireBrokerLock(dir string) (release func(), err error) {
	if layoutErr := verifySpool(dir); layoutErr != nil {
		return nil, layoutErr
	}
	spoolGID := -1
	if root, rootErr := os.Lstat(dir); rootErr == nil {
		spoolGID = fileGID(root)
	}
	path := filepath.Join(dir, BrokerLock)
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("broker lock (run setup.sh prepare): %w", err)
	}
	info, err := f.Stat()
	if err == nil {
		st, ok := info.Sys().(*syscall.Stat_t)
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o640 || !ok || st.Nlink != 1 ||
			int(st.Uid) != os.Geteuid() || int(st.Gid) != spoolGID {
			err = fmt.Errorf("broker lock %s must be a regular 0640 file of the broker in the spool's group; "+
				"run setup.sh prepare", path)
		}
	}
	if err == nil {
		err = lockExclusive(f)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = fmt.Errorf("%w (lock %s)", ErrAlreadyRunning, path)
		}
	}
	if err != nil {
		_ = f.Close()

		return nil, err
	}

	return func() { _ = f.Close() }, nil
}

// VerifySkill checks that dir holds exactly the files listed in manifest ("<sha256>  <path>" lines,
// as sha256sum writes them), so triage runs the reviewed skill revision and nothing else.
func VerifySkill(dir, manifest string) error {
	want, err := readManifest(manifest)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	seen := 0
	walkErr := fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("unexpected symlink %s", rel)
		case d.IsDir():
			return nil
		}
		sum, listed := want[rel]
		if !listed {
			return fmt.Errorf("unexpected file %s", rel)
		}
		raw, readErr := root.ReadFile(rel)
		if readErr != nil {
			return readErr
		}
		if got := sha256.Sum256(raw); hex.EncodeToString(got[:]) != sum {
			return fmt.Errorf("%s does not match the pinned revision", rel)
		}
		seen++

		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if seen != len(want) {
		return fmt.Errorf("%d of %d pinned files are missing", len(want)-seen, len(want))
	}

	return nil
}

// readManifest reads "<sha256>  <path>" lines into path -> sum.
func readManifest(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	want := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, file, ok := strings.Cut(strings.TrimSpace(sc.Text()), "  ")
		if ok && len(sum) == 64 {
			want[file] = sum
		}
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("manifest %s lists no files", path)
	}

	return want, sc.Err()
}
