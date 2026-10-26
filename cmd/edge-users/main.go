// edge-users performs explicit offline bootstrap/recovery, never prints secrets.
package main

import (
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/config"
	"kore-edge/internal/security"
	"os"
	"strings"
)

func run(args []string) error {
	f := flag.NewFlagSet("edge-users", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	action := f.String("action", "", "import-htpasswd or reset-htpasswd")
	directory := f.String("directory", "", "private users directory")
	configuration := f.String("config", "", "existing reviewed YAML")
	origin := f.String("origin", "", "HTTPS public origin")
	output := f.String("output", "", "new private YAML path")
	path := f.String("file", "", "private single-admin bcrypt file")
	if f.Parse(args) != nil || f.NArg() != 0 || (*action != "import-htpasswd" && *action != "reset-htpasswd" && *action != "prepare-config") {
		return accounts.ErrRejected
	}
	if *action == "prepare-config" {
		old, e := config.Load(*configuration)
		if e != nil {
			return e
		}
		var next config.Config
		next.Version = 1
		next.Mode = "production"
		next.HTTP = old.HTTP
		next.Security.Mode = "local_readonly"
		next.Accounts = accounts.Config{Enabled: true, Directory: *directory, PublicOrigin: *origin}
		if next.Validate() != nil {
			return accounts.ErrRejected
		}
		b, e := yaml.Marshal(next)
		if e != nil {
			return e
		}
		out, e := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		if _, e = out.Write(b); e != nil {
			out.Close()
			return e
		}
		if e = out.Sync(); e != nil {
			out.Close()
			return e
		}
		return out.Close()
	}
	b, e := security.ReadPrivateFile(*path, 4096)
	if e != nil {
		return accounts.ErrRejected
	}
	line := strings.TrimSpace(string(b))
	if strings.ContainsAny(line, "\r\n") {
		return accounts.ErrRejected
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return accounts.ErrRejected
	}
	store, e := accounts.Open(*directory)
	if e != nil {
		return e
	}
	defer store.Close()
	if *action == "reset-htpasswd" {
		return store.ResetHash(parts[0], parts[1])
	}
	return store.BootstrapHash(parts[0], parts[1])
}
func main() {
	if run(os.Args[1:]) != nil {
		fmt.Fprintln(os.Stderr, "User bootstrap failed; no credentials are printed.")
		os.Exit(1)
	}
}
