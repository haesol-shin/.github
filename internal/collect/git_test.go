package collect

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitEnvExtraheader(t *testing.T) {
	env := gitEnv([]string{"PATH=/usr/bin"}, testToken)
	joined := strings.Join(env, "\n")
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + testToken))
	if !strings.Contains(joined, "GIT_CONFIG_COUNT=1") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+basic) {
		t.Fatal(joined)
	}
	if strings.Contains(joined, testToken) {
		t.Fatalf("raw token in git env %q", joined)
	}
	empty := gitEnv(nil, "")
	for _, item := range empty {
		if strings.HasPrefix(item, "GIT_CONFIG_") {
			t.Fatalf("unexpected %q", item)
		}
	}
}

func TestGitMetadataExactDiffCases(t *testing.T) {
	src := t.TempDir()
	git := gitTest{t: t, dir: src, env: gitIdentity(t)}
	git.run("init", "--quiet", "--initial-branch=main")
	git.run("config", "core.autocrlf", "false")
	git.run("config", "core.quotePath", "true")

	writeFile(t, filepath.Join(src, "keep.txt"), "keep\n")
	writeFile(t, filepath.Join(src, "deleted.txt"), "gone\n")
	writeFile(t, filepath.Join(src, "quoted-é.txt"), "quoted\n")
	git.run("add", ".")
	git.run("commit", "-m", "base")
	base := strings.TrimSpace(git.output("rev-parse", "HEAD"))

	if err := os.Remove(filepath.Join(src, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "keep.txt"), "keep\nchanged\n")
	git.run("add", "-A")
	git.run("commit", "-m", "head")
	head := strings.TrimSpace(git.output("rev-parse", "HEAD"))

	bare := t.TempDir()
	runGitDir(t, "", gitIdentity(t), "clone", "--bare", "--quiet", src, bare)
	runGitDir(t, bare, gitIdentity(t), "update-ref", "refs/pull/7/head", head)

	pr := map[string]any{
		"number": 7,
		"head":   map[string]any{"sha": head},
		"base":   map[string]any{"repo": map[string]any{"clone_url": bare}},
	}
	gotBase, digest, err := GitMetadata(context.Background(), pr, "", base)
	if err != nil {
		t.Fatal(err)
	}
	if gotBase != base {
		t.Fatalf("merge base %s want %s", gotBase, base)
	}
	want := expectedDiffDigest(t, src, base, head)
	if digest != want {
		t.Fatalf("digest %s want %s", digest, want)
	}

	prMismatch := map[string]any{
		"number": 7,
		"head":   map[string]any{"sha": strings.Repeat("0", 40)},
		"base":   map[string]any{"repo": map[string]any{"clone_url": bare}},
	}
	_, _, err = GitMetadata(context.Background(), prMismatch, "", base)
	if err == nil || !strings.Contains(err.Error(), "fetched pull request head does not match the GitHub API") {
		t.Fatalf("sha mismatch: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), testToken) {
		t.Fatal(err)
	}

	_, _, err = GitMetadata(context.Background(), pr, "", strings.Repeat("e", 40))
	if err == nil {
		t.Fatal("expected missing merge-base error")
	}

	emptyHead := base
	runGitDir(t, bare, gitIdentity(t), "update-ref", "refs/pull/8/head", emptyHead)
	emptyPR := map[string]any{
		"number": 8,
		"head":   map[string]any{"sha": emptyHead},
		"base":   map[string]any{"repo": map[string]any{"clone_url": bare}},
	}
	_, emptyDigest, err := GitMetadata(context.Background(), emptyPR, "", base)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(nil)
	if emptyDigest != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("empty digest %s", emptyDigest)
	}
}

