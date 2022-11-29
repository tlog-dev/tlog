package tlog

import (
	"crypto/md5"  //nolint
	"crypto/sha1" //nolint
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// AutoLabels is a list of automatically filled labels
//     _hostname - local hostname
//     _user - current user
//     _pid - process pid
//     _timezone - local timezone code (UTC, MSK)
//     _goversion - go version
//     _execmd5 - this binary md5 hash
//     _execsha1 - this binary sha1 hash
//     _execname - executable base name (project name)
//     _randid - random id. May be used to distinguish different runs.
var AutoLabels = map[string]func() interface{}{
	"_hostname":   func() interface{} { return Hostname() },
	"_user":       func() interface{} { return User() },
	"_os":         func() interface{} { return runtime.GOOS },
	"_arch":       func() interface{} { return runtime.GOARCH },
	"_numcpu":     func() interface{} { return fmt.Sprintf("%v", runtime.NumCPU()) },
	"_gomaxprocs": func() interface{} { return fmt.Sprintf("%v", runtime.GOMAXPROCS(0)) },
	"_goversion":  func() interface{} { return runtime.Version },
	"_pid": func() interface{} {
		return os.Getpid()
	},
	"_timezone": func() interface{} {
		n, _ := time.Now().Zone()
		return n
	},
	"_execmd5":  func() interface{} { return ExecutableMD5() },
	"_execsha1": func() interface{} { return ExecutableSHA1() },
	"_execname": func() interface{} {
		return filepath.Base(os.Args[0])
	},
	"_randid": func() interface{} {
		return MathRandID().StringFull()
	},
}

// Hostname returns hostname or err.Error().
func Hostname() string {
	h, err := os.Hostname()
	if h == "" && err != nil {
		h = err.Error()
	}

	return h
}

// User returns current username or err.Error().
func User() string {
	u, err := user.Current()
	if u != nil && u.Username != "" {
		return u.Username
	} else if err != nil {
		return err.Error()
	}

	return ""
}

// ExecutableMD5 returns current process executable md5 hash.
// May be useful to find exact executable later.
func ExecutableMD5() string {
	path, err := os.Executable()
	if err != nil {
		return err.Error()
	}

	f, err := os.Open(path)
	if err != nil {
		return err.Error()
	}
	defer f.Close()

	h := md5.New() //nolint
	_, err = io.Copy(h, f)
	if err != nil {
		return err.Error()
	}

	return fmt.Sprintf("%02x", h.Sum(nil))
}

// ExecutableSHA1 returns current process executable sha1 hash.
// May be useful to find exact executable later.
func ExecutableSHA1() string {
	path, err := os.Executable()
	if err != nil {
		return err.Error()
	}

	f, err := os.Open(path)
	if err != nil {
		return err.Error()
	}
	defer f.Close()

	h := sha1.New() //nolint
	_, err = io.Copy(h, f)
	if err != nil {
		return err.Error()
	}

	return fmt.Sprintf("%02x", h.Sum(nil))
}

// ParseLabels parses comma separated list of labels and fills them with values (See FillLabelsWithDefaults).
func ParseLabels(s string) []interface{} {
	l := strings.Split(s, ",")

	res := make([]interface{}, 0, len(l)*2)

	for _, l := range l {
		if l == "" {
			continue
		}

		p := strings.IndexByte(l, '=')
		if p == -1 {
			if f, ok := AutoLabels[l]; ok {
				res = append(res, l, f())
			} else {
				res = append(res, l, None)
			}

			continue
		}

		var k, v = l[:p], l[p+1:]

		if x, err := strconv.Atoi(v); err == nil {
			res = append(res, k, x)
			continue
		}

		res = append(res, k, v)
	}

	return res
}
