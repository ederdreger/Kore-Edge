package installer_test

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPanelHTTPSProbeWaitsAndDiagnosesWithoutSecrets(t *testing.T) {
	for _, tc := range []struct {
		name, scenario, diagnostic string
		calls                      int
		ok                         bool
	}{
		{"listener starting", "starting", "", 3, true},
		{"upstream starting", "upstream", "", 2, true},
		{"listener unavailable", "unavailable", "tentativa=3/3", 3, false},
		{"invalid certificate", "certificate", "curl=60", 1, false},
		{"unreadable certificate", "ca-file", "curl=77", 1, false},
		{"anonymous data exposed", "public", "HTTP=200", 1, false},
		{"wrong route", "route", "HTTP=404", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counter := filepath.Join(t.TempDir(), "count")
			command := exec.Command(bash(t), "-c", `
set -euo pipefail
source ../../scripts/gateway-lib.sh
counter=$1; scenario=$2
if command -v cygpath >/dev/null 2>&1; then counter=$(cygpath -u "$counter"); fi
curl(){
 [[ $1 == -q ]] || return 99
 args=" $* "
 [[ $args == *" --cacert reviewed.crt "* && $args == *" --noproxy * "* && $args == *" --header Authorization: "* && $args == *" --header Cookie: "* && $args == *" --connect-to [fd12::10]:8443:127.0.0.1:8443 "* ]] || return 99
 [[ $args != *" --insecure "* && $args != *" -k "* ]] || return 99
 n=0; if [[ -f $counter ]]; then read -r n <"$counter"; fi
 n=$((n+1)); printf '%s\n' "$n" >"$counter"
 case "$scenario" in
 starting) if ((n<3)); then printf 000; return 7; fi ;;
 upstream) if ((n<2)); then printf 502; return; fi ;;
 unavailable) printf 000; return 7 ;;
 certificate) printf 'SECRET_TLS_ERROR' >&2; printf 000; return 60 ;;
 ca-file) printf 000; return 77 ;;
 public) printf 200; return ;;
 route) printf 404; return ;;
 esac
 printf 401
}
if check_panel_https fd12::10 8443 reviewed.crt 3 0; then result=0; else result=1; fi
printf 'calls='; cat "$counter"
printf 'diagnostic=%s\n' "$GATEWAY_CHECK_ERROR"
exit "$result"
`, "probe-test", counter, tc.scenario)
			out, e := command.CombinedOutput()
			if (e == nil) != tc.ok || !strings.Contains(string(out), "calls="+strconv.Itoa(tc.calls)) || !strings.Contains(string(out), tc.diagnostic) || strings.Contains(string(out), "SECRET_TLS_ERROR") {
				t.Fatal(string(out), e)
			}
		})
	}
}
