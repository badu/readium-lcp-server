package problem

// rfc 7807
// problem.Type should be an URI
// for example http://readium.org/readium/[lcpserver|lsdserver]/<code>
// for standard http error messages use "about:blank" status in json equals http status
import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/technoweenie/grohl"

	"github.com/readium/readium-lcp-server/localization"
)

const (
	ContentType_PROBLEM_JSON = "application/problem+json"
)

type Problem struct {
	Type string `json:"type"`
	//optionnal
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status,omitempty"` //if present = http response code
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	//Additional members
}

const ERROR_BASE_URL = "http://readium.org/license-status-document/error/"
const SERVER_INTERNAL_ERROR = ERROR_BASE_URL + "server"
const REGISTRATION_BAD_REQUEST = ERROR_BASE_URL + "registration"
const RETURN_BAD_REQUEST = ERROR_BASE_URL + "return"
const RENEW_BAD_REQUEST = ERROR_BASE_URL + "renew"
const RENEW_REJECT = ERROR_BASE_URL + "renew/date"

func Error(w http.ResponseWriter, r *http.Request, problem Problem, status int) {
	acceptLanguages := r.Header.Get("Accept-Language")
	w.Header().Set("Content-Type", ContentType_PROBLEM_JSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	problem.Status = status

	if problem.Type == "about:blank" { // lookup Title  statusText should match http status
		localization.LocalizeMessage(acceptLanguages, &problem.Title, http.StatusText(status))
	} else {
		localization.LocalizeMessage(acceptLanguages, &problem.Title, problem.Title)
		localization.LocalizeMessage(acceptLanguages, &problem.Detail, problem.Detail)
	}
	jsonError, e := json.Marshal(problem)
	if e != nil {
		http.Error(w, "{}", problem.Status)
	}
	fmt.Fprintln(w, string(jsonError))
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	grohl.Log(grohl.Data{"method": r.Method, "path": r.URL.Path, "status": "404"})
	Error(w, r, Problem{Type: "about:blank"}, http.StatusNotFound)
}