func TestGitMetadataResourceCapsFailClosed(t *testing.T) {
	pr, base := resourceCapRemote(t)
	const generous = int64(1 << 30)
	const generousEntries = 1 << 20
	cases := []struct {
		name   string
		limits gitLimits
		want   string
	}{
		{
			name: "fetched objects",
			limits: gitLimits{
				fetchedObjectBytes:  1,
				tempDiskBytes:       generous,
				diffInputBytes:      generous,
				expandedTreeEntries: generousEntries,
				diffOutputBytes:     generous,
			},
			want: "git fetched-object bytes exceeded limit",
		},
		{
			name: "temporary directory",
			limits: gitLimits{
				fetchedObjectBytes:  generous,
				tempDiskBytes:       1,
				diffInputBytes:      generous,
				expandedTreeEntries: generousEntries,
				diffOutputBytes:     generous,
			},
			want: "git temporary-directory bytes exceeded limit",
		},
		{
			name: "diff output",
			limits: gitLimits{
				fetchedObjectBytes:  generous,
				tempDiskBytes:       generous,
				diffInputBytes:      generous,
				expandedTreeEntries: generousEntries,
				diffOutputBytes:     16,
			},
			want: "git diff output bytes exceeded limit",
		},
		{
			name: "logical changed blobs",
			limits: gitLimits{
				fetchedObjectBytes:  generous,
				tempDiskBytes:       generous,
				diffInputBytes:      1024 * 1024,
				expandedTreeEntries: generousEntries,
				diffOutputBytes:     generous,
			},
			want: "git logical changed-blob bytes exceeded limit",
		},
		{
			name: "expanded tree entries",
			limits: gitLimits{
				fetchedObjectBytes:  generous,
				tempDiskBytes:       generous,
				diffInputBytes:      generous,
				expandedTreeEntries: 1,
				diffOutputBytes:     generous,
			},
			want: "git expanded tree entries exceeded limit",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := gitMetadataWithLimits(context.Background(), pr, "", base, tc.limits)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestGitMetadataCountsRepeatedBlobPerPath(t *testing.T) {
	src := t.TempDir()
	git := gitTest{t: t, dir: src, env: gitIdentity(t)}
	git.run("init", "--quiet", "--initial-branch=main")
	git.run("commit", "--allow-empty", "-m", "base")
	base := strings.TrimSpace(git.output("rev-parse", "HEAD"))
	content := strings.Repeat("same compressed line\n", 64*1024)
	for _, name := range []string{"one.txt", "two.txt", "three.txt"} {
		writeFile(t, filepath.Join(src, name), content)
	}
	git.run("add", ".")
	git.run("commit", "-m", "head")
	head := strings.TrimSpace(git.output("rev-parse", "HEAD"))
	bare := t.TempDir()
	runGitDir(t, "", gitIdentity(t), "clone", "--bare", "--quiet", src, bare)
	runGitDir(t, bare, gitIdentity(t), "update-ref", "refs/pull/10/head", head)
	pr := map[string]any{
		"number": 10,
		"head":   map[string]any{"sha": head},
		"base":   map[string]any{"repo": map[string]any{"clone_url": bare}},
	}
	const generous = int64(1 << 30)
	_, _, err := gitMetadataWithLimits(context.Background(), pr, "", base, gitLimits{
		fetchedObjectBytes:  generous,
		tempDiskBytes:       generous,
		diffInputBytes:      int64(len(content) * 2),
		expandedTreeEntries: 1 << 20,
		diffOutputBytes:     generous,
	})
	if err == nil || !strings.Contains(err.Error(), "git logical changed-blob bytes exceeded limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitMetadataCountsRepeatedSubtreePerPath(t *testing.T) {
	src := t.TempDir()
	git := gitTest{t: t, dir: src, env: gitIdentity(t)}
	git.run("init", "--quiet", "--initial-branch=main")
	git.run("commit", "--allow-empty", "-m", "base")
	base := strings.TrimSpace(git.output("rev-parse", "HEAD"))
	for _, directory := range []string{"one", "two", "three"} {
		parent := filepath.Join(src, directory, "nested")
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(parent, "payload.txt"), "same\n")
	}
	git.run("add", ".")
	git.run("commit", "-m", "head")
	head := strings.TrimSpace(git.output("rev-parse", "HEAD"))
	bare := t.TempDir()
	runGitDir(t, "", gitIdentity(t), "clone", "--bare", "--quiet", src, bare)
	runGitDir(t, bare, gitIdentity(t), "update-ref", "refs/pull/11/head", head)
	pr := map[string]any{
		"number": 11,
		"head":   map[string]any{"sha": head},
		"base":   map[string]any{"repo": map[string]any{"clone_url": bare}},
	}
	const generous = int64(1 << 30)
	_, _, err := gitMetadataWithLimits(context.Background(), pr, "", base, gitLimits{
		fetchedObjectBytes:  generous,
		tempDiskBytes:       generous,
		diffInputBytes:      generous,
		expandedTreeEntries: 5,
		diffOutputBytes:     generous,
	})
	if err == nil || !strings.Contains(err.Error(), "git expanded tree entries exceeded limit") {
		t.Fatalf("error = %v", err)
	}
}

func resourceCapRemote(t *testing.T) (map[string]any, string) {
	t.Helper()
	src := t.TempDir()
	git := gitTest{t: t, dir: src, env: gitIdentity(t)}
	git.run("init", "--quiet", "--initial-branch=main")
	writeFile(t, filepath.Join(src, "payload.txt"), "base\n")
	git.run("add", ".")
	git.run("commit", "-m", "base")
	base := strings.TrimSpace(git.output("rev-parse", "HEAD"))
	writeFile(t, filepath.Join(src, "payload.txt"), strings.Repeat("changed payload\n", 512*1024))
	git.run("add", ".")
	git.run("commit", "-m", "head")
	head := strings.TrimSpace(git.output("rev-parse", "HEAD"))

	bare := t.TempDir()
	runGitDir(t, "", gitIdentity(t), "clone", "--bare", "--quiet", src, bare)
	runGitDir(t, bare, gitIdentity(t), "update-ref", "refs/pull/9/head", head)
	return map[string]any{
		"number": 9,
		"head":   map[string]any{"sha": head},
		"base":   map[string]any{"repo": map[string]any{"clone_url": bare}},
	}, base
}

func expectedDiffDigest(t *testing.T, dir, base, head string) string {
	t.Helper()
	cmd := exec.Command("git",
		"-c", "core.quotePath=true",
		"diff",
		"--binary",
		"--full-index",
		"--no-color",
		"--no-ext-diff",
		"--no-textconv",
		"--no-renames",
		base, head, "--",
	)
	cmd.Dir = dir
	cmd.Env = gitIdentity(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(out)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type gitTest struct {
	t   *testing.T
	dir string
	env []string
}

func (g gitTest) run(args ...string) {
	g.t.Helper()
	runGitDir(g.t, g.dir, g.env, args...)
}

func (g gitTest) output(args ...string) string {
	g.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	cmd.Env = g.env
	out, err := cmd.Output()
	if err != nil {
		g.t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func runGitDir(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitIdentity(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=collector",
		"GIT_AUTHOR_EMAIL=collector@example.com",
		"GIT_COMMITTER_NAME=collector",
		"GIT_COMMITTER_EMAIL=collector@example.com",
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
