package repositorypolicy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type actionManifest struct {
	Runs struct {
		Steps []struct {
			Env map[string]string `yaml:"env"`
			Run string            `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"runs"`
}

type artifactPin struct {
	SourceCommit string `json:"source_commit"`
	Toolchain    string `json:"toolchain"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	CGOEnabled   string `json:"cgo_enabled"`
	GoModSHA256  string `json:"go_mod_sha256"`
	GoSumSHA256  string `json:"go_sum_sha256"`
	Recipe       string `json:"recipe"`
	SHA256       string `json:"sha256"`
	Size         int    `json:"size"`
	URI          string `json:"uri"`
}

const buildRecipe = "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o repo-ops-validator ./cmd/repo-ops-validator"
const testToken = "repo-ops-action-test-secret"

func TestRepositoryPolicyActionTrustBoundary(t *testing.T) {
	requireTool(t, "bash")
	requireTool(t, "jq")
	requireTool(t, "node")
	requireTool(t, "sha256sum")

	binary := []byte("#!/bin/sh\n[ \"$GITHUB_TOKEN\" = repo-ops-action-test-secret ] || exit 23\nprintf 'event=%s\\ncheck=%s\\nroot=%s\\n' \"$2\" \"$4\" \"$REPO_OPS_CONTRACT_ROOT\"\nif [ -n \"${FAKE_MARKER:-}\" ]; then printf executed >\"$FAKE_MARKER\"; fi\n")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestData := r.URL.String() + "\n" + r.Host + "\n" + fmt.Sprint(r.Header)
		if strings.Contains(requestData, testToken) {
			t.Errorf("credential leaked to artifact server")
			http.Error(w, "credential leak", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/artifact":
			_, _ = w.Write(binary)
		case "/redirect":
			http.Redirect(w, r, "/artifact", http.StatusFound)
		case "/downgrade":
			http.Redirect(w, r, "http://example.invalid/artifact", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/empty":
			w.Header().Set("Content-Length", "0")
		case "/truncated":
			w.Header().Set("Content-Length", fmt.Sprint(len(binary)+1))
			_, _ = w.Write(binary)
		case "/oversized":
			w.(http.Flusher).Flush()
			_, _ = w.Write(append(binary, 'x'))
		case "/timeout":
			w.(http.Flusher).Flush()
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if _, err := w.Write([]byte("x")); err != nil {
						return
					}
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	certificate := filepath.Join(t.TempDir(), "server.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(certificate, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, check := range []string{"contract", "merge-approval"} {
		t.Run("verified artifact executes "+check+" with the event and base root", func(t *testing.T) {
			env := newActionEnvironment(t, binary, server.URL+"/artifact", certificate)
			env.check = check
			result := runAction(t, env)
			if result.err != nil {
				t.Fatalf("action failed: %v\n%s", result.err, result.output)
			}
			want := "event=" + env.eventPath + "\ncheck=" + check + "\nroot=" + env.root + "\n"
			if result.output != want {
				t.Fatalf("output mismatch\nwant: %q\n got: %q", want, result.output)
			}
		})
	}

	t.Run("HTTPS redirect preserves verified execution", func(t *testing.T) {
		env := newActionEnvironment(t, binary, server.URL+"/redirect", certificate)
		result := runAction(t, env)
		if result.err != nil {
			t.Fatalf("action failed after HTTPS redirect: %v\n%s", result.err, result.output)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *actionEnvironment)
	}{
		{"missing pin", func(t *testing.T, env *actionEnvironment) { os.Remove(env.pinPath) }},
		{"unparseable pin", func(t *testing.T, env *actionEnvironment) { writeFile(t, env.pinPath, []byte("{")) }},
		{"missing required field", func(t *testing.T, env *actionEnvironment) {
			env.updatePinObject(t, func(pin map[string]any) { delete(pin, "toolchain") })
		}},
		{"unknown pin field", func(t *testing.T, env *actionEnvironment) {
			var value map[string]any
			readJSON(t, env.pinPath, &value)
			value["unexpected"] = true
			writeJSON(t, env.pinPath, value)
		}},
		{"empty toolchain", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.Toolchain = "" })
		}},
		{"wrong operating system", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.GOOS = "darwin" })
		}},
		{"invalid source commit", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.SourceCommit = "not-a-commit" })
		}},
		{"wrong architecture", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.GOARCH = "arm64" })
		}},
		{"enabled cgo", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.CGOEnabled = "1" })
		}},
		{"malformed module hash", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.GoModSHA256 = "not-a-digest" })
		}},
		{"malformed artifact hash", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.SHA256 = "not-a-digest" })
		}},
		{"uppercase artifact hash", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.SHA256 = strings.ToUpper(pin.SHA256) })
		}},
		{"prefixless artifact hash", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.SHA256 = strings.TrimPrefix(pin.SHA256, "sha256:") })
		}},
		{"wrong build recipe", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.Recipe = "go build ./cmd/repo-ops-validator" })
		}},
		{"nonpositive size", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.Size = 0 })
		}},
		{"fractional size", func(t *testing.T, env *actionEnvironment) {
			env.updatePinObject(t, func(pin map[string]any) { pin["size"] = 1.5 })
		}},
		{"unsafe integer size", func(t *testing.T, env *actionEnvironment) {
			env.updatePinObject(t, func(pin map[string]any) { pin["size"] = 9007199254740992 })
		}},
		{"string size", func(t *testing.T, env *actionEnvironment) {
			env.updatePinObject(t, func(pin map[string]any) { pin["size"] = "123" })
		}},
		{"non-HTTPS URI", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = "http://example.invalid/artifact" })
		}},
		{"redirect protocol downgrade", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = server.URL + "/downgrade" })
		}},
		{"redirect limit", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = server.URL + "/loop" })
		}},
		{"unreachable URI", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = "https://127.0.0.1:1/artifact" })
		}},
		{"empty response", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = server.URL + "/empty" })
		}},
		{"truncated response", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = server.URL + "/truncated" })
		}},
		{"oversized response", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.URI = server.URL + "/oversized" })
		}},
		{"length mismatch", func(t *testing.T, env *actionEnvironment) { env.updatePin(t, func(pin *artifactPin) { pin.Size++ }) }},
		{"digest mismatch", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.SHA256 = "sha256:" + strings.Repeat("0", 64) })
		}},
		{"go.mod mismatch", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.GoModSHA256 = "sha256:" + strings.Repeat("0", 64) })
		}},
		{"go.sum mismatch", func(t *testing.T, env *actionEnvironment) {
			env.updatePin(t, func(pin *artifactPin) { pin.GoSumSHA256 = "sha256:" + strings.Repeat("0", 64) })
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newActionEnvironment(t, binary, server.URL+"/artifact", certificate)
			marker := filepath.Join(t.TempDir(), "executed")
			env.extra = append(env.extra, "FAKE_MARKER="+marker)
			test.mutate(t, env)
			result := runAction(t, env)
			if result.err == nil {
				t.Fatalf("action unexpectedly succeeded: %s", result.output)
			}
			if strings.Contains(result.output, env.token) {
				t.Fatalf("credential leaked in failure output: %q", result.output)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("rejected artifact executed; marker stat: %v", err)
			}
		})
	}

	t.Run("unverified bytes never execute", func(t *testing.T) {
		env := newActionEnvironment(t, binary, server.URL+"/artifact", certificate)
		marker := filepath.Join(t.TempDir(), "executed")
		env.extra = append(env.extra, "FAKE_MARKER="+marker)
		env.updatePin(t, func(pin *artifactPin) { pin.SHA256 = "sha256:" + strings.Repeat("0", 64) })
		result := runAction(t, env)
		if result.err == nil {
			t.Fatal("action unexpectedly succeeded")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("unverified artifact executed; marker stat: %v", err)
		}
	})

	t.Run("download has an absolute thirty second timeout", func(t *testing.T) {
		env := newActionEnvironment(t, binary, server.URL+"/timeout", certificate)
		started := time.Now()
		result := runAction(t, env)
		elapsed := time.Since(started)
		if result.err == nil {
			t.Fatal("action unexpectedly succeeded")
		}
		if elapsed < 29*time.Second || elapsed > 35*time.Second {
			t.Fatalf("timeout was %s, want 29s..35s", elapsed)
		}
	})
}

type actionEnvironment struct {
	root        string
	actionPath  string
	pinPath     string
	eventPath   string
	runnerTemp  string
	certificate string
	token       string
	check       string
	extra       []string
}

type actionResult struct {
	output string
	err    error
}

func newActionEnvironment(t *testing.T, binary []byte, uri, certificate string) *actionEnvironment {
	t.Helper()
	repositoryRoot := projectRoot(t)
	root := t.TempDir()
	actionPath := filepath.Join(root, "actions", "repository-policy")
	if err := os.MkdirAll(actionPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"action.yml", "run.sh", "download.mjs"} {
		content, err := os.ReadFile(filepath.Join(repositoryRoot, "actions", "repository-policy", name))
		if err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if name == "run.sh" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(actionPath, name), content, mode); err != nil {
			t.Fatal(err)
		}
	}

	mod := []byte("module example.invalid/action-test\n\ngo 1.22\n")
	sum := []byte("example.invalid/module v1.0.0 h1:test\n")
	writeFile(t, filepath.Join(root, "go.mod"), mod)
	writeFile(t, filepath.Join(root, "go.sum"), sum)
	eventPath := filepath.Join(root, "event.json")
	writeFile(t, eventPath, []byte("{}\n"))

	pin := artifactPin{
		SourceCommit: strings.Repeat("a", 40),
		Toolchain:    "go version go1.22.12 linux/arm64",
		GOOS:         "linux",
		GOARCH:       "amd64",
		CGOEnabled:   "0",
		GoModSHA256:  digest(mod),
		GoSumSHA256:  digest(sum),
		Recipe:       buildRecipe,
		SHA256:       digest(binary),
		Size:         len(binary),
		URI:          uri,
	}
	pinPath := filepath.Join(actionPath, "artifact.json")
	writeJSON(t, pinPath, pin)

	return &actionEnvironment{
		root: root, actionPath: actionPath, pinPath: pinPath,
		eventPath: eventPath, runnerTemp: t.TempDir(), certificate: certificate,
		token: testToken, check: "contract",
	}
}

func (env *actionEnvironment) updatePin(t *testing.T, update func(*artifactPin)) {
	t.Helper()
	var pin artifactPin
	readJSON(t, env.pinPath, &pin)
	update(&pin)
	writeJSON(t, env.pinPath, pin)
}

func (env *actionEnvironment) updatePinObject(t *testing.T, update func(map[string]any)) {
	t.Helper()
	var pin map[string]any
	readJSON(t, env.pinPath, &pin)
	update(pin)
	writeJSON(t, env.pinPath, pin)
}

func runAction(t *testing.T, env *actionEnvironment) actionResult {
	t.Helper()
	manifestBytes, err := os.ReadFile(filepath.Join(env.actionPath, "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest actionManifest
	if err := yaml.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Runs.Steps) != 1 || manifest.Runs.Steps[0].Run == "" {
		t.Fatal("composite action must contain one executable validation step")
	}
	step := manifest.Runs.Steps[0]
	if len(step.Env) != 2 ||
		step.Env["GITHUB_TOKEN"] != "${{ inputs['github-token'] }}" ||
		step.Env["INPUT_CHECK"] != "${{ inputs.check }}" {
		t.Fatalf("composite action input wiring changed: %#v", step.Env)
	}

	command := exec.Command("bash", "-c", step.Run)
	command.Env = append(os.Environ(),
		"GITHUB_ACTION_PATH="+env.actionPath,
		"GITHUB_EVENT_PATH="+env.eventPath,
		"GITHUB_TOKEN="+env.token,
		"INPUT_CHECK="+env.check,
		"RUNNER_TEMP="+env.runnerTemp,
		"NODE_EXTRA_CA_CERTS="+env.certificate,
	)
	command.Env = append(command.Env, env.extra...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err = command.Run()
	if strings.Contains(output.String(), env.token) {
		t.Fatalf("credential leaked in action output: %q", output.String())
	}
	return actionResult{output: output.String(), err: err}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, append(content, '\n'))
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is unavailable: %v", name, err)
	}
}
