package collect

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The byte caps are 16× the largest measured PR 13/14 preparation sample:
// 409447 aggregate object bytes, 240422 temporary bytes, 438762 logical
// changed-blob bytes with per-path multiplicity, and 383732 diff-output bytes.
const (
	gitReadSize                 = 1024 * 1024
	maxFetchedObjectBytes int64 = 6551152
	maxTempDiskBytes      int64 = 3846752
	maxDiffInputBytes     int64 = 7020192
	maxDiffOutputBytes    int64 = 6139712
	diskPollInterval            = 5 * time.Millisecond
)

type gitLimits struct {
	fetchedObjectBytes int64
	tempDiskBytes      int64
	diffInputBytes     int64
	diffOutputBytes    int64
}

var productionGitLimits = gitLimits{
	fetchedObjectBytes: maxFetchedObjectBytes,
	tempDiskBytes:      maxTempDiskBytes,
	diffInputBytes:     maxDiffInputBytes,
	diffOutputBytes:    maxDiffOutputBytes,
}

func (g *GitRunner) Metadata(ctx context.Context, pullRequest map[string]any, token, mergeBase string) (string, string, error) {
	return GitMetadata(ctx, pullRequest, token, mergeBase)
}

func GitMetadata(ctx context.Context, pullRequest map[string]any, token, mergeBase string) (string, string, error) {
	return gitMetadataWithLimits(ctx, pullRequest, token, mergeBase, productionGitLimits)
}

func gitMetadataWithLimits(ctx context.Context, pullRequest map[string]any, token, mergeBase string, limits gitLimits) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	number, ok := lookupInt(pullRequest["number"])
	if !ok {
		return "", "", fmt.Errorf("pull request number is required")
	}
	headSHA, ok := nestedString(pullRequest, "head", "sha")
	if !ok || headSHA == "" {
		return "", "", fmt.Errorf("pull request head SHA is required")
	}
	cloneURL, ok := nestedString(pullRequest, "base", "repo", "clone_url")
	if !ok || cloneURL == "" {
		return "", "", fmt.Errorf("pull request clone URL is required")
	}

	directory, err := os.MkdirTemp("", "repo-ops-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(directory)

	env := gitEnv(os.Environ(), token)
	if err := runGitLimited(ctx, "", directory, env, limits.tempDiskBytes, "init", "--quiet", directory); err != nil {
		return "", "", err
	}
	fetchedBytes := int64(0)
	fetch := func(refspec string) error {
		objects := filepath.Join(directory, ".git", "objects")
		if err := runGitLimited(
			ctx,
			directory,
			directory,
			env,
			limits.tempDiskBytes,
			"fetch", "--quiet", "--no-tags", "--depth=1", cloneURL, refspec,
		); err != nil {
			return err
		}
		after, err := directorySize(objects)
		if err != nil {
			return err
		}
		// Count each post-fetch object store size. This conservatively includes
		// already-fetched objects rather than undercounting pack replacement.
		fetchedBytes += after
		if fetchedBytes > limits.fetchedObjectBytes {
			return fmt.Errorf(
				"git fetched-object bytes exceeded limit (%d > %d)",
				fetchedBytes,
				limits.fetchedObjectBytes,
			)
		}
		return checkDirectoryLimit(directory, limits.tempDiskBytes)
	}

	refspec := "refs/pull/" + strconv.Itoa(number) + "/head:refs/repo-ops/head"
	if err := fetch(refspec); err != nil {
		return "", "", err
	}
	fetched, err := gitOutputLimited(
		ctx,
		directory,
		directory,
		env,
		limits.tempDiskBytes,
		"rev-parse",
		"refs/repo-ops/head",
	)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(fetched) != headSHA {
		return "", "", fmt.Errorf("fetched pull request head does not match the GitHub API")
	}
	if err := runGitLimited(ctx, directory, directory, env, limits.tempDiskBytes, "cat-file", "-e", mergeBase+"^{commit}"); err != nil {
		baseRefspec := mergeBase + ":refs/repo-ops/base"
		if err := fetch(baseRefspec); err != nil {
			return "", "", err
		}
	}
	if err := runGitLimited(ctx, directory, directory, env, limits.tempDiskBytes, "update-ref", "refs/repo-ops/base", mergeBase); err != nil {
		return "", "", err
	}
	if err := checkDiffInputBytes(ctx, directory, env, limits); err != nil {
		return "", "", err
	}

	digest, err := gitDiffDigest(ctx, directory, env, limits)
	if err != nil {
		return "", "", err
	}
	return mergeBase, digest, nil
}

