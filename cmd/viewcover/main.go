// Command viewcover reports the views that hold a condition and that no
// test rendered.
//
// It reads a `go test -v` run made with HML_TRACE=1, from stdin or from
// -trace, and compares the paths the run printed against the .hml files
// under -views whose template has a condition. It prints one line per
// missed view to stderr and exits 1 when there is one. See package
// github.com/croaky/hml/viewcover for why, and for what the trace is.
//
// An app pins it like any other tool:
//
//	go get -tool github.com/croaky/hml/cmd/viewcover
//	HML_TRACE=1 go test -v ./... > run.txt
//	go tool viewcover -views ui/views -trace run.txt
//
// The paths the app traces must be the ones the walk produces: with
// -views ui/views, the walk yields "ui/views/companies/edit.hml", so
// that is what the app hands to viewcover.Trace.
//
// -transform names the transforms the views call, so they parse. The
// command registers each name as the identity, which is enough: it
// parses the views and never renders them.
//
// It never runs the tests itself. Run the suite once, traced, and pipe
// it here, so one run answers both checks and there is one way the
// trace is made.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/croaky/hml"
	"github.com/croaky/hml/viewcover"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("viewcover: ")

	var views, tracePath, transformList string
	flag.StringVar(&views, "views", "", "directory of .hml views to check (required)")
	flag.StringVar(&tracePath, "trace", "", "a saved traced run to read instead of stdin")
	flag.StringVar(&transformList, "transform", "", "comma-separated transform names the views call")
	flag.Parse()

	if views == "" {
		flag.Usage()
		os.Exit(2)
	}
	views = strings.TrimSuffix(views, "/")

	transforms := map[string]hml.Transform{}
	for name := range strings.SplitSeq(transformList, ",") {
		if name = strings.TrimSpace(name); name != "" {
			transforms[name] = func(s string) string { return s }
		}
	}

	var in io.Reader = os.Stdin
	if tracePath != "" {
		f, err := os.Open(tracePath)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		in = f
	}

	rendered, err := viewcover.ParseTrace(in)
	if err != nil {
		log.Fatal(err)
	}
	conditional, err := viewcover.Conditional(os.DirFS("."), views, transforms)
	if err != nil {
		log.Fatal(err)
	}
	missed, err := viewcover.Report(conditional, rendered)
	if err != nil {
		log.Fatal(err)
	}

	for _, v := range missed {
		fmt.Fprintf(os.Stderr, "no test renders: %s\n", v)
	}
	if n := len(missed); n > 0 {
		log.Fatalf("%d view(s) with a condition and no test", n)
	}
}
