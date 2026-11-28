// Package container runs a single command in a Docker container
// matching the CI environment, and proves a fix locally.
//
// It is deliberately not a CI runner: it runs one command, once,
// in one container. No workflow engine, no services, no caching,
// no `act` clone. The image comes from, in order: the --image
// flag, the CI fingerprint's runner image, the Dockerfile, the
// workflow's runs-on label, or the default ubuntu:24.04.
//
// Docker is required at run time, not at build time. When Docker
// is missing, Run fails with a clear error and --dry-run still
// shows the exact command that would run.
package container

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Ciago-Technologies/local-ci/internal/fingerprint"
)

// Options tunes a container run.
type Options struct {
	// Image is an explicit image (--image). Wins over everything.
	Image string
	// Dir is the repo directory mounted at /work. Defaults to ".".
	Dir string
	// Fingerprint is a CI fingerprint, used for image selection.
	Fingerprint *fingerprint.Fingerprint
	// RunsOn is a workflow runs-on label, e.g. "ubuntu-latest".
	RunsOn string
	// DockerfileImage is the repo Dockerfile's FROM image, if any.
	DockerfileImage string
	// Env lists vars to pass into the container: "NAME" (value
	// taken from the host environment) or "NAME=VALUE".
	Env []string
	// Services lists images to start detached before the main
	// run, e.g. "postgres:16" or "db=postgres:16". Each starts as
	// docker run -d --name localci-<name> <image> and is left
	// running afterwards — the caller stops it with
	// `docker rm -f localci-<name>`.
	Services []string
	// DryRun prints the docker command instead of running it.
	DryRun bool
	// Command is the command to run inside the container.
	Command []string
	// Script, when true, means Command[0] is a whole shell script
	// (e.g. a workflow step's run: block) and is passed to sh -c
	// verbatim. When false, Command is a word list joined with
	// shell quoting.
	Script bool
}

// ErrDockerMissing means no docker binary was found.
var ErrDockerMissing = fmt.Errorf("docker is required for localci run — install Docker (https://docs.docker.com/get-docker) and try again")

// DefaultImage is used when nothing else selects an image. It
// tracks the current ubuntu-latest runner image.
const DefaultImage = "ubuntu:24.04"

// ResolveImage picks the container image and explains where it
// came from. Precedence: explicit flag, CI fingerprint runner
// image, CI fingerprint Dockerfile image, local Dockerfile image,
// runs-on label, default.
func ResolveImage(o Options) (image, source string, err error) {
	if o.Image != "" {
		return o.Image, "from --image flag", nil
	}
	if o.Fingerprint != nil {
		if img, ok := runnerImageToDocker(o.Fingerprint.RunnerImage); ok {
			return img, "from CI runner image " + o.Fingerprint.RunnerImage, nil
		}
		if o.Fingerprint.ContainerImage != "" {
			return o.Fingerprint.ContainerImage, "from CI Dockerfile (FROM " + o.Fingerprint.ContainerImage + ")", nil
		}
	}
	if o.DockerfileImage != "" {
		return o.DockerfileImage, "from local Dockerfile (FROM " + o.DockerfileImage + ")", nil
	}
	if o.RunsOn != "" {
		img, err := runsOnToDocker(o.RunsOn)
		if err != nil {
			return "", "", err
		}
		return img, "from runs-on " + o.RunsOn, nil
	}
	return DefaultImage, "default (no image selected — using " + DefaultImage + ")", nil
}

var ubuntuImageRe = regexp.MustCompile(`^ubuntu(\d+)(?:\.\d+)?`)

// runnerImageToDocker maps a GitHub runner image ("ubuntu24",
// "ubuntu24/20251123.1") to a public Docker image ("ubuntu:24.04").
func runnerImageToDocker(runnerImage string) (string, bool) {
	base := runnerImage
	if i := strings.IndexByte(base, '/'); i >= 0 {
		base = base[:i]
	}
	m := ubuntuImageRe.FindStringSubmatch(strings.ToLower(base))
	if m == nil {
		return "", false
	}
	return "ubuntu:" + m[1] + ".04", true
}

// runsOnToDocker maps a runs-on label to a Docker image. macOS and
// Windows runners cannot run in Docker — that is an error, not a
// silent Linux substitution.
func runsOnToDocker(runsOn string) (string, error) {
	lower := strings.ToLower(runsOn)
	switch {
	case strings.Contains(lower, "macos"):
		return "", fmt.Errorf("runs-on %s is a macOS runner — macOS cannot run in Docker; use --image with a Linux image instead", runsOn)
	case strings.Contains(lower, "windows"):
		return "", fmt.Errorf("runs-on %s is a Windows runner — Windows cannot run in Docker; use --image with a Linux image instead", runsOn)
	case strings.Contains(lower, "ubuntu-22.04") || strings.Contains(lower, "ubuntu-22"):
		return "ubuntu:22.04", nil
	case strings.Contains(lower, "ubuntu-20.04") || strings.Contains(lower, "ubuntu-20"):
		return "ubuntu:20.04", nil
	case strings.Contains(lower, "ubuntu"):
		return "ubuntu:24.04", nil
	default:
		return "", fmt.Errorf("cannot map runs-on %s to a Docker image — use --image", runsOn)
	}
}

