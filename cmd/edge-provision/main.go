// edge-provision is an offline laboratory issuer. It never prints bearer tokens.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/provision"
)

func run(args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("edge-provision", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	action := flags.String("action", "", "issue, revoke or status")
	directory := flags.String("directory", "", "absolute private runtime directory outside checkout")
	tokenOut := flags.String("token-out", "", "new credential file inside private directory; issue only")
	tokenID := flags.String("token-id", "", "public token identifier; revoke only")
	edgeID := flags.String("edge-id", "", "edge identifier; status only")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return provision.ErrRejected
	}
	if *action != "issue" && *action != "revoke" && *action != "status" {
		return provision.ErrRejected
	}
	var assignment provision.Assignment
	if *action == "issue" {
		body, e := io.ReadAll(io.LimitReader(input, 4097))
		if e != nil || len(body) > 4096 || catalog.DecodeStrict(body, &assignment) != nil {
			return provision.ErrRejected
		}
	}
	registry, e := provision.Open(*directory)
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	switch *action {
	case "issue":
		// A token file is explicit, new, owner-only and confined to the private runtime
		// directory. Never accept stdout, checkout paths or overwriting registry state.
		path := filepath.Clean(*tokenOut)
		if !filepath.IsAbs(path) || filepath.Dir(path) != filepath.Clean(*directory) || filepath.Base(path) == "registry.json" || filepath.Base(path) == "registry.lock" || filepath.Ext(path) != ".json" {
			return provision.ErrRejected
		}
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return provision.ErrStorage
		}
		committed := false
		defer func() {
			file.Close()
			if !committed {
				_ = os.Remove(path)
			}
		}()
		credential, e := registry.Issue(context.Background(), assignment, now)
		if e != nil {
			return e
		}
		if json.NewEncoder(file).Encode(credential) != nil || file.Sync() != nil || file.Close() != nil {
			_ = registry.Revoke(context.Background(), credential.ID, now)
			return provision.ErrStorage
		}
		committed = true
		return json.NewEncoder(output).Encode(credential.TokenInfo)
	case "revoke":
		if registry.Revoke(context.Background(), *tokenID, now) != nil {
			return provision.ErrRejected
		}
		return json.NewEncoder(output).Encode(map[string]string{"state": "revoked"})
	case "status":
		status, e := registry.Status(context.Background(), *edgeID)
		if e != nil {
			return e
		}
		return json.NewEncoder(output).Encode(status)
	}
	return errors.New("unsupported action")
}
func main() {
	if run(os.Args[1:], os.Stdin, os.Stdout) != nil {
		fmt.Fprintln(os.Stderr, "provisioning command failed; no credentials are printed")
		os.Exit(1)
	}
}