func gitDiffDigest(ctx context.Context, directory string, env []string, limits gitLimits) (string, error) {
	args := []string{
		"-c", "core.quotePath=true",
		"diff",
		"--binary",
		"--full-index",
		"--no-color",
		"--no-ext-diff",
		"--no-textconv",
		"--no-renames",
		"refs/repo-ops/base",
		"refs/repo-ops/head",
		"--",
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	stopMonitor := monitorDirectory(cmd, directory, limits.tempDiskBytes)
	digest := sha256.New()
	buf := make([]byte, gitReadSize)
	written, copyErr := io.CopyBuffer(
		digest,
		io.LimitReader(stdout, limits.diffOutputBytes+1),
		buf,
	)
	if copyErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		monitorErr := stopMonitor()
		if monitorErr != nil {
			return "", monitorErr
		}
		return "", copyErr
	}
	if written > limits.diffOutputBytes {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stopMonitor()
		return "", fmt.Errorf(
			"git diff output bytes exceeded limit (%d > %d)",
			written,
			limits.diffOutputBytes,
		)
	}
	if err := cmd.Wait(); err != nil {
		monitorErr := stopMonitor()
		if monitorErr != nil {
			return "", monitorErr
		}
		return "", fmt.Errorf("git diff: %w", err)
	}
	if err := stopMonitor(); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func checkDiffInputBytes(ctx context.Context, directory string, env []string, limits gitLimits) error {
	raw, err := gitBoundedOutput(
		ctx,
		directory,
		env,
		limits.tempDiskBytes,
		maxDiffOutputBytes,
		"",
		"diff-tree",
		"-r",
		"--raw",
		"--full-index",
		"--no-commit-id",
		"--no-renames",
		"refs/repo-ops/base",
		"refs/repo-ops/head",
		"--",
	)
	if err != nil {
		return fmt.Errorf("git diff-tree: %w", err)
	}
	var names []string
	zero := strings.Repeat("0", 40)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		metadata := strings.Fields(strings.SplitN(line, "\t", 2)[0])
		if len(metadata) < 5 || !strings.HasPrefix(metadata[0], ":") {
			return fmt.Errorf("git diff-tree returned malformed metadata")
		}
		for _, object := range metadata[2:4] {
			if object != zero {
				names = append(names, object)
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	input := strings.Join(names, "\n") + "\n"
	checked, err := gitBoundedOutput(
		ctx,
		directory,
		env,
		limits.tempDiskBytes,
		maxDiffOutputBytes,
		input,
		"cat-file",
		"--batch-check=%(objectname) %(objecttype) %(objectsize)",
	)
	if err != nil {
		return fmt.Errorf("git cat-file: %w", err)
	}
	total := int64(0)
	for _, line := range strings.Split(strings.TrimSpace(checked), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return fmt.Errorf("git cat-file returned malformed metadata")
		}
		if fields[1] != "blob" {
			continue
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return fmt.Errorf("git cat-file returned invalid object size")
		}
		if size > limits.diffInputBytes-total {
			return fmt.Errorf(
				"git logical changed-blob bytes exceeded limit (%d + %d > %d)",
				total,
				size,
				limits.diffInputBytes,
			)
		}
		total += size
	}
	return nil
}

func gitBoundedOutput(
	ctx context.Context,
	directory string,
	env []string,
	diskLimit, outputLimit int64,
	input string,
	args ...string,
) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	stopMonitor := monitorDirectory(cmd, directory, diskLimit)
	var output bytes.Buffer
	written, copyErr := io.CopyBuffer(
		&output,
		io.LimitReader(stdout, outputLimit+1),
		make([]byte, 32*1024),
	)
	if copyErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		monitorErr := stopMonitor()
		if monitorErr != nil {
			return "", monitorErr
		}
		return "", copyErr
	}
	if written > outputLimit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stopMonitor()
		return "", fmt.Errorf("git metadata output bytes exceeded limit (%d > %d)", written, outputLimit)
	}
	if err := cmd.Wait(); err != nil {
		monitorErr := stopMonitor()
		if monitorErr != nil {
			return "", monitorErr
		}
		return "", err
	}
	if err := stopMonitor(); err != nil {
		return "", err
	}
	return output.String(), nil
}

func gitEnv(parent []string, token string) []string {
	env := append([]string{}, parent...)
	if token == "" {
		return env
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+basic,
	)
}

func runGitLimited(ctx context.Context, dir, monitorDir string, env []string, diskLimit int64, args ...string) error {
	_, err := gitOutputLimited(ctx, dir, monitorDir, env, diskLimit, args...)
	return err
}

func gitOutputLimited(ctx context.Context, dir, monitorDir string, env []string, diskLimit int64, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}
	stopMonitor := monitorDirectory(cmd, monitorDir, diskLimit)
	if err := cmd.Wait(); err != nil {
		monitorErr := stopMonitor()
		if monitorErr != nil {
			return "", monitorErr
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	if err := stopMonitor(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func monitorDirectory(cmd *exec.Cmd, directory string, limit int64) func() error {
	done := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(diskPollInterval)
		defer ticker.Stop()
		for {
			if err := checkDirectoryLimit(directory, limit); err != nil {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				result <- err
				return
			}
			select {
			case <-done:
				result <- checkDirectoryLimit(directory, limit)
				return
			case <-ticker.C:
			}
		}
	}()
	return func() error {
		close(done)
		return <-result
	}
}

func checkDirectoryLimit(directory string, limit int64) error {
	size, err := directorySize(directory)
	if err != nil {
		return err
	}
	if size > limit {
		return fmt.Errorf("git temporary-directory bytes exceeded limit (%d > %d)", size, limit)
	}
	return nil
}

func directorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		size += info.Size()
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return size, err
}
