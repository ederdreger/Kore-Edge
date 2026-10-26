// edge-ops prepares releases offline. It never controls running services.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"kore-edge/internal/operations"
)

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("edge-ops", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	action := flags.String("action", "", "stage, status, select or rollback")
	directory := flags.String("directory", "", "absolute private release directory outside checkout")
	binary := flags.String("binary", "", "reviewed binary file; stage only")
	checksum := flags.String("sha256", "", "expected binary SHA256; stage only")
	configuration := flags.String("config", "", "reviewed YAML file; stage only")
	revision := flags.String("revision", "", "release ID; select only")
	generation := flags.String("expected-generation", "", "required for select/rollback")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return operations.ErrRejected
	}
	if *action != "stage" && *action != "status" && *action != "select" && *action != "rollback" {
		return operations.ErrRejected
	}
	if *action != "stage" && (*binary != "" || *checksum != "" || *configuration != "") {
		return operations.ErrRejected
	}
	if *action != "select" && *revision != "" {
		return operations.ErrRejected
	}
	var expected uint64
	if *action == "select" || *action == "rollback" {
		var e error
		expected, e = strconv.ParseUint(*generation, 10, 64)
		if e != nil {
			return operations.ErrRejected
		}
	} else if *generation != "" {
		return operations.ErrRejected
	}
	s, e := operations.Open(*directory)
	if e != nil {
		return e
	}
	defer s.Close()
	var result any
	switch *action {
	case "stage":
		result, e = s.Stage(*binary, *checksum, *configuration)
	case "status":
		result, e = s.Status()
	case "select":
		result, e = s.Select(expected, *revision)
	case "rollback":
		result, e = s.Rollback(expected)
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(result)
}
func main() {
	if run(os.Args[1:], os.Stdout) != nil {
		fmt.Fprintln(os.Stderr, "operations command failed; no configuration or credentials are printed")
		os.Exit(1)
	}
}
