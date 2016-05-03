package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/dgrijalva/jwt-go"
	"github.com/lessgo/lessgo"
)

type (
	// BasicAuthConfig defines the config for HTTP basic auth middleware.
	BasicAuthConfig struct {
		// AuthFunc is the function to validate basic auth credentials.
		AuthFunc BasicAuthFunc
	}

	// BasicAuthFunc defines a function to validate basic auth credentials.
	BasicAuthFunc func(string, string) bool

	// JWTAuthConfig defines the config for JWT auth middleware.
	JWTAuthConfig struct {
		// SigningKey is the key to validate token.
		// Required.
		SigningKey []byte

		// SigningMethod is used to check token signing method.
		// Optional, with default value as `HS256`.
		SigningMethod string

		// ContextKey is the key to be used for storing user information from the
		// token into context.
		// Optional, with default value as `user`.
		ContextKey string

		// Extractor is a function that extracts token from the request
		// Optional, with default values as `JWTFromHeader`.
		Extractor JWTExtractor
	}

	// JWTExtractor defines a function that takes `lessgo.Context` and returns either
	// a token or an error.
	JWTExtractor func(lessgo.Context) (string, error)
)

const (
	basic  = "Basic"
	bearer = "Bearer"
)

// Algorithims
const (
	AlgorithmHS256 = "HS256"
)

var (
	// DefaultBasicAuthConfig is the default basic auth middleware config.
	DefaultBasicAuthConfig = BasicAuthConfig{}

	// DefaultJWTAuthConfig is the default JWT auth middleware config.
	DefaultJWTAuthConfig = JWTAuthConfig{
		SigningMethod: AlgorithmHS256,
		ContextKey:    "user",
		Extractor:     JWTFromHeader,
	}
)

// BasicAuth returns an HTTP basic auth middleware from config.
// See `BasicAuth()`.
func BasicAuth(configJSON string) lessgo.MiddlewareFunc {
	config := BasicAuthConfig{}
	json.Unmarshal([]byte(configJSON), &config)
	return func(next lessgo.HandlerFunc) lessgo.HandlerFunc {
		return func(c lessgo.Context) error {
			auth := c.Request().Header().Get(lessgo.HeaderAuthorization)
			l := len(basic)

			if len(auth) > l+1 && auth[:l] == basic {
				b, err := base64.StdEncoding.DecodeString(auth[l+1:])
				if err != nil {
					return err
				}
				cred := string(b)
				for i := 0; i < len(cred); i++ {
					if cred[i] == ':' {
						// Verify credentials
						if config.AuthFunc(cred[:i], cred[i+1:]) {
							return next(c)
						}
						c.Response().Header().Set(lessgo.HeaderWWWAuthenticate, basic+" realm=Restricted")
						return lessgo.ErrUnauthorized
					}
				}
			}
			return lessgo.NewHTTPError(http.StatusBadRequest, "invalid basic-auth authorization header="+auth)
		}
	}
}

// JWTAuth returns a JSON Web Token (JWT) auth middleware.
//
// For valid token, it sets the user in context and calls next handler.
// For invalid token, it sends "401 - Unauthorized" response.
// For empty or invalid `Authorization` header, it sends "400 - Bad Request".
//
// See https://jwt.io/introduction
func JWTAuth(configJSON string) lessgo.MiddlewareFunc {
	config := JWTAuthConfig{}
	json.Unmarshal([]byte(configJSON), &config)
	// Defaults
	if config.SigningKey == nil {
		panic("jwt middleware requires signing key")
	}
	if config.SigningMethod == "" {
		config.SigningMethod = DefaultJWTAuthConfig.SigningMethod
	}
	if config.ContextKey == "" {
		config.ContextKey = DefaultJWTAuthConfig.ContextKey
	}
	if config.Extractor == nil {
		config.Extractor = DefaultJWTAuthConfig.Extractor
	}

	return func(next lessgo.HandlerFunc) lessgo.HandlerFunc {
		return func(c lessgo.Context) error {
			auth, err := config.Extractor(c)
			if err != nil {
				return lessgo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			token, err := jwt.Parse(auth, func(t *jwt.Token) (interface{}, error) {
				// Check the signing method
				if t.Method.Alg() != config.SigningMethod {
					return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
				}
				return config.SigningKey, nil

			})
			if err == nil && token.Valid {
				// Store user information from token into context.
				c.Set(config.ContextKey, token)
				return next(c)
			}
			return lessgo.ErrUnauthorized
		}
	}
}

// JWTFromHeader is a `JWTExtractor` that extracts token from the `Authorization` request
// header.
func JWTFromHeader(c lessgo.Context) (string, error) {
	auth := c.Request().Header().Get(lessgo.HeaderAuthorization)
	l := len(bearer)
	if len(auth) > l+1 && auth[:l] == bearer {
		return auth[l+1:], nil
	}
	return "", lessgo.NewHTTPError(http.StatusBadRequest, "invalid jwt authorization header="+auth)
}

// JWTFromQuery returns a `JWTExtractor` that extracts token from the provided query
// parameter.
func JWTFromQuery(param string) JWTExtractor {
	return func(c lessgo.Context) (string, error) {
		return c.QueryParam(param), nil
	}
}
