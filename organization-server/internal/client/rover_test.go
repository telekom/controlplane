// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
)

var testToken = accesstoken.NewStaticAccessToken("test-token")

func TestGetResources_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/resources" {
			t.Errorf("expected /resources, got %s", r.URL.Path)
		}
		prefix := r.URL.Query().Get("group")
		if prefix != "eni" {
			t.Errorf("expected group eni, got %s", prefix)
		}
		team := r.URL.Query().Get("team")
		if team != "hyperion" {
			t.Errorf("expected team hyperion, got %s", team)
		}
		acceptHeader := r.Header.Get("Accept")
		if acceptHeader != "application/json" {
			t.Errorf("expected Accept: application/json, got %s", acceptHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		resp := ResourceListResponse{
			Items: []ResourceRef{
				{Name: "my-api", Kind: "ApiExposure", APIVersion: "v1", Path: "/apis/my-api"},
				{Name: "my-sub", Kind: "ApiSubscription", APIVersion: "v1", Path: "/subs/my-sub"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	result, err := client.GetResources(context.Background(), "controlplane", "eni", "hyperion")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Name != "my-api" {
		t.Errorf("expected my-api, got %s", result.Items[0].Name)
	}
	if result.Items[0].Kind != "ApiExposure" {
		t.Errorf("expected ApiExposure, got %s", result.Items[0].Kind)
	}
	if result.Items[1].Name != "my-sub" {
		t.Errorf("expected my-sub, got %s", result.Items[1].Name)
	}
}

func TestGetResources_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	result, err := client.GetResources(context.Background(), "test", "eni", "team1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

func TestGetResources_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetResources_RejectsRedirectBeforeFollowing(t *testing.T) {
	redirectedRequests := 0
	redirectedAuthorization := ""
	redirectedServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		redirectedRequests++
		redirectedAuthorization = r.Header.Get("Authorization")
	}))
	defer redirectedServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectedServer.URL, http.StatusFound)
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected redirect error")
	}
	if redirectedRequests != 0 {
		t.Errorf("expected no redirected requests, got %d", redirectedRequests)
	}
	if redirectedAuthorization != "" {
		t.Errorf("expected no redirected token, got Authorization %q", redirectedAuthorization)
	}
}

func TestGetResources_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetResources_ConnectionRefused(t *testing.T) {
	client := NewRoverClient("http://127.0.0.1:1", testToken, "")
	_, err := client.GetResources(context.Background(), "test", "eni", "team1")

	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestGetResources_QueryParamConstruction(t *testing.T) {
	var capturedPath, capturedGroup, capturedTeam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedGroup = r.URL.Query().Get("group")
		capturedTeam = r.URL.Query().Get("team")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL+"/rover/", testToken, "")
	_, _ = client.GetResources(context.Background(), "prod", "my hub/&", "my team/?")

	if capturedPath != "/rover/resources" {
		t.Errorf("path: want /rover/resources, got %s", capturedPath)
	}
	if capturedGroup != "my hub/&" {
		t.Errorf("group: want my hub/&, got %s", capturedGroup)
	}
	if capturedTeam != "my team/?" {
		t.Errorf("team: want my team/?, got %s", capturedTeam)
	}
}

func TestGetResources_ConsumesEveryPageWithHeaders(t *testing.T) {
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("request %d Authorization: got %q", requests, got)
		}
		if got := r.Header.Get("X-Environment"); got != "controlplane" {
			t.Errorf("request %d X-Environment: got %q", requests, got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("request %d Accept: got %q", requests, got)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = w.Write([]byte(`{"items":[{"name":"first"}],"_links":{"next":"` + server.URL + `/resources?cursor=opaque%2Fcursor"}}`))
			return
		}
		if got := r.URL.Query().Get("cursor"); got != "opaque/cursor" {
			t.Errorf("cursor: want opaque/cursor, got %q", got)
		}
		_, _ = w.Write([]byte(`{"items":[{"name":"second"}],"_links":{"next":""}}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	result, err := client.GetResources(context.Background(), "controlplane", "eni", "hyperion")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Name != "first" || result.Items[1].Name != "second" {
		t.Fatalf("unexpected items: %#v", result.Items)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
}

func TestGetResources_ResolvesRelativeNextLink(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			_, _ = w.Write([]byte(`{"items":[{"name":"first"}],"_links":{"next":"?cursor=next"}}`))
			return
		}
		if got := r.URL.Query().Get("cursor"); got != "next" {
			t.Errorf("cursor: want next, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization: got %q", got)
		}
		_, _ = w.Write([]byte(`{"items":[{"name":"second"}]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	result, err := client.GetResources(context.Background(), "env", "group", "team")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
}

func TestGetResources_ReturnsErrorForLaterPageFailure(t *testing.T) {
	tests := map[string]func(http.ResponseWriter){
		"server error": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		},
		"invalid JSON": func(w http.ResponseWriter) {
			_, _ = w.Write([]byte("not json"))
		},
	}

	for name, writeFailure := range tests {
		t.Run(name, func(t *testing.T) {
			requests := 0
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if requests == 1 {
					_, _ = w.Write([]byte(`{"items":[{"name":"first"}],"_links":{"next":"` + server.URL + `/resources?cursor=next"}}`))
					return
				}
				writeFailure(w)
			}))
			defer server.Close()

			client := NewRoverClient(server.URL, testToken, "")
			result, err := client.GetResources(context.Background(), "env", "group", "team")
			if err == nil {
				t.Fatal("expected later-page error")
			}
			if result != nil {
				t.Fatalf("expected no partial result, got %#v", result)
			}
		})
	}
}

