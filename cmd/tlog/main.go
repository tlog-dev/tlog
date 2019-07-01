package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/beorn7/perks/quantile"
	"github.com/nikandfor/app"
	"github.com/nikandfor/json"
	"github.com/pkg/errors"

	"github.com/nikandfor/tlog"
)

func main() {
	a := app.App
	a.Commands = []*app.Command{
		{
			Name:        "convert",
			Action:      convert,
			Description: "Convert logs from one format to another",
			Flags: []app.Flag{
				app.F{Name: "from", Description: "file format"}.NewString("proto"),
				app.F{Name: "to", Description: "file format"}.NewString("json"),
			},
		},
		{
			Name:        "analyze",
			Action:      analyze,
			Description: "Analyze some log statistics",
			Commands:    []*app.Command{},
			Flags: []app.Flag{
				app.F{Name: "file", Aliases: []string{"f"}, Description: "file to read"}.NewFile(""),
				app.F{Name: "format", Aliases: []string{"fmt"}, Description: "file format"}.NewString("json"),
				app.F{Name: "show-labels", Aliases: []string{"lab"}}.NewBool(true),
				app.F{Name: "show-locations", Aliases: []string{"loc"}}.NewBool(true),
				app.F{Name: "show-spans", Aliases: []string{"sp"}}.NewBool(false),
				app.F{Name: "show-messages", Aliases: []string{"msg"}}.NewBool(false),
				app.F{Name: "quantiles", Aliases: []string{"q"}}.NewString("0.5,0.9,0.99,0.999,1"),
			},
		},
	}

	app.RunAndExit(os.Args)
}

func analyze(c *app.Command) error {
	fn := c.Flag("file").VString()
	var fr io.ReadCloser
	if fn == "" || fn == "-" {
		fr = os.Stdin
	} else {
		f, err := os.Open(c.Flag("file").VString())
		if err != nil {
			return errors.Wrap(err, "open file")
		}
		defer f.Close()
		fr = f
	}

	jr := json.NewReader(fr)
	var r tlog.Reader
	switch strings.ToLower(c.Flag("fmt").VString()) {
	case "json":
		r = tlog.NewJSONReader(jr)
	default:
		return errors.New("unsupported reader format")
	}

	var lb tlog.Labels
	var ls []*tlog.LocationInfo
	var ms []*tlog.Message
	var sp []*tlog.Span

	lsm := map[tlog.Location]*tlog.LocationInfo{}
	spm := map[tlog.ID]*tlog.Span{}
	sms := map[tlog.ID][]*tlog.Message{}

loop:
	for {
		v := r.Read()
		switch v := v.(type) {
		case error:
			if v == io.EOF {
				break loop
			}
			return errors.Wrap(v, "read")
		case *tlog.LocationInfo:
			ls = append(ls, v)
			lsm[v.PC] = v
		case *tlog.Message:
			ms = append(ms, v)
			if id := v.SpanID(); id != 0 {
				sms[id] = append(sms[id], v)
			}
		case *tlog.Span:
			sp = append(sp, v)
			spm[v.ID] = v
		case *tlog.SpanFinish:
			s := spm[v.ID]
			if s == nil {
				continue
			}
			s.Elapsed = v.Elapsed
			s.Flags = v.Flags
		case tlog.Labels:
			lb.Merge(v)
		}
	}

	sort.Slice(ls, func(i, j int) bool {
		if ls[i].File == ls[j].File {
			return ls[i].Line < ls[j].Line
		}
		return ls[i].File < ls[j].File
	})

	args := c.Args()

	var l0, l1 tlog.Location
	if args.Len() > 0 {
		var err error
		l0, err = parseLocation(args[0])
		if err != nil {
			return errors.Wrap(err, "parse location")
		}
		if _, ok := lsm[l0]; !ok {
			return errors.New("no such location")
		}

		if args.Len() > 1 {
			l1, err = parseLocation(args[1])
			if err != nil {
				return errors.Wrap(err, "parse location")
			}
			if _, ok := lsm[l1]; !ok {
				return errors.New("no such location")
			}
		}
	}

	var qvs []float64
	fq := c.Flag("quantiles").VString()
	for _, a := range strings.Split(fq, ",") {
		if a == "" {
			continue
		}
		v, err := strconv.ParseFloat(a, 64)
		if err != nil {
			return errors.Wrap(err, "parse quantile")
		}
		qvs = append(qvs, v)
	}

	if c.Flag("lab").VBool() {
		fmt.Printf("Labels: %q\n", lb)
	}

	if c.Flag("loc").VBool() {
		fmt.Printf("Locations: %d\n", len(ls))
		for _, l := range ls {
			fmt.Printf("  %8x %-20v %-4d %v\n", uintptr(l.PC), l.Func, l.Line, l.File)
		}
	}

	if c.Flag("sp").VBool() {
		fmt.Printf("Spans: %d\n", len(sp))
		for _, s := range sp {
			if l0 != 0 {
				if l0 != s.Location && l1 != s.Location {
					continue
				}
			}
			l := lsm[s.Location]
			if l == nil {
				l = &tlog.LocationInfo{PC: s.Location}
			}
			fmt.Printf("  %v %v   %8x [%x] %-20v %v %v\n", s.ID, s.Parent, uintptr(s.Location), s.Flags, l.Func, s.Started.Format("2006.05.04_03:02:01.000000"), s.Elapsed)
		}
	}

	if c.Flag("msg").VBool() {
		fmt.Printf("Messages: %d\n", len(ms))
		for _, m := range ms {
			if l0 != 0 {
				if l0 != m.Location && l1 != m.Location {
					continue
				}
			}
			var sid tlog.ID
			var tm string
			if len(m.Args) == 1 {
				sid = m.Args[0].(tlog.ID)
				tm = fmt.Sprintf("+%12.3fms", m.Time.Seconds()*1000)
			} else {
				tm = m.AbsTime().Format("03:02:01.000000")
			}
			fmt.Printf("  %v   %8x  %v   %v\n", sid, uintptr(m.Location), tm, m.Format)
		}
	}

	q := quantile.NewHighBiased(0.0001)

	if l1 == 0 {
		for _, s := range sp {
			if s.Location == l0 {
				q.Insert(s.Elapsed.Seconds())
			}
		}
	}

out:
	for _, m := range ms {
		if l0 == m.Location {
			if l1 == 0 {
				q.Insert(m.Time.Seconds())
			} else if id := m.SpanID(); id != 0 {
				sp := spm[id]
				if l1 == sp.Location {
					q.Insert(m.Time.Seconds())
				} else {
					for _, sm := range sms[id] {
						if l1 == sm.Location {
							q.Insert((m.Time - sm.Time).Seconds())
							continue out
						}
					}
					tlog.Printf("couldn't find diff for msg %v %v", m.Time, m.Format)
				}
			}
		} else if l1 == m.Location {
			if id := m.SpanID(); id != 0 {
				sp := spm[id]
				if l0 == sp.Location {
					q.Insert((sp.Elapsed - m.Time).Seconds())
				}
			}
		}
	}

	fmt.Printf("timings: (%d measurements)\n", q.Count())
	for _, qv := range qvs {
		fmt.Printf("  %.3f - %8.3f\n", qv, q.Query(qv)*1000)
	}

	return nil
}

func parseLocation(a string) (l0 tlog.Location, err error) {
	_, err = fmt.Sscanf(a, "%x", &l0)
	if err != nil {
		return
	}
	return
}

func convert(c *app.Command) error {
	return nil
}
