// +build linux darwin

package main

import (
	"flag"
	"log/syslog"

	"github.com/nikandfor/tlog"
)

var (
	syslogAddr  = flag.String("syslog", "/run/systemd/journal/syslog", "syslog socket address")
	syslogProto = flag.String("syslog-proto", "", "syslog address type")
)

func main() {
	flag.Parse()

	sl, err := syslog.Dial(*syslogProto, *syslogAddr, syslog.LOG_INFO|syslog.LOG_DAEMON, "tlogdemotag")
	if err != nil {
		tlog.Fatalf("dial syslog: %v", err)
	}

	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(sl, 0))

	tlog.Printf("tlog message to syslog")
}
