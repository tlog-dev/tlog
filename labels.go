package tlog

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type (
	// Labels is a set of labels with optional values.
	//
	// Labels are attached to all following events (in an optimal way) until replaced.
	Labels []string
)

var (
	// AutoLabels is a list of automatically filled labels
	//     _hostname - local hostname
	//     _user - current user
	//     _pid - process pid
	//     _md5 - this binary md5 hash
	//     _sha1 - this binary sha1 hash
	//     _project - project name (binary name)
	AutoLabels = map[string]func() string{
		"_hostname": Hostname,
		"_user":     User,
		"_pid": func() string {
			return fmt.Sprintf("%d", os.Getpid())
		},
		"_md5":  ExecutableMD5,
		"_sha1": ExecutableSHA1,
		"_project": func() string {
			return filepath.Base(os.Args[0])
		},
	}
)

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

	h := md5.New()
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

	h := sha1.New()
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

// Set sets k label value to v
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

// Get gets k label value or "", false
func (ls *Labels) Get(k string) (string, bool) {
	for _, l := range *ls {
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

// Merge merges two Labels sets
func (ls *Labels) Merge(b Labels) {
	for _, add := range b {
		if add == "" {
			continue
		}
		kv := strings.SplitN(add, "=", 2)
		if kv[0] == "" {
			ls.Del(kv[1])
		} else {
			ls.Set(kv[0], kv[1])
		}
	}
}

// Copy copies Labels including deleted thumbstones
func (ls *Labels) Copy() Labels {
	r := make(Labels, len(*ls))
	for i, v := range *ls {
		r[i] = v
	}
	return r
}