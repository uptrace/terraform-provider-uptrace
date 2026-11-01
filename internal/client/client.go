package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/uptrace/oapi-codegen-dd/v3/pkg/runtime"

	"github.com/uptrace/terraform/internal/generated"
)

const (
	defaultRequestTimeout = 30 * time.Second
	maxRetries            = 2
	retryWaitMax          = 5 * time.Second
)

type retryMethodContextKey struct{}

// Client wraps the generated OpenAPI client with retry and auth configuration.
type Client struct {
	// API is the oapi-codegen generated client for all spec-defined endpoints.
	API *generated.Client
}

// New creates a client with retryable HTTP transport and bearer token auth.
func New(endpoint, token, providerVersion string) (*Client, error) {
	rc := newRetryableHTTPClient()

	setHeaders := func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "terraform-provider-uptrace/"+providerVersion)
		return nil
	}

	apiClient, err := runtime.NewAPIClient(
		endpoint,
		runtime.WithHTTPClient(&httpDoerAdapter{client: rc.StandardClient()}),
		runtime.WithRequestEditorFn(setHeaders),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		API: generated.NewClient(apiClient),
	}, nil
}

func newRetryableHTTPClient() *retryablehttp.Client {
	rc := retryablehttp.NewClient()
	rc.HTTPClient = &http.Client{Timeout: defaultRequestTimeout}
	rc.RetryMax = maxRetries
	rc.RetryWaitMax = retryWaitMax
	rc.CheckRetry = retryPolicyForSafeMethods
	rc.Logger = nil
	return rc
}

func retryPolicyForSafeMethods(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if !isSafeHTTPMethod(retryRequestMethod(ctx, resp)) {
		return false, nil
	}
	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

func retryRequestMethod(ctx context.Context, resp *http.Response) string {
	if method, ok := ctx.Value(retryMethodContextKey{}).(string); ok {
		return method
	}
	if resp != nil && resp.Request != nil {
		return resp.Request.Method
	}
	return ""
}

func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// IsNotFound reports whether err is an API 404 error.
func IsNotFound(err error) bool {
	return hasStatusCode(err, http.StatusNotFound)
}

// IsForbidden reports whether err is an API 403 error.
func IsForbidden(err error) bool {
	return hasStatusCode(err, http.StatusForbidden)
}

// APIErrorMessage returns the server-side message from an API error, or
// err.Error() otherwise. Needed because the generated ProblemDetails.Error()
// is a stub that returns "unmapped client error" for every API response.
//
// The server answers every error with an RFC 9457 problem: detail holds the
// whole message, and errors names each invalid input of a validation problem.
func APIErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	problem, ok := apiProblem(err)
	if !ok {
		return err.Error()
	}
	msg := problem.Detail
	if msg == "" {
		msg = problem.Title
	}
	if msg == "" {
		return err.Error()
	}
	for _, fe := range problem.Errors {
		if fe.Field == "" || strings.Contains(msg, fe.Field) {
			continue
		}
		msg += "; " + fe.Field + ": " + fe.Detail
	}
	return msg
}

// apiProblem unwraps the ClientAPIError envelope to the decoded problem. The
// runtime wraps the decoded error as a value (not pointer), so we match on the
// value type.
func apiProblem(err error) (generated.ProblemDetails, bool) {
	clientErr, ok := errors.AsType[*runtime.ClientAPIError](err)
	if !ok {
		return generated.ProblemDetails{}, false
	}
	return errors.AsType[generated.ProblemDetails](clientErr.Unwrap())
}

func hasStatusCode(err error, code int) bool {
	clientErr, ok := errors.AsType[*runtime.ClientAPIError](err)
	if !ok {
		return false
	}
	return clientErr.StatusCode() == code
}

// httpDoerAdapter adapts a standard *http.Client to the runtime.HttpRequestDoer
// interface which expects Do(context.Context, *http.Request).
type httpDoerAdapter struct {
	client *http.Client
}

func (a *httpDoerAdapter) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	ctx = context.WithValue(ctx, retryMethodContextKey{}, req.Method)
	return a.client.Do(req.WithContext(ctx))
}
