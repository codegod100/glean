package server

import "net/http"

const (
	HXRequest  = "HX-Request"
	HXRedirect = "HX-Redirect"
	HXRefresh  = "HX-Refresh"
)

func isHXRequest(r *http.Request) bool {
	return r.Header.Get(HXRequest) == "true"
}