func TestGetResources_RejectsUnsafeNextOrigin(t *testing.T) {
	unsafeRequests := 0
	unsafeServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		unsafeRequests++
	}))
	defer unsafeServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"name":"first"}],"_links":{"next":"` + unsafeServer.URL + `/resources?cursor=next"}}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	result, err := client.GetResources(context.Background(), "env", "group", "team")
	if err == nil {
		t.Fatal("expected unsafe next origin error")
	}
	if result != nil {
		t.Fatalf("expected no partial result, got %#v", result)
	}
	if unsafeRequests != 0 {
		t.Fatalf("expected no request to unsafe origin, got %d", unsafeRequests)
	}
}

func TestGetResources_RejectsURLUserinfoBeforeRequest(t *testing.T) {
	t.Run("configured base", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requests++
		}))
		defer server.Close()

		client := NewRoverClient("http://user@"+server.Listener.Addr().String(), testToken, "")
		if _, err := client.GetResources(context.Background(), "env", "group", "team"); err == nil {
			t.Fatal("expected configured base userinfo error")
		}
		if requests != 0 {
			t.Fatalf("expected no request, got %d", requests)
		}
	})

	t.Run("continuation", func(t *testing.T) {
		requests := 0
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests++
			_, _ = w.Write([]byte(`{"items":[],"_links":{"next":"http://user@` + server.Listener.Addr().String() + `/resources?cursor=next"}}`))
		}))
		defer server.Close()

		client := NewRoverClient(server.URL, testToken, "")
		if _, err := client.GetResources(context.Background(), "env", "group", "team"); err == nil {
			t.Fatal("expected continuation userinfo error")
		}
		if requests != 1 {
			t.Fatalf("expected only initial request, got %d", requests)
		}
	})
}

func TestGetResources_RejectsCursorLoop(t *testing.T) {
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"items":[],"_links":{"next":"` + server.URL + `/resources?cursor=repeat"}}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	if _, err := client.GetResources(context.Background(), "env", "group", "team"); err == nil {
		t.Fatal("expected cursor loop error")
	}
	if requests != 2 {
		t.Fatalf("expected loop detection after 2 requests, got %d", requests)
	}
}

func TestGetResources_StopsBeforePage1001(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		cursor, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
		_, _ = w.Write([]byte(`{"items":[],"_links":{"next":"?cursor=` + strconv.Itoa(cursor+1) + `"}}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, testToken, "")
	result, err := client.GetResources(context.Background(), "env", "group", "team")
	if err == nil || err.Error() != "following rover-server pagination: page limit of 1000 exceeded" {
		t.Fatalf("expected page limit error, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected no partial result, got %#v", result)
	}
	if requests != 1000 {
		t.Fatalf("expected exactly 1000 requests, got %d", requests)
	}
}

func TestGetResources_RejectsOversizedResponse(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(strings.Repeat("x", 4*1024*1024+1)))
			}))
			defer server.Close()

			client := NewRoverClient(server.URL, testToken, "")
			result, err := client.GetResources(context.Background(), "env", "group", "team")
			if err == nil || err.Error() != "rover-server response exceeds 4194304 bytes" {
				t.Fatalf("expected response size error, got %v", err)
			}
			if result != nil {
				t.Fatalf("expected no partial result, got %#v", result)
			}
		})
	}
}

// The internal listener authenticates with the projected SA token and derives
// its admin business context from X-Environment; both must be on the wire.
func TestGetResources_SendsProjectedTokenAndEnvironment(t *testing.T) {
	tokenFile := writeTempToken(t)

	var capturedAuth, capturedEnv string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedEnv = r.Header.Get("X-Environment")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := NewRoverClient(server.URL, accesstoken.NewAccessToken(tokenFile), "")
	if _, err := client.GetResources(context.Background(), "env", "g", "t"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAuth != "Bearer "+readFile(t, tokenFile) {
		t.Errorf("Authorization: got %q", capturedAuth)
	}
	if capturedEnv != "env" {
		t.Errorf("X-Environment: want env, got %q", capturedEnv)
	}
}

// writeTempToken writes an unsigned JWT with a future exp, matching what the
// kubelet projects (the file token reader parses exp to decide on re-reads).
func writeTempToken(t *testing.T) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	tok := header + "." + base64.RawURLEncoding.EncodeToString(claims) + "."

	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		t.Fatalf("writing token file: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}
