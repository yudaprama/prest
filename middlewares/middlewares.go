package middlewares

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/urfave/negroni/v3"
)

var (
	jsonErrFormat = `{"error": "%s"}`
)

// HandlerSet add content type header
func HandlerSet() negroni.Handler {
	return negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		format := r.URL.Query().Get("_renderer")
		recorder := httptest.NewRecorder()
		negroniResp := negroni.NewResponseWriter(recorder)
		next(negroniResp, r)
		renderFormat(w, recorder, format)
	})
}

// SetTimeoutToContext adds the configured timeout in seconds to the request context
//
// By default it is 60 seconds, can be modified to a different value
func SetTimeoutToContext() negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		next(rw, r.WithContext(context.WithValue(r.Context(), pctx.HTTPTimeoutKey, config.PrestConf.HTTPTimeout))) // nolint
	})
}

// AuthMiddleware handle request token validation (disabled)
func AuthMiddleware(_ string) negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		next(rw, r)
	})
}

// AccessControl is a middleware to handle permissions on tables in pREST (disabled)
func AccessControl() negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, rq *http.Request, next http.HandlerFunc) {
		next(rw, rq)
	})
}

// JwtMiddleware check if actual request have JWT (disabled)
func JwtMiddleware(_ string, _ string, _ string) negroni.Handler {
	return negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		next(w, r)
	})
}

// Cors middleware
//
// Deprecated: we'll use github.com/rs/cors instead
func Cors(origin []string, headers []string) negroni.Handler {
	return negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		w.Header().Set(headerAllowOrigin, strings.Join(origin, ","))
		w.Header().Set(headerAllowCredentials, strconv.FormatBool(true))
		if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set(headerAllowMethods, strings.Join(defaultAllowMethods, ","))
			w.Header().Set(headerAllowHeaders, strings.Join(headers, ","))
			if allowed := checkCors(r, origin); !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	})
}

func ExposureMiddleware() negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, rq *http.Request, next http.HandlerFunc) {
		url := rq.URL.Path
		exposeConf := config.PrestConf.ExposeConf

		if strings.HasPrefix(url, "/databases") && !exposeConf.DatabaseListing {
			http.Error(rw, fmt.Sprintf(jsonErrFormat, "unauthorized listing"), http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(url, "/tables") && !exposeConf.TableListing {
			http.Error(rw, fmt.Sprintf(jsonErrFormat, "unauthorized listing"), http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(url, "/schemas") && !exposeConf.SchemaListing {
			http.Error(rw, fmt.Sprintf(jsonErrFormat, "unauthorized listing"), http.StatusUnauthorized)
			return
		}

		next(rw, rq)
	})
}