// BuildArgs returns the docker argv for this run: docker run --rm
// -v <dir>:/work -w /work [--env ...] <image> sh -c "<command>".
// Env values are passed through to docker, never logged.
func BuildArgs(o Options, image string) ([]string, error) {
	if len(o.Command) == 0 {
		return nil, fmt.Errorf("no command to run")
	}
	abs, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--rm",
		"-v", abs + ":/work",
		"-w", "/work",
		"--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()),
	}
	for _, e := range o.Env {
		name, _, _ := strings.Cut(e, "=")
		if name == "" {
			continue
		}
		args = append(args, "-e", e)
	}
	script := shellJoin(o.Command)
	if o.Script {
		script = o.Command[0]
	}
	args = append(args, image, "sh", "-c", script)
	return args, nil
}

// ServiceContainerName returns the detached container name for a
// service image: "postgres:16" -> "localci-postgres",
// "db=postgres:16" -> "localci-db".
func ServiceContainerName(service string) string {
	name, image := service, service
	if n, img, ok := strings.Cut(service, "="); ok && n != "" && img != "" {
		name, image = n, img
	} else {
		// Name from the image: last path segment before the tag.
		base := image
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		if i := strings.IndexByte(base, ':'); i >= 0 {
			base = base[:i]
		}
		if i := strings.IndexByte(base, '@'); i >= 0 {
			base = base[:i]
		}
		name = base
	}
	// Sanitise to [a-z0-9-].
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	name = strings.Trim(b.String(), "-")
	if name == "" {
		name = "service"
	}
	return "localci-" + name
}

// ServiceImage returns just the image part of a --service entry.
func ServiceImage(service string) string {
	if _, img, ok := strings.Cut(service, "="); ok && img != "" {
		return img
	}
	return service
}

// ServiceStartArgs returns the docker argv to start one service detached.
func ServiceStartArgs(service string) []string {
	return []string{"run", "-d", "--name", ServiceContainerName(service), ServiceImage(service)}
}

// ServiceStartCommands returns the docker commands to start all services.
func (o Options) ServiceStartCommands() [][]string {
	var out [][]string
	for _, s := range o.Services {
		if strings.TrimSpace(s) == "" {
			continue
		}
		out = append(out, ServiceStartArgs(s))
	}
	return out
}

// DryRunString renders the docker command as a human would type
// it, with env values masked (names only — values are never
// printed, even in --dry-run output).
func DryRunString(o Options, image string) (string, error) {
	args, err := BuildArgs(o, image)
	if err != nil {
		return "", err
	}
	parts := []string{"docker"}
	for i, a := range args {
		if i > 0 && args[i-1] == "-e" {
			name, _, _ := strings.Cut(a, "=")
			if strings.Contains(a, "=") {
				parts = append(parts, shellQuote(name+"=***"))
			} else {
				parts = append(parts, shellQuote(a))
			}
			continue
		}
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " "), nil
}

// Run executes the container run, streaming its output to stdout
// and stderr. It returns the container's exit code. When DryRun is
// set, it prints the docker command and returns 0 without running
// anything.
func Run(o Options, stdout, stderr io.Writer) (int, error) {
	image, source, err := ResolveImage(o)
	if err != nil {
		return 2, err
	}
	if o.DryRun {
		for _, svcArgs := range o.ServiceStartCommands() {
			fmt.Fprintf(stdout, "# service: docker %s\n", strings.Join(svcArgs, " "))
		}
		s, err := DryRunString(o, image)
		if err != nil {
			return 2, err
		}
		fmt.Fprintf(stdout, "# image: %s (%s)\n%s\n", image, source, s)
		if len(o.ServiceStartCommands()) > 0 {
			fmt.Fprintf(stdout, "# services are left running — stop them with: docker rm -f %s\n", serviceNames(o))
		}
		return 0, nil
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return 2, ErrDockerMissing
	}
	// Start services first, detached. They are left running
	// afterwards — print a reminder to stop them.
	for _, svcArgs := range o.ServiceStartCommands() {
		cmd := exec.Command(docker, svcArgs...)
		cmd.Stdout = stderr
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return 2, fmt.Errorf("starting service %s: %w (stop any running service with docker rm -f %s)", svcArgs[len(svcArgs)-1], err, ServiceContainerName(svcArgs[len(svcArgs)-1]))
		}
		fmt.Fprintf(stderr, "localci run: service started: %s\n", ServiceContainerName(svcArgs[len(svcArgs)-1]))
	}
	args, err := BuildArgs(o, image)
	if err != nil {
		return 2, err
	}
	fmt.Fprintf(stderr, "localci run: image %s (%s)\n", image, source)
	cmd := exec.Command(docker, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if len(o.ServiceStartCommands()) > 0 {
				fmt.Fprintf(stderr, "localci run: services left running: %s (docker rm -f %s to stop)\n", serviceNames(o), serviceNames(o))
			}
			return ee.ExitCode(), nil
		}
		return 2, err
	}
	if len(o.ServiceStartCommands()) > 0 {
		fmt.Fprintf(stderr, "localci run: services left running: %s (docker rm -f %s to stop)\n", serviceNames(o), serviceNames(o))
	}
	return 0, nil
}

func serviceNames(o Options) string {
	var names []string
	for _, s := range o.Services {
		if strings.TrimSpace(s) != "" {
			names = append(names, ServiceContainerName(s))
		}
	}
	return strings.Join(names, " ")
}

// shellJoin joins a command for sh -c, quoting each word.
func shellJoin(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = shellQuote(w)
	}
	return strings.Join(quoted, " ")
}

// shellQuote quotes one word for POSIX sh.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`!*?~#()[]{}<>|&;") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
