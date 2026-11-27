package rest

import (
	"crypto/subtle"
	"net/http"
)

func (apiHandler *handler) requireBasicAuthentication(
	nextHandler http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
		responseWriter http.ResponseWriter,
		request *http.Request,
	) {
		providedUsername, providedPassword, credentialsProvided :=
			request.BasicAuth()

		usernameMatches := subtle.ConstantTimeCompare(
			[]byte(providedUsername),
			[]byte(apiHandler.username),
		)
		passwordMatches := subtle.ConstantTimeCompare(
			[]byte(providedPassword),
			[]byte(apiHandler.password),
		)

		if !credentialsProvided || usernameMatches != 1 || passwordMatches != 1 {
			responseWriter.Header().Set(
				"WWW-Authenticate",
				`Basic realm="device-monitor", charset="UTF-8"`,
			)
			apiHandler.writeAPIError(
				responseWriter,
				http.StatusUnauthorized,
				"authentication_required",
				"valid HTTP Basic credentials are required",
			)
			return
		}

		nextHandler.ServeHTTP(responseWriter, request)
	})
}
