package dockerfile2llb

import (
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	"github.com/btidor/syntax/dockerfile/instructions"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

const PackageStepCount = 3

var aptions = strings.Join([]string{
	// Override the important apt options, since we don't know what
	// configuration the container ships with.
	"--option Acquire::ForceHash=sha256",
	"--option Acquire::GzipIndexes=false",
	"--option Dir::Cache=/btidor.syntax/cache",
	"--option Dir::Cache::archives=archives/",
	"--option Dir::State=/btidor.syntax/state",
	"--option Dir::State::lists=lists/",
	"--yes", "--quiet",
}, " ")

var aptRegex = regexp.MustCompile(`^'([^']*)'\s+([^ ]+)\s+([0-9]+)(\s+SHA256:([0-9a-fA-F]+))?`)

type PackageDownload struct {
	uri      string
	filename string
	size     int
	sha256   string
}

type PackageInvocation struct {
	d    *dispatchState
	cmd  *instructions.PackageCommand
	dopt dispatchOpt

	updateStage, downloadStage, installStage string
}

func NewPackageInvocation(d *dispatchState, c *instructions.PackageCommand,
	dopt dispatchOpt) *PackageInvocation {

	var names [3]string
	for i, stage := range []string{"update", "download", "install"} {
		// Precompute the three (`PackageStepCount`) step names. Note that
		// `prefixCommand` increments the step counter each time it's called.
		var msg = fmt.Sprintf("ADD (apt %s) %s", stage, strings.Join(c.PackageNames, " "))
		names[i] = prefixCommand(d, msg, false, nil, nil)
	}
	var i = PackageInvocation{d, c, dopt, names[0], names[1], names[2]}
	return &i
}

func (i *PackageInvocation) Dispatch() error {
	// Run `apt-get update` with the cache volume mounted.
	//
	// Since the cache volume is not guaranteed to persist between stages and
	// these files are required by `apt-get install`, copy them into the
	// temporary image.
	var tmp, err = i.Run(i.d.state, i.updateStage, false,
		[]string{
			"mkdir -p /btidor.syntax/state/lists/partial",
			fmt.Sprintf("apt-get update %s", aptions),
			"cp -r /btidor.syntax/state /btidor.syntax/backup",
		},
		llb.AddMount("/btidor.syntax/state", i.d.state,
			llb.AsPersistentCacheDir("btidor.syntax", llb.CacheMountShared)),
	)
	if err != nil {
		return err
	}

	// Run `apt-get install --download-only` through the Docker HTTP cache and
	// store results in the temporary image.
	tmp, err = i.Run(tmp, i.downloadStage, false, []string{
		"mv /btidor.syntax/backup /btidor.syntax/state",
		fmt.Sprintf("apt-get install -qq --print-uris %s %s "+
			"> /btidor.syntax/install", aptions, strings.Join(i.cmd.PackageNames, " ")),
	})
	if err != nil {
		return err
	}
	data, err := i.ReadFile(tmp, "/btidor.syntax/install")
	if err != nil {
		return err
	}
	uris, err := i.ParseURIs(data)
	if err != nil {
		return err
	}
	tmp, err = i.DownloadFiles(tmp, uris, "/btidor.syntax/cache/archives/")
	if err != nil {
		return err
	}

	// Run `apt-get install --no-download` in the original image. The temporary
	// image is used as a mount point to provide the sources and cache.
	i.d.state, err = i.Run(i.d.state, i.installStage, true,
		[]string{
			fmt.Sprintf("apt-get install --no-download %s %s",
				aptions, strings.Join(i.cmd.PackageNames, " ")),
		},
		llb.AddMount("/btidor.syntax", tmp, llb.SourcePath("/btidor.syntax")),
	)
	return err
}

func (i *PackageInvocation) Run(state llb.State, stageName string, withLayer bool,
	script []string, extra ...llb.RunOption) (llb.State, error) {

	// Options collected from `dispatchRun()`
	var opts = []llb.RunOption{
		llb.AddEnv("DEBIAN_FRONTEND", "noninteractive"),
		dfCmd(i.cmd),
		location(i.dopt.sourceMap, i.cmd.Location()),
		llb.WithCustomName(stageName),
		llb.Args(withShell(i.d.image, []string{strings.Join(script, " && ")})),
	}
	if i.d.ignoreCache {
		opts = append(opts, llb.IgnoreCache)
	}
	opts = append(opts, extra...)
	var next = state.Run(opts...).Root()

	var err = commitToHistory(&i.d.image, stageName, withLayer, &next, i.d.epoch)
	return next, err
}

func (i *PackageInvocation) ReadFile(state llb.State, path string) ([]byte, error) {
	// Unfortunately, this spooky action at a distance is required for state
	// marshalling to succeed in some cases. We're copying the behavior from the
	// very end of `toDispatchState()`.
	opts := []llb.LocalOption{}
	if includePatterns := normalizeContextPaths(i.d.ctxPaths); includePatterns != nil {
		opts = append(opts, llb.FollowPaths(includePatterns))
	}
	bctx, err := i.dopt.dockerClient.MainContext(i.dopt.context, opts...)
	if err != nil {
		return nil, err
	}
	i.dopt.rawBuildContext.Output = bctx.Output()

	state = state.SetMarshalDefaults(llb.Platform(i.dopt.targetPlatform))

	// Send the current state to be executed, then read the given file from the
	// layer.
	def, err := state.Marshal(i.dopt.context)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal LLB definition")
	}
	res, err := i.dopt.gatewayClient.Solve(i.dopt.context, client.SolveRequest{
		Definition:   def.ToPB(),
		CacheImports: i.dopt.dockerClient.CacheImports,
	})
	if err != nil {
		return nil, err
	}
	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}
	return ref.ReadFile(i.dopt.context, client.ReadRequest{Filename: path})
}

func (i *PackageInvocation) ParseURIs(uris []byte) ([]PackageDownload, error) {
	var results []PackageDownload
	for _, line := range strings.Split(string(uris), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		match := aptRegex.FindStringSubmatch(line)
		if match == nil {
			return nil, errors.Errorf("could not parse apt uri line: %q", line)
		}
		size, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, err
		}
		sha256 := ""
		if len(match) > 5 {
			sha256 = match[5]
		}
		results = append(results, PackageDownload{match[1], match[2], size, sha256})
	}
	return results, nil
}

func (i *PackageInvocation) DownloadFiles(base llb.State, files []PackageDownload,
	destination string) (llb.State, error) {

	var action *llb.FileAction
	var mode = fs.FileMode(0644)
	var copyOpt = &llb.CopyInfo{
		Mode:           &mode,
		CreateDestPath: true,
	}
	for _, file := range files {
		var httpOpts = []llb.HTTPOption{
			llb.Filename(file.filename),
		}
		if file.sha256 != "" {
			httpOpts = append(httpOpts, llb.Checksum(
				digest.NewDigestFromEncoded(digest.SHA256, file.sha256)))
		}
		http := llb.HTTP(file.uri, httpOpts...)
		if action == nil {
			action = llb.Copy(http, file.filename, destination, copyOpt)
		} else {
			action = action.Copy(http, file.filename, destination, copyOpt)
		}
	}
	return base.File(action,
		dfCmd(i.cmd),
		location(i.dopt.sourceMap, i.cmd.Location()),
		llb.WithCustomName("COPY (apt packages)"),
	), nil
}
