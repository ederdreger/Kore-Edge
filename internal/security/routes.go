package security

import (
	"net/http"
	"regexp"
	"strings"
)

var routeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func routeName(path string) string {
	switch path {
	case "/healthz":
		return "health"
	case "/readyz":
		return "ready"
	case "/api/v1/integration":
		return "integration"
	case "/api/v1/session":
		return "session"
	case "/api/v1/edge":
		return "edge"
	case "/metrics":
		return "metrics"
	case "/api/v1/catalog":
		return "catalog"
	case "/api/v1/streams":
		return "streams"
	case "/api/v1/diagnostics":
		return "diagnostics"
	case "/api/v1/wireguard":
		return "wireguard"
	case "/api/v1/wireguard/plan":
		return "wireguard_plan"
	case "/api/v1/multicast/interfaces":
		return "interfaces"
	case "/api/v1/multicast/plan":
		return "multicast_plan"
	case "/api/v1/monitoring":
		return "monitoring"
	case "/api/v1/provisioning":
		return "provision_status"
	case "/api/v1/provisioning/register":
		return "provision_register"
	case "/", "/ui/":
		return "ui"
	}
	if strings.HasPrefix(path, "/api/v1/streams/") && routeID.MatchString(strings.TrimPrefix(path, "/api/v1/streams/")) {
		return "stream_item"
	}
	if strings.HasPrefix(path, "/ui/assets/") || strings.HasPrefix(path, "/ui/licenses/") || strings.HasPrefix(path, "/player-assets/") {
		return "ui_asset"
	}
	if strings.HasPrefix(path, "/player/") {
		return "player"
	}
	if strings.HasPrefix(path, "/hls/") {
		return "hls"
	}
	return ""
}
func readMethod(method string) bool { return method == http.MethodGet || method == http.MethodHead }
func allowed(role, route, method string) bool {
	if route == "" {
		return false
	}
	if role == "metrics" {
		return readMethod(method) && (route == "health" || route == "ready" || route == "metrics")
	}
	if role == "enroller" {
		return readMethod(method) && (route == "health" || route == "ready" || route == "session" || route == "provision_status") || route == "provision_register" && method == http.MethodPost
	}
	if readMethod(method) {
		if route == "provision_register" || route == "stream_item" || strings.HasSuffix(route, "_plan") {
			return false
		}
		return role == "viewer" || role == "operator" || role == "admin"
	}
	if role == "admin" {
		return route == "catalog" && method == http.MethodPut || route == "provision_register" && method == http.MethodPost || operational(route, method)
	}
	return role == "operator" && operational(route, method)
}
func operational(route, method string) bool {
	return route == "streams" && method == http.MethodPost || route == "stream_item" && method == http.MethodDelete || (route == "wireguard_plan" || route == "multicast_plan") && method == http.MethodPost
}
