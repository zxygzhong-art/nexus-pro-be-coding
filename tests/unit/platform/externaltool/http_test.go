package externaltool_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nexus-pro-api/internal/platform/externaltool"
	"nexus-pro-api/internal/platform/mcpclient"
)

func TestCallGETEncodesScalarAndArrayQueryAndNormalizesJSON(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/search" {
			t.Fatalf("request = %s %s, want GET /api/search", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if got := r.URL.Query()["tag"]; !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
			t.Fatalf("tag query = %v", got)
		}
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("active") != "true" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"items":[1,2],"ok":true}`))
	}))
	defer httpServer.Close()

	result, err := externaltool.Call(context.Background(), externaltool.Request{
		Endpoint: httpServer.URL + "/api",
		Method:   http.MethodGet,
		Path:     "search",
		Headers:  http.Header{"Authorization": {"Bearer secret"}},
		Arguments: map[string]any{
			"tag":    []string{"alpha", "beta"},
			"page":   2,
			"active": true,
		},
		Timeout:           time.Second,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || result.ContentType != "application/json" || result.Text != "" {
		t.Fatalf("result metadata = %+v", result)
	}
	object, ok := result.JSON.(map[string]any)
	if !ok || object["ok"] != true {
		t.Fatalf("JSON result = %#v", result.JSON)
	}
	items, ok := object["items"].([]any)
	if !ok || len(items) != 2 || items[0].(json.Number).String() != "1" {
		t.Fatalf("JSON number normalization = %#v", object["items"])
	}
}

func TestCallDELETEUsesQueryAndPOSTUsesJSONBody(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantQuery  string
		wantBody   string
		resultText string
	}{
		{name: "delete", method: http.MethodDelete, wantQuery: "id=a&id=b", resultText: "deleted"},
		{name: "post", method: http.MethodPost, wantBody: `{"count":2,"nested":{"enabled":true}}`, resultText: "created"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != test.method || r.URL.Path != "/items" {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.URL.RawQuery != test.wantQuery {
					t.Fatalf("query = %q, want %q", r.URL.RawQuery, test.wantQuery)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != test.wantBody {
					t.Fatalf("body = %q, want %q", body, test.wantBody)
				}
				if test.wantBody != "" && r.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(test.resultText))
			}))
			defer httpServer.Close()

			arguments := map[string]any{"id": []string{"a", "b"}}
			if test.method == http.MethodPost {
				arguments = map[string]any{"count": 2, "nested": map[string]any{"enabled": true}}
			}
			result, err := externaltool.Call(context.Background(), externaltool.Request{
				Endpoint:          httpServer.URL,
				Method:            test.method,
				Path:              "/items",
				Arguments:         arguments,
				Timeout:           time.Second,
				EndpointValidator: mcpclient.AllowPrivateEndpoints,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Text != test.resultText || result.JSON != nil {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestCallRejectsCrossOriginCredentialsFragmentsAndUnsupportedMethods(t *testing.T) {
	var targetCalled atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled.Store(true)
	}))
	defer target.Close()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()

	for _, path := range []string{
		target.URL + "/steal",
		"http://user:password@" + strings.TrimPrefix(endpoint.URL, "http://") + "/private",
		"/items#secret",
	} {
		_, err := externaltool.Call(context.Background(), externaltool.Request{
			Endpoint:          endpoint.URL,
			Method:            http.MethodGet,
			Path:              path,
			EndpointValidator: mcpclient.AllowPrivateEndpoints,
			Timeout:           time.Second,
		})
		if !errors.Is(err, externaltool.ErrInvalidPath) {
			t.Fatalf("path %q error = %v, want ErrInvalidPath", path, err)
		}
	}
	if targetCalled.Load() {
		t.Fatal("cross-origin path reached target server")
	}

	_, err := externaltool.Call(context.Background(), externaltool.Request{
		Endpoint:          endpoint.URL,
		Method:            http.MethodTrace,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
	})
	if !errors.Is(err, externaltool.ErrUnsupportedMethod) {
		t.Fatalf("TRACE error = %v, want ErrUnsupportedMethod", err)
	}
}

func TestCallInheritsDefaultSSRFPolicy(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer httpServer.Close()

	_, err := externaltool.Call(context.Background(), externaltool.Request{
		Endpoint: httpServer.URL,
		Method:   http.MethodGet,
	})
	if !errors.Is(err, mcpclient.ErrUnsafeEndpoint) {
		t.Fatalf("Call() error = %v, want inherited ErrUnsafeEndpoint", err)
	}
}

func TestCallReturnsClassifiedNon2xxWithoutExposingBody(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"secret":"must-not-leak"}`))
	}))
	defer httpServer.Close()

	result, err := externaltool.Call(context.Background(), externaltool.Request{
		Endpoint:          httpServer.URL,
		Method:            http.MethodGet,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
		Timeout:           time.Second,
	})
	var statusErr *externaltool.StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusTeapot {
		t.Fatalf("Call() result=%+v error=%v, want StatusError(418)", result, err)
	}
	if result.StatusCode != http.StatusTeapot || result.JSON != nil || result.Text != "" {
		t.Fatalf("non-2xx result exposed body: %+v", result)
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("non-2xx error exposed response body: %v", err)
	}
}

func TestCallEnforcesResponseLimitForChunkedBody(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(strings.Repeat("x", 128)))
	}))
	defer httpServer.Close()

	_, err := externaltool.Call(context.Background(), externaltool.Request{
		Endpoint:          httpServer.URL,
		Method:            http.MethodGet,
		EndpointValidator: mcpclient.AllowPrivateEndpoints,
		Timeout:           time.Second,
		MaxResponseBytes:  32,
	})
	if !errors.Is(err, externaltool.ErrResponseTooLarge) {
		t.Fatalf("Call() error = %v, want ErrResponseTooLarge", err)
	}
}
