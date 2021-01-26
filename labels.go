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
	"strings"
	"time"

	"github.com/nikandfor/tlog/low"
)

// Labels is a set of labels with optional values.
//
// Global Labels are attached to all the following events (in an optimal way) until replaced.
// Span Labels are attached to Span and all it's events.
type Labels []string

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
var AutoLabels = map[string]func() string{
	"_hostname":   Hostname,
	"_user":       User,
	"_os":         func() string { return runtime.GOOS },
	"_arch":       func() string { return runtime.GOARCH },
	"_numcpu":     func() string { return fmt.Sprintf("%v", runtime.NumCPU()) },
	"_gomaxprocs": func() string { return fmt.Sprintf("%v", runtime.GOMAXPROCS(0)) },
	"_goversion":  runtime.Version,
	"_pid": func() string {
		return fmt.Sprintf("%d", os.Getpid())
	},
	"_timezone": func() (n string) {
		n, _ = time.Now().Zone()
		return
	},
	"_execmd5":  ExecutableMD5,
	"_execsha1": ExecutableSHA1,
	"_execname": func() string {
		return filepath.Base(os.Args[0])
	},
	"_randid": func() string {
		return MathRandID().FullString()
	},
	"_runid": func() string {
		return low.RunID
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
func ParseLabels(s string) Labels {
	return FillLabelsWithDefaults(strings.Split(s, ",")...)
}

// FillLabelsWithDefaults creates Labels and fills autolabels (See AutoLabels).
func FillLabelsWithDefaults(labels ...string) Labels {
	ll := make(Labels, 0, len(labels))

	for _, lab := range labels {
		if f, ok := AutoLabels[lab]; ok {
			ll = append(ll, lab+"="+f())
		} else {
			ll = append(ll, lab)
		}
	}

	return ll
}

// Set sets k label value to v.
func (ls *Labels) Set(k, v string) {
	val := k
	if v != "" {
		val += "=" + v
	}

	for i := 0; i < len(*ls); i++ {
		l := (*ls)[i]
		if l == k || strings.HasPrefix(l, k+"=") {
			(*ls)[i] = val
			return
		}
	}
	*ls = append(*ls, val)
}

// Lookup gets k label value or "", false.
func (ls Labels) Lookup(k string) (string, bool) {
	for _, l := range ls {
		if l == k {
			return "", true
		} else if strings.HasPrefix(l, k+"=") {
			return l[len(k)+1:], true
		}
	}
	return "", false
}

// Del deletes label with key k.
func (ls *Labels) Del(k string) {
	for i := 0; i < len(*ls); i++ {
		li := (*ls)[i]
		if li != k && !strings.HasPrefix(li, k+"=") {
			continue
		}

		l := len(*ls) - 1
		if i < l {
			copy((*ls)[i:], (*ls)[i+1:])
		}
		*ls = (*ls)[:l]

		break
	}
}

// Merge merges two Labels sets.
func (ls *Labels) Merge(b Labels) {
	for _, add := range b {
		if add == "" {
			continue
		}
		kv := strings.SplitN(add, "=", 2)

		switch {
		case len(kv) == 1:
			ls.Set(kv[0], "")
		case kv[0] == "":
			ls.Del(kv[1])
		default:
			ls.Set(kv[0], kv[1])
		}
	}
}

// Copy copies Labels including deleted thumbstones.
func (ls Labels) Copy() Labels {
	r := make(Labels, len(ls))
	copy(r, ls)
	return r
}
