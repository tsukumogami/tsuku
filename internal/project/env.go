package project

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// ErrIsSymlink is returned by a DiscoveryEnv's OpenNoFollow when the final
// component of the path is a symlink.
//
// Opening without following a symlink fails rather than succeeding, and the
// failure is the signal that the symlink sequence applies. The production
// environment translates the platform errno into this sentinel so callers
// compare against one value instead of against ELOOP on one platform and
// something else on another.
var ErrIsSymlink = errors.New("path is a symlink")

// FileMeta is the metadata discovery reads about one path.
//
// It is a value rather than an fs.FileInfo because the two fields an
// fs.FileInfo does not carry portably -- the owner and the device/inode pair --
// are the two this package's decisions turn on, and a test has to be able to
// present them for a file it did not create.
type FileMeta struct {
	// UID is the owning user's numeric id. Numeric rather than resolved
	// through the name service: resolution is a network call on some hosts,
	// and discovery runs from the shell prompt.
	UID uint32

	// Mode carries the permission bits and the file type.
	Mode fs.FileMode

	// Dev and Ino identify the object. Comparing them across an open is what
	// makes the decision and the bytes concern the same file; a path cannot.
	Dev uint64
	Ino uint64
}

// IsSymlink reports whether the metadata describes a symbolic link.
func (m FileMeta) IsSymlink() bool { return m.Mode&fs.ModeSymlink != 0 }

// IsRegular reports whether the metadata describes an ordinary file.
func (m FileMeta) IsRegular() bool { return m.Mode.IsRegular() }

// File is an open handle discovery reads a config through.
//
// Meta reports the metadata of the object actually opened, not of the path it
// was opened by, which is the whole reason the handle is in the interface: the
// decision is made on what Meta returns and the bytes come from the same
// handle, so nothing can be swapped in between.
type File interface {
	Meta() (FileMeta, error)
	io.Reader
	io.Closer
}

// DiscoveryEnv is the filesystem and identity surface discovery uses.
//
// Every access the walk makes goes through it. That is not indirection for its
// own sake: the layouts this package has to decide about involve another user's
// files, files owned by root, and a directory nobody but an administrator could
// have created, and a test has to reach all three as an unprivileged user with
// no second account.
type DiscoveryEnv struct {
	// UID is the invoking user's numeric id, passed rather than read from the
	// process. It follows the pattern configPermissionCondition already uses.
	UID uint32

	// EvalSymlinks resolves every component of path through symlinks.
	EvalSymlinks func(path string) (string, error)

	// Lstat reads a path's own metadata without following a final symlink.
	Lstat func(path string) (FileMeta, error)

	// Readlink reads a symlink's target, as written.
	Readlink func(path string) (string, error)

	// Ancestors returns dir followed by every directory above it, ending at
	// the top of the tree this environment presents.
	//
	// The production form ends at the filesystem root. A test's form ends at
	// its fixture root, and that is the point: without it, a fixture built
	// under a temporary directory climbs into the real filesystem above it,
	// where an injected owner is not the owner of anything, and a layout that
	// must be found is refused for a reason that has nothing to do with the
	// rule under test.
	Ancestors func(dir string) []string

	// OpenNoFollow opens path for reading without following a final symlink
	// and without blocking. It returns ErrIsSymlink when the final component
	// is a link, which is the signal to run the symlink sequence rather than
	// an error to report.
	OpenNoFollow func(path string) (File, error)
}

// OSDiscoveryEnv returns the environment backed by the real filesystem and the
// invoking process's effective uid.
func OSDiscoveryEnv() DiscoveryEnv {
	return DiscoveryEnv{
		UID:          uint32(os.Geteuid()),
		EvalSymlinks: filepath.EvalSymlinks,
		Lstat:        osLstat,
		Readlink:     os.Readlink,
		Ancestors:    osAncestors,
		OpenNoFollow: osOpenNoFollow,
	}
}

func osLstat(path string) (FileMeta, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return FileMeta{}, err
	}
	return metaFromInfo(info)
}

// osAncestors walks from dir to the filesystem root.
func osAncestors(dir string) []string {
	dir = filepath.Clean(dir)
	out := []string{dir}
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		dir = parent
		out = append(out, dir)
	}
}

// osFile adapts *os.File to the File interface.
type osFile struct{ *os.File }

func (f osFile) Meta() (FileMeta, error) {
	info, err := f.Stat()
	if err != nil {
		return FileMeta{}, err
	}
	return metaFromInfo(info)
}

func osOpenNoFollow(path string) (File, error) {
	// O_NOFOLLOW so the object opened is the one the metadata was read for
	// rather than wherever a link points, and O_NONBLOCK so a named pipe at
	// this path does not wait for a writer -- which, on the shell-prompt path,
	// means every prompt.
	//
	// The same idiom and its reasoning are in
	// internal/actions/install_program_files.go.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EMLINK) {
			return nil, ErrIsSymlink
		}
		return nil, err
	}
	return osFile{f}, nil
}

func metaFromInfo(info fs.FileInfo) (FileMeta, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// Nothing to decide on. Reporting it as an error keeps the
		// "cannot determine" case failing rather than reading as uid 0.
		return FileMeta{}, errors.New("stat: no underlying syscall.Stat_t")
	}
	return FileMeta{
		UID:  st.Uid,
		Mode: info.Mode(),
		Dev:  uint64(st.Dev),
		Ino:  uint64(st.Ino),
	}, nil
}
