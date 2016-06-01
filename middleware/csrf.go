package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lessgo/lessgo"
)

type (
	// CSRFConfig defines the config for CSRF middleware.
	CSRFConfig struct {
		// Key to create CSRF token.
		Secret []byte `json:"secret"`

		// TokenLookup is a string in the form of "<source>:<key>" that is used
		// to extract token from the request.
		// Optional. Default value "header:X-CSRF-Token".
		// Possible values:
		// - "header:<name>"
		// - "form:<name>"
		// - "header:<name>"
		TokenLookup string `json:"token_lookup"`

		// Context key to store generated CSRF token into context.
		// Optional. Default value "csrf".
		ContextKey string `json:"context_key"`

		// Name of the CSRF cookie. This cookie will store CSRF token.
		// Optional. Default value "csrf".
		CookieName string `json:"cookie_name"`

		// Domain of the CSRF cookie.
		// Optional. Default value none.
		CookieDomain string `json:"cookie_domain"`

		// Path of the CSRF cookie.
		// Optional. Default value none.
		CookiePath string `json:"cookie_path"`

		// Expiration time of the CSRF cookie.
		// Optional. Default value 24H.
		CookieExpires time.Time `json:"cookie_expires"`

		// Indicates if CSRF cookie is secure.
		CookieSecure bool `json:"cookie_secure"`
		// Optional. Default value false.

		// Indicates if CSRF cookie is HTTP only.
		// Optional. Default value false.
		CookieHTTPOnly bool `json:"cookie_http_only"`
	}

	// csrfTokenExtractor defines a function that takes `lessgo.Context` and returns
	// either a token or an error.
	csrfTokenExtractor func(*lessgo.Context) (string, error)
)

var (
	// DefaultCSRFConfig is the default CSRF middleware config.
	DefaultCSRFConfig = CSRFConfig{
		TokenLookup:   "header:" + lessgo.HeaderXCSRFToken,
		ContextKey:    "csrf",
		CookieName:    "csrf",
		CookieExpires: time.Now().Add(24 * time.Hour),
	}
)

// CSRF returns a Cross-Site Request Forgery (CSRF) middleware.
// See: https://en.wikipedia.org/wiki/Cross-site_request_forgery
var CSRFWithConfig = lessgo.ApiMiddleware{
	Name:   "CSRFWithConfig",
	Desc:   `Cross-Site Request Forgery (CSRF)`,
	Config: DefaultCSRFConfig,
	Middleware: func(confObject interface{}) lessgo.MiddlewareFunc {
		config := confObject.(CSRFConfig)
		// Defaults
		if config.Secret == nil {
			panic("csrf secret must be provided")
		}
		if config.TokenLookup == "" {
			config.TokenLookup = DefaultCSRFConfig.TokenLookup
		}
		if config.ContextKey == "" {
			config.ContextKey = DefaultCSRFConfig.ContextKey
		}
		if config.CookieName == "" {
			config.CookieName = DefaultCSRFConfig.CookieName
		}
		if config.CookieExpires.IsZero() {
			config.CookieExpires = DefaultCSRFConfig.CookieExpires
		}

		// Initialize
		parts := strings.Split(config.TokenLookup, ":")
		extractor := csrfTokenFromHeader(parts[1])
		switch parts[0] {
		case "form":
			extractor = csrfTokenFromForm(parts[1])
		case "query":
			extractor = csrfTokenFromQuery(parts[1])
		}

		return func(next lessgo.HandlerFunc) lessgo.HandlerFunc {
			return func(c *lessgo.Context) error {
				req := c.Request()

				// Set CSRF token
				salt, err := generateSalt(8)
				if err != nil {
					return err
				}
				token := generateCSRFToken(config.Secret, salt)
				c.Set(config.ContextKey, token)
				cookie := &http.Cookie{
					Name:     config.CookieName,
					Value:    token,
					Expires:  config.CookieExpires,
					Secure:   config.CookieSecure,
					HttpOnly: config.CookieHTTPOnly,
				}
				if config.CookiePath != "" {
					cookie.Path = config.CookiePath
				}
				if config.CookieDomain != "" {
					cookie.Domain = config.CookieDomain
				}
				c.SetCookie(cookie)

				switch req.Method {
				case lessgo.GET, lessgo.HEAD, lessgo.OPTIONS, lessgo.TRACE:
				default:
					token, err := extractor(c)
					if err != nil {
						return err
					}
					ok, err := validateCSRFToken(token, config.Secret)
					if err != nil {
						return err
					}
					if !ok {
						return lessgo.NewHTTPError(http.StatusForbidden, "invalid csrf token")
					}
				}
				return next(c)
			}
		}
	},
}

// csrfTokenFromForm returns a `csrfTokenExtractor` that extracts token from the
// provided request header.
func csrfTokenFromHeader(header string) csrfTokenExtractor {
	return func(c *lessgo.Context) (string, error) {
		return c.Request().Header.Get(header), nil
	}
}

// csrfTokenFromForm returns a `csrfTokenExtractor` that extracts token from the
// provided form parameter.
func csrfTokenFromForm(param string) csrfTokenExtractor {
	return func(c *lessgo.Context) (string, error) {
		token := c.FormValue(param)
		if token == "" {
			return "", errors.New("empty csrf token in form param")
		}
		return token, nil
	}
}

// csrfTokenFromQuery returns a `csrfTokenExtractor` that extracts token from the
// provided query parameter.
func csrfTokenFromQuery(param string) csrfTokenExtractor {
	return func(c *lessgo.Context) (string, error) {
		token := c.QueryParam(param)
		if token == "" {
			return "", errors.New("empty csrf token in query param")
		}
		return token, nil
	}
}

func generateCSRFToken(secret, salt []byte) string {
	h := hmac.New(sha1.New, secret)
	h.Write(salt)
	return fmt.Sprintf("%s:%s", hex.EncodeToString(h.Sum(nil)), hex.EncodeToString(salt))
}

func validateCSRFToken(token string, secret []byte) (bool, error) {
	sep := strings.Index(token, ":")
	if sep < 0 {
		return false, nil
	}
	salt, err := hex.DecodeString(token[sep+1:])
	if err != nil {
		return false, err
	}
	return token == generateCSRFToken(secret, salt), nil
}

func generateSalt(len uint8) (salt []byte, err error) {
	salt = make([]byte, len)
	_, err = rand.Read(salt)
	return
}
