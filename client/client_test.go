package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetryableHTTPClientRetriesOnlySafeMethodsAfterTransportError(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		wantCalls int32
		wantErr   bool
	}{
		{
			name:      "retries GET",
			method:    http.MethodGet,
			wantCalls: 2,
		},
		{
			name:      "does not retry POST",
			method:    http.MethodPost,
			wantCalls: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			rc := newRetryableHTTPClient()
			rc.RetryWaitMin = 0
			rc.RetryWaitMax = 0
			rc.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tt.method, req.Method)
				if calls.Add(1) == 1 {
					return nil, errors.New("temporary failure")
				}
				return responseWithStatus(req, http.StatusOK), nil
			})

			req, err := http.NewRequest(tt.method, "https://example.test/resource", strings.NewReader("payload"))
			require.NoError(t, err)

			resp, err := (&httpDoerAdapter{client: rc.StandardClient()}).Do(context.Background(), req)
			if resp != nil {
				defer func() { _ = resp.Body.Close() }()
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
			}
			require.Equal(t, tt.wantCalls, calls.Load())
		})
	}
}

func TestRetryableHTTPClientRetriesOnlySafeMethodsAfterServerError(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantCalls  int32
		wantStatus int
	}{
		{
			name:       "retries GET",
			method:     http.MethodGet,
			wantCalls:  2,
			wantStatus: http.StatusOK,
		},
		{
			name:       "does not retry POST",
			method:     http.MethodPost,
			wantCalls:  1,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			rc := newRetryableHTTPClient()
			rc.RetryWaitMin = 0
			rc.RetryWaitMax = 0
			rc.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, tt.method, req.Method)
				if calls.Add(1) == 1 {
					return responseWithStatus(req, http.StatusInternalServerError), nil
				}
				return responseWithStatus(req, http.StatusOK), nil
			})

			req, err := http.NewRequest(tt.method, "https://example.test/resource", strings.NewReader("payload"))
			require.NoError(t, err)

			resp, err := (&httpDoerAdapter{client: rc.StandardClient()}).Do(context.Background(), req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tt.wantStatus, resp.StatusCode)
			require.Equal(t, tt.wantCalls, calls.Load())
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func responseWithStatus(req *http.Request, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(http.StatusText(statusCode))),
		Request:    req,
	}
}
