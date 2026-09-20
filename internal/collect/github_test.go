package collect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "secret-token-value-xyz"

func TestGetHeadersPaginationAndErrors(t *testing.T) {
	var saw []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = append(saw, r.Method+" "+r.URL.RequestURI())
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("authorization %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("accept %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != APIVersion {
			t.Errorf("api version %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != UserAgent {
			t.Errorf("user-agent %q", got)
		}
		if strings.Contains(r.URL.String(), testToken) {
			t.Fatal("token appeared in URL")
		}
		switch r.URL.RequestURI() {
		case "/repos/o/r/pulls/1/files?per_page=100":
			w.Header().Set("Link", `<`+serverURL(r, "/repos/o/r/pulls/1/files?per_page=100&page=2")+`>; rel="next"`)
			io.WriteString(w, `[{"filename":"a.md"}]`)
		case "/repos/o/r/pulls/1/files?per_page=100&page=2":
			io.WriteString(w, `[{"filename":"b.md"}]`)
		case "/fail":
			http.Error(w, "nope "+testToken, http.StatusNotFound)
		case "/bad-json":
			io.WriteString(w, "{")
		case "/not-list":
			io.WriteString(w, `{"items":"nope"}`)
		case "/check-runs":
			io.WriteString(w, `{"check_runs":[{"name":"quality"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewGitHubClient(testToken, server.URL, server.Client())
	ctx := context.Background()

	items, err := client.GetAll(ctx, "/repos/o/r/pulls/1/files?per_page=100", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items %#v", items)
	}
	if asMap(items[0])["filename"] != "a.md" || asMap(items[1])["filename"] != "b.md" {
		t.Fatalf("order %#v", items)
	}

	runs, err := client.GetAll(ctx, "/check-runs", "check_runs", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || asMap(runs[0])["name"] != "quality" {
		t.Fatalf("check_runs %#v", runs)
	}

	_, err = client.Get(ctx, "/fail", "")
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "404") || !strings.Contains(msg, "/fail") {
		t.Fatalf("error %q", msg)
	}
	if strings.Contains(msg, testToken) {
		t.Fatalf("token leaked in error %q", msg)
	}

	_, err = client.Get(ctx, "/bad-json", "")
	if err == nil {
		t.Fatal("expected JSON error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked in JSON error %q", err)
	}

	_, err = client.GetAll(ctx, "/not-list", "items", "")
	if err == nil || !strings.Contains(err.Error(), "not a list") {
		t.Fatalf("expected non-list error, got %v", err)
	}
}

func serverURL(r *http.Request, path string) string {
	return "http://" + r.Host + path
}
func TestAPIContentDecoding(t *testing.T) {
	payload := "hello policy\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	wrapped := encoded[:4] + "\n" + encoded[4:]
	contentJSON, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "ok.yml"):
			io.WriteString(w, `{"encoding":"base64","content":`+string(contentJSON)+`}`)
		case strings.Contains(r.URL.Path, "raw.yml"):
			io.WriteString(w, `{"encoding":"utf-8","content":"x"}`)
		case strings.Contains(r.URL.Path, "bad64.yml"):
			io.WriteString(w, `{"encoding":"base64","content":"!!!!"}`)
		case strings.Contains(r.URL.Path, "badutf.yml"):
			io.WriteString(w, `{"encoding":"base64","content":"`+base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe})+`"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewGitHubClient(testToken, server.URL, server.Client())
	ctx := context.Background()

	got, err := APIContent(ctx, client, "o/r", "ok.yml", "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if got != payload {
		t.Fatalf("got %q", got)
	}

	_, err = APIContent(ctx, client, "o/r", "raw.yml", "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "unsupported content encoding") {
		t.Fatalf("encoding error: %v", err)
	}
	_, err = APIContent(ctx, client, "o/r", "bad64.yml", "deadbeef")
	if err == nil {
		t.Fatal("expected base64 error")
	}
	_, err = APIContent(ctx, client, "o/r", "badutf.yml", "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("utf8 error: %v", err)
	}
}

func TestCollaboratorPermissionCacheAndFailure(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/collaborators/octo/permission") {
			http.NotFound(w, r)
			return
		}
		hits++
		if hits == 1 {
			io.WriteString(w, `{"permission":"admin"}`)
			return
		}
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	client := NewGitHubClient(testToken, server.URL, server.Client())
	ctx := context.Background()
	cache := map[string]*string{}
	if collaboratorPermission(ctx, client, "o/r", "", cache) != nil {
		t.Fatal("empty login")
	}
	first := collaboratorPermission(ctx, client, "o/r", "octo", cache)
	if first == nil || *first != "admin" {
		t.Fatalf("first %#v", first)
	}
	second := collaboratorPermission(ctx, client, "o/r", "octo", cache)
	if second == nil || *second != "admin" || hits != 1 {
		t.Fatalf("cache hits=%d second=%#v", hits, second)
	}
	other := collaboratorPermission(ctx, client, "o/r", "missing", map[string]*string{})
	if other != nil {
		t.Fatalf("failure should be nil, got %#v", other)
	}
}
