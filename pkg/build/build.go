// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package build

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/driver"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/imagetools"
	"github.com/vmware-tanzu/buildkit-cli-for-kubectl/pkg/progress"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/docker/distribution/reference"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/urlutil"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/upload/uploadprovider"
	"github.com/moby/buildkit/util/entitlements"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var (
	errStdinConflict      = errors.New("invalid argument: can't use stdin for both build context and dockerfile")
	errDockerfileConflict = errors.New("ambiguous Dockerfile source: both stdin and flag correspond to Dockerfiles")
)

type Options struct {
	Inputs      Inputs
	Tags        []string
	Labels      map[string]string
	BuildArgs   map[string]string
	Pull        bool
	ImageIDFile string
	ExtraHosts  []string
	NetworkMode string

	NoCache   bool
	Target    string
	Platforms []specs.Platform
	Exports   []client.ExportEntry
	Session   []session.Attachable

	CacheFrom []client.CacheOptionsEntry
	CacheTo   []client.CacheOptionsEntry

	Allow []entitlements.Entitlement
	// DockerTarget
	FrontendImage string
}

type Inputs struct {
	ContextPath    string
	DockerfilePath string
	InStream       io.Reader
}

type DriverInfo struct {
	Driver   driver.Driver
	Name     string
	Platform []specs.Platform
	Err      error
}

func filterAvailableDrivers(drivers []DriverInfo) ([]DriverInfo, error) {
	out := make([]DriverInfo, 0, len(drivers))
	err := errors.Errorf("no drivers found")
	for _, di := range drivers {
		if di.Err == nil && di.Driver != nil {
			out = append(out, di)
		}
		if di.Err != nil {
			err = di.Err
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	return nil, err
}

type driverPair struct {
	driverIndex int
	platforms   []specs.Platform
	so          *client.SolveOpt
}

func driverIndexes(m map[string][]driverPair) []int {
	out := make([]int, 0, len(m))
	visited := map[int]struct{}{}
	for _, dp := range m {
		for _, d := range dp {
			if _, ok := visited[d.driverIndex]; ok {
				continue
			}
			visited[d.driverIndex] = struct{}{}
			out = append(out, d.driverIndex)
		}
	}
	return out
}

func allIndexes(l int) []int {
	out := make([]int, 0, l)
	for i := 0; i < l; i++ {
		out = append(out, i)
	}
	return out
}

func ensureBooted(ctx context.Context, drivers []DriverInfo, idxs []int, pw progress.Writer) (map[string]map[string]*client.Client, error) {
	lock := sync.Mutex{}
	clients := map[string]map[string]*client.Client{} // [driverName][chosenNodeName]
	eg, ctx := errgroup.WithContext(ctx)

	for _, i := range idxs {
		lock.Lock()
		clients[drivers[i].Name] = map[string]*client.Client{}
		lock.Unlock()
		func(i int) {
			eg.Go(func() error {
				builderClients, err := driver.Boot(ctx, drivers[i].Driver, pw)
				if err != nil {
					return err
				}
				lock.Lock()
				clients[drivers[i].Name][builderClients.ChosenNode.NodeName] = builderClients.ChosenNode.BuildKitClient
				lock.Unlock()
				return nil
			})
		}(i)
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return clients, nil
}

func splitToDriverPairs(availablePlatforms map[string]int, opt map[string]Options) map[string][]driverPair {
	m := map[string][]driverPair{}
	for k, opt := range opt {
		mm := map[int][]specs.Platform{}
		for _, p := range opt.Platforms {
			k := platforms.Format(p)
			idx := availablePlatforms[k] // default 0
			pp := mm[idx]
			pp = append(pp, p)
			mm[idx] = pp
		}
		dps := make([]driverPair, 0, 2)
		for idx, pp := range mm {
			dps = append(dps, driverPair{driverIndex: idx, platforms: pp})
		}
		m[k] = dps
	}
	return m
}

func resolveDrivers(ctx context.Context, drivers []DriverInfo, opt map[string]Options, pw progress.Writer) (map[string][]driverPair, map[string]map[string]*client.Client, error) {
	availablePlatforms := map[string]int{}
	for i, d := range drivers {
		for _, p := range d.Platform {
			availablePlatforms[platforms.Format(p)] = i
		}
	}

	undetectedPlatform := false
	allPlatforms := map[string]int{}
	for _, opt := range opt {
		for _, p := range opt.Platforms {
			k := platforms.Format(p)
			allPlatforms[k] = -1
			if _, ok := availablePlatforms[k]; !ok {
				undetectedPlatform = true
			}
		}
	}

	// fast path
	if len(drivers) == 1 || len(allPlatforms) == 0 {
		m := map[string][]driverPair{}
		for k, opt := range opt {
			m[k] = []driverPair{{driverIndex: 0, platforms: opt.Platforms}}
		}
		clients, err := ensureBooted(ctx, drivers, driverIndexes(m), pw)
		if err != nil {
			return nil, nil, err
		}
		return m, clients, nil
	}

	// map based on existing platforms
	if !undetectedPlatform {
		m := splitToDriverPairs(availablePlatforms, opt)
		clients, err := ensureBooted(ctx, drivers, driverIndexes(m), pw)
		if err != nil {
			return nil, nil, err
		}
		return m, clients, nil
	}

	// boot all drivers in k
	clients, err := ensureBooted(ctx, drivers, allIndexes(len(drivers)), pw)
	if err != nil {
		return nil, nil, err
	}

	eg, ctx := errgroup.WithContext(ctx)
	workers := make([][]*client.WorkerInfo, len(clients))

	i := 0
	for driverName := range clients {
		for nodeName, c := range clients[driverName] {
			if c == nil {
				continue
			}
			func(nodeName string, i int) {
				eg.Go(func() error {
					ww, err := clients[driverName][nodeName].ListWorkers(ctx)
					if err != nil {
						return errors.Wrap(err, "listing workers")
					}
					workers[i] = ww
					return nil
				})
			}(nodeName, i)
			i++
		}
	}

	if err := eg.Wait(); err != nil {
		return nil, nil, err
	}

	for i, ww := range workers {
		for _, w := range ww {
			for _, p := range w.Platforms {
				p = platforms.Normalize(p)
				ps := platforms.Format(p)

				if _, ok := availablePlatforms[ps]; !ok {
					availablePlatforms[ps] = i
				}
			}
		}
	}

	return splitToDriverPairs(availablePlatforms, opt), clients, nil
}

func toRepoOnly(in string) (string, error) {
	m := map[string]struct{}{}
	p := strings.Split(in, ",")
	for _, pp := range p {
		n, err := reference.ParseNormalizedNamed(pp)
		if err != nil {
			return "", err
		}
		m[n.Name()] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ","), nil
}

// TODO - this could use some optimization...
func toSolveOpt(ctx context.Context, d driver.Driver, multiDriver bool, opt Options, dl dockerLoadCallback) (solveOpt *client.SolveOpt, release func(), err error) {
	defers := make([]func(), 0, 2)
	releaseF := func() {
		for _, f := range defers {
			f()
		}
	}

	defer func() {
		if err != nil {
			releaseF()
		}
	}()

	if opt.ImageIDFile != "" {
		if multiDriver || len(opt.Platforms) != 0 {
			return nil, nil, errors.Errorf("image ID file cannot be specified when building for multiple platforms")
		}
		// Avoid leaving a stale file if we eventually fail
		if err := os.Remove(opt.ImageIDFile); err != nil && !os.IsNotExist(err) {
			return nil, nil, errors.Wrap(err, "removing image ID file")
		}
	}

	// inline cache from build arg
	if v, ok := opt.BuildArgs["BUILDKIT_INLINE_CACHE"]; ok {
		if v, _ := strconv.ParseBool(v); v {
			opt.CacheTo = append(opt.CacheTo, client.CacheOptionsEntry{
				Type:  "inline",
				Attrs: map[string]string{},
			})
		}
	}

	for _, e := range opt.CacheTo {
		if e.Type != "inline" && !d.Features()[driver.CacheExport] {
			return nil, nil, notSupported(d, driver.CacheExport)
		}
	}

	so := client.SolveOpt{
		Frontend:            "dockerfile.v0",
		FrontendAttrs:       map[string]string{},
		LocalDirs:           map[string]string{},
		CacheExports:        opt.CacheTo,
		CacheImports:        opt.CacheFrom,
		AllowedEntitlements: opt.Allow,
	}

	if opt.FrontendImage != "" {
		so.Frontend = "gateway.v0"
		so.FrontendAttrs["source"] = opt.FrontendImage
	}

	if multiDriver {
		// force creation of manifest list
		so.FrontendAttrs["multi-platform"] = "true"
	}

	switch len(opt.Exports) {
	case 1:
		// valid
	case 0:
		// Shouln't happen - higher level constructs should create 1+ exports
		return nil, nil, errors.Errorf("zero outputs currently unsupported")
		// TODO - alternative would be
		// opt.Exports = []client.ExportEntry{{Type: "image", Attrs: map[string]string{}}}
	default:
		return nil, nil, errors.Errorf("multiple outputs currently unsupported")
	}

	// Update any generic "runtime" exports to be runtime specific now:
	driverFeatures := d.Features()
	for i, e := range opt.Exports {
		switch e.Type {
		case "runtime":
			// TODO - this is a bit messy - can we just leverage "image" Type and "do the right thing" instead of
			// having this alternate "runtime" type that's ~inconsistent?
			if driverFeatures[driver.ContainerdExporter] {
				opt.Exports[i].Type = "image"
			} else if driverFeatures[driver.DockerExporter] {
				opt.Exports[i].Type = "docker"
			} else {
				// TODO should we allow building without load or push, perhaps a new "nil" or equivalent output type?
				return nil, nil, errors.Errorf("loading image into cluster runtime not supported by this builder, please specify --push or a client local output: --output=type=local,dest=. --output=type=tar,dest=out.tar ")
			}
		}
	}

	// fill in image exporter names from tags
	if len(opt.Tags) > 0 {
		tags := make([]string, len(opt.Tags))
		for i, tag := range opt.Tags {
			ref, err := reference.Parse(tag)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "invalid tag %q", tag)
			}
			tags[i] = ref.String()
		}
		for i, e := range opt.Exports {
			switch e.Type {
			case "image", "oci", "docker":
				opt.Exports[i].Attrs["name"] = strings.Join(tags, ",")
			}
		}
	} else {
		for _, e := range opt.Exports {
			if e.Type == "image" && e.Attrs["name"] == "" && e.Attrs["push"] != "" {
				if ok, _ := strconv.ParseBool(e.Attrs["push"]); ok {
					return nil, nil, errors.Errorf("tag is needed when pushing to registry")
				}
			}
		}
	}

	// set up exporters
	for i, e := range opt.Exports {
		pushing := false
		if p, ok := e.Attrs["push"]; ok {
			pushing, _ = strconv.ParseBool(p)
		}
		if (e.Type == "local" || e.Type == "tar") && opt.ImageIDFile != "" {
			return nil, nil, errors.Errorf("local and tar exporters are incompatible with image ID file")
		}

		// TODO - do more research on what this really means and if it's wired up correctly
		if e.Type == "oci" && !driverFeatures[driver.OCIExporter] {
			return nil, nil, notSupported(d, driver.OCIExporter)
		}
		if e.Type == "docker" {
			if !driverFeatures[driver.DockerExporter] {
				return nil, nil, notSupported(d, driver.DockerExporter)
			}
			// If the runtime is docker and we're not in
			// rootless mode, then we will have mounted
			// the docker.sock inside the buildkit pods, and
			// can use the exec trick with
			//   buildctl --addr unix://run/docker.sock
			// to proxy to the API endpoint and perform an image load
			// This isn't as efficient as tagging/exposing the images
			// directly within containerd, but will suffice during
			// the transition period while many people still run Docker
			// as their container runtime for kubernetes

			// If an Output (file) is already specified, let it pass through
			if e.Output == nil {
				w, cancel, err := dl("")
				if err != nil {
					return nil, nil, err
				}
				defers = append(defers, cancel)
				opt.Exports[i].Output = wrapWriteCloser(w)
			}
		}
		if e.Type == "containerd" { // TODO - should this just be dropped in favor of "image"?

			// TODO should this just be wired up as "image" instead?
			return nil, nil, notSupported(d, driver.ContainerdExporter)
			// TODO implement this scenario
		}
		if e.Type == "image" && !pushing {
			if !driverFeatures[driver.ContainerdExporter] {
				// TODO - this could use a little refinement - if the user specifies `--output=image` it would be nice
				// to auto-wire this to handle both runtimes (docker and containerd)
				return nil, nil, notSupported(d, driver.ContainerdExporter)
			}
			// TODO - is there a better layer to optimize this part of the flow at?
			multiNode := false
			builders, err := d.List(ctx) // TODO - we're doing this multiple times - clean this up by passing in some more context to toSolveOpt
			if err != nil {
				return nil, nil, err
			}
			if len(builders) > 1 { // TODO - this is messy and ~wrong - should just be getting nodes for a given builder here
				multiNode = true
			} else if len(builders) == 1 {
				if len(builders[0].Nodes) > 1 {
					multiNode = true
				}
			}

			// TODO - figure out how to wire this up so the exporter output type is oci instead of image

			// If an Output (file) is already specified, let it pass through
			if e.Output == nil && multiNode {
				// TODO - Explore if there's a model to avoid having to transfer to the builder node
				opt.Exports[i].Type = "oci" // TODO - this most likely means the image isn't saved locally too
				w, cancel, err := dl("")
				if err != nil {
					return nil, nil, err
				}
				defers = append(defers, cancel)
				opt.Exports[i].Output = wrapWriteCloser(w)
			}
		}

		// TODO - image + runtime==docker, use the same proxy as above
		/* Nuke this once everything's refactored
		if e.Type == "image" && isDefaultMobyDriver { // TODO kube driver use-case coverage here...
			opt.Exports[i].Type = "moby"
			if e.Attrs["push"] != "" {
				if ok, _ := strconv.ParseBool(e.Attrs["push"]); ok {
					return nil, nil, errors.Errorf("auto-push is currently not implemented for docker driver")
				}
			}
		}
		*/

		// TODO If we're multi-node and "image" is specified, should we replicate it on all nodes?
	}

	so.Exports = opt.Exports
	so.Session = opt.Session

	releaseLoad, err := LoadInputs(opt.Inputs, &so)
	if err != nil {
		return nil, nil, err
	}
	defers = append(defers, releaseLoad)

	if opt.Pull {
		so.FrontendAttrs["image-resolve-mode"] = "pull"
	}
	if opt.Target != "" {
		so.FrontendAttrs["target"] = opt.Target
	}
	if opt.NoCache {
		so.FrontendAttrs["no-cache"] = ""
	}
	for k, v := range opt.BuildArgs {
		so.FrontendAttrs["build-arg:"+k] = v
	}
	for k, v := range opt.Labels {
		so.FrontendAttrs["label:"+k] = v
	}

	// set platforms
	if len(opt.Platforms) != 0 {
		pp := make([]string, len(opt.Platforms))
		for i, p := range opt.Platforms {
			pp[i] = platforms.Format(p)
		}
		if len(pp) > 1 && !d.Features()[driver.MultiPlatform] {
			return nil, nil, notSupported(d, driver.MultiPlatform)
		}
		so.FrontendAttrs["platform"] = strings.Join(pp, ",")
	}

	// setup networkmode
	switch opt.NetworkMode {
	case "host", "none":
		so.FrontendAttrs["force-network-mode"] = opt.NetworkMode
		so.AllowedEntitlements = append(so.AllowedEntitlements, entitlements.EntitlementNetworkHost)
	case "", "default":
	default:
		return nil, nil, errors.Errorf("network mode %q not supported by buildkit", opt.NetworkMode)
	}

	// setup extrahosts
	extraHosts, err := toBuildkitExtraHosts(opt.ExtraHosts)
	if err != nil {
		return nil, nil, err
	}
	so.FrontendAttrs["add-hosts"] = extraHosts

	return &so, releaseF, nil
}

func Build(ctx context.Context, drivers []DriverInfo, opt map[string]Options, kubeClientConfig clientcmd.ClientConfig, registrySecretName string, pw progress.Writer) (resp map[string]*client.SolveResponse, err error) {
	if len(drivers) == 0 {
		return nil, errors.Errorf("driver required for build")
	}

	drivers, err = filterAvailableDrivers(drivers)
	if err != nil {
		return nil, errors.Wrapf(err, "no valid drivers found")
	}
	m, clients, err := resolveDrivers(ctx, drivers, opt, pw)
	if err != nil {
		close(pw.Status())
		<-pw.Done()
		return nil, err
	}

	defers := make([]func(), 0, 2)
	defer func() {
		if err != nil {
			for _, f := range defers {
				f()
			}
		}
	}()

	var auth imagetools.Auth

	mw := progress.NewMultiWriter(pw)
	eg, ctx := errgroup.WithContext(ctx)
	for k, opt := range opt {
		multiDriver := len(m[k]) > 1
		for i, dp := range m[k] {
			d := drivers[dp.driverIndex].Driver
			driverName := drivers[dp.driverIndex].Name
			opt.Platforms = dp.platforms

			// TODO - this is also messy and wont work for multi-driver scenarios (no that it's possible yet...)
			if auth == nil {
				auth = d.GetAuthWrapper(registrySecretName)
				opt.Session = append(opt.Session, d.GetAuthProvider(registrySecretName, os.Stderr))
			}
			so, release, err := toSolveOpt(ctx, d, multiDriver, opt, func(arg string) (io.WriteCloser, func(), error) {
				// Set up loader based on first found type (only 1 supported)
				for _, entry := range opt.Exports {
					if entry.Type == "docker" {
						return newDockerLoader(ctx, d, kubeClientConfig, driverName, mw)
					} else if entry.Type == "oci" {
						return newContainerdLoader(ctx, d, kubeClientConfig, driverName, mw)
					}
				}
				// TODO - Push scenario?  (or is this a "not reached" scenario now?)
				return nil, nil, fmt.Errorf("raw push scenario not yet supported")
			})
			if err != nil {
				return nil, err
			}
			defers = append(defers, release)
			m[k][i].so = so
		}
	}

	resp = map[string]*client.SolveResponse{}
	var respMu sync.Mutex

	multiTarget := len(opt) > 1

	for k, opt := range opt {
		err := func(k string) error {
			opt := opt
			dps := m[k]
			multiDriver := len(m[k]) > 1

			res := make([]*client.SolveResponse, len(dps))
			wg := &sync.WaitGroup{}
			wg.Add(len(dps))

			var pushNames string

			eg.Go(func() error {
				pw := mw.WithPrefix("default", false)
				defer close(pw.Status())
				wg.Wait()
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				respMu.Lock()
				resp[k] = res[0]
				respMu.Unlock()
				if len(res) == 1 {
					if opt.ImageIDFile != "" {
						return ioutil.WriteFile(opt.ImageIDFile, []byte(res[0].ExporterResponse["containerimage.digest"]), 0644)
					}
					return nil
				}

				if pushNames != "" {
					progress.Write(pw, fmt.Sprintf("merging manifest list %s", pushNames), func() error {
						descs := make([]specs.Descriptor, 0, len(res))

						for _, r := range res {
							s, ok := r.ExporterResponse["containerimage.digest"]
							if ok {
								descs = append(descs, specs.Descriptor{
									Digest:    digest.Digest(s),
									MediaType: images.MediaTypeDockerSchema2ManifestList,
									Size:      -1,
								})
							}
						}
						if len(descs) > 0 {
							itpull := imagetools.New(imagetools.Opt{
								Auth: auth,
							})

							names := strings.Split(pushNames, ",")
							dt, desc, err := itpull.Combine(ctx, names[0], descs)
							if err != nil {
								return err
							}
							if opt.ImageIDFile != "" {
								return ioutil.WriteFile(opt.ImageIDFile, []byte(desc.Digest), 0644)
							}
							itpush := imagetools.New(imagetools.Opt{
								Auth: auth,
							})

							for _, n := range names {
								nn, err := reference.ParseNormalizedNamed(n)
								if err != nil {
									return err
								}
								if err := itpush.Push(ctx, nn, desc, dt); err != nil {
									return err
								}
							}

							respMu.Lock()
							resp[k] = &client.SolveResponse{
								ExporterResponse: map[string]string{
									"containerimage.digest": desc.Digest.String(),
								},
							}
							respMu.Unlock()
						}
						return nil
					})
				}
				return nil
			})

			for i, dp := range dps {
				so := *dp.so

				if multiDriver {
					for i, e := range so.Exports {
						switch e.Type {
						case "oci", "tar":
							return errors.Errorf("%s for multi-node builds currently not supported", e.Type)
						case "image":
							if pushNames == "" && e.Attrs["push"] != "" {
								if ok, _ := strconv.ParseBool(e.Attrs["push"]); ok {
									pushNames = e.Attrs["name"]
									if pushNames == "" {
										return errors.Errorf("tag is needed when pushing to registry")
									}
									names, err := toRepoOnly(e.Attrs["name"])
									if err != nil {
										return err
									}
									e.Attrs["name"] = names
									e.Attrs["push-by-digest"] = "true"
									so.Exports[i].Attrs = e.Attrs
								}
							}
						}
					}
				}

				func(i int, dp driverPair, so client.SolveOpt) {
					pw := mw.WithPrefix(k, multiTarget)

					// TODO this is a little mess - could use some refactoring
					var c *client.Client
					for _, client := range clients[drivers[dp.driverIndex].Name] {
						c = client
						break
					}

					var statusCh chan *client.SolveStatus
					if pw != nil {
						pw = progress.ResetTime(pw)
						statusCh = pw.Status()
						eg.Go(func() error {
							<-pw.Done()
							return pw.Err()
						})
					}

					eg.Go(func() error {
						defer wg.Done()
						rr, err := c.Solve(ctx, nil, so, statusCh)
						if err != nil {
							// Try to give a slightly more helpful error message if the use
							// hasn't wired up a kubernetes secret for push/pull properly
							if strings.Contains(strings.ToLower(err.Error()), "401 unauthorized") {
								msg := drivers[dp.driverIndex].Driver.GetAuthHintMessage()
								return errors.Wrap(err, msg)
							}
							return err
						}
						res[i] = rr
						return nil
					})

				}(i, dp, so)
			}

			return nil
		}(k)
		if err != nil {
			return nil, err
		}
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return resp, nil
}

func createTempDockerfile(r io.Reader) (string, error) {
	dir, err := ioutil.TempDir("", "dockerfile")
	if err != nil {
		return "", err
	}
	f, err := os.Create(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return dir, err
}

func LoadInputs(inp Inputs, target *client.SolveOpt) (func(), error) {
	if inp.ContextPath == "" {
		return nil, errors.New("please specify build context (e.g. \".\" for the current directory)")
	}

	// TODO: handle stdin, symlinks, remote contexts, check files exist

	var (
		err              error
		dockerfileReader io.Reader
		dockerfileDir    string
		dockerfileName   = inp.DockerfilePath
		toRemove         []string
	)

	switch {
	case inp.ContextPath == "-":
		if inp.DockerfilePath == "-" {
			return nil, errStdinConflict
		}

		buf := bufio.NewReader(inp.InStream)
		magic, err := buf.Peek(archiveHeaderSize * 2)
		if err != nil && err != io.EOF {
			return nil, errors.Wrap(err, "failed to peek context header from STDIN")
		}

		if isArchive(magic) {
			// stdin is context
			up := uploadprovider.New()
			target.FrontendAttrs["context"] = up.Add(buf)
			target.Session = append(target.Session, up)
		} else {
			if inp.DockerfilePath != "" {
				return nil, errDockerfileConflict
			}
			// stdin is dockerfile
			dockerfileReader = buf
			inp.ContextPath, _ = ioutil.TempDir("", "empty-dir")
			toRemove = append(toRemove, inp.ContextPath)
			target.LocalDirs["context"] = inp.ContextPath
		}

	case isLocalDir(inp.ContextPath):
		target.LocalDirs["context"] = inp.ContextPath
		switch inp.DockerfilePath {
		case "-":
			dockerfileReader = inp.InStream
		case "":
			dockerfileDir = inp.ContextPath
		default:
			dockerfileDir = filepath.Dir(inp.DockerfilePath)
			dockerfileName = filepath.Base(inp.DockerfilePath)
		}

	case urlutil.IsGitURL(inp.ContextPath), urlutil.IsURL(inp.ContextPath):
		if inp.DockerfilePath == "-" {
			return nil, errors.Errorf("Dockerfile from stdin is not supported with remote contexts")
		}
		target.FrontendAttrs["context"] = inp.ContextPath
	default:
		return nil, errors.Errorf("unable to prepare context: path %q not found", inp.ContextPath)
	}

	if dockerfileReader != nil {
		dockerfileDir, err = createTempDockerfile(dockerfileReader)
		if err != nil {
			return nil, err
		}
		toRemove = append(toRemove, dockerfileDir)
		dockerfileName = "Dockerfile"
	}

	if dockerfileName == "" {
		dockerfileName = "Dockerfile"
	}
	target.FrontendAttrs["filename"] = dockerfileName

	if dockerfileDir != "" {
		target.LocalDirs["dockerfile"] = dockerfileDir
	}

	release := func() {
		for _, dir := range toRemove {
			os.RemoveAll(dir)
		}
	}
	return release, nil
}

func notSupported(d driver.Driver, f driver.Feature) error {
	return errors.Errorf("%s feature is currently not supported for %s driver. Please switch to a different driver (eg. \"kubectl buildkit create --use\")", f, d.Factory().Name())
}

type dockerLoadCallback func(name string) (io.WriteCloser, func(), error)

func newDockerLoader(ctx context.Context, d driver.Driver, kubeClientConfig clientcmd.ClientConfig, builderName string, mw *progress.MultiWriter) (io.WriteCloser, func(), error) {
	nodeNames := []string{}
	// TODO this isn't quite right - we need a better "list only pods from one instance" func
	builders, err := d.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, builder := range builders {
		if builder.Name != builderName {
			continue
		}
		for _, node := range builder.Nodes {
			nodeNames = append(nodeNames, node.Name)
		}
	}
	if len(nodeNames) == 0 {
		return nil, nil, fmt.Errorf("no builders found for %s", builderName)
	}
	// TODO revamp this flow to return a list of pods

	readers := make([]*io.PipeReader, len(nodeNames))
	writers := make([]io.Writer, len(nodeNames))
	pws := make([]*io.PipeWriter, len(nodeNames))
	names := make([]string, len(nodeNames))
	clients := make([]*dockerclient.Client, len(nodeNames))
	started := make([]chan struct{}, len(nodeNames))
	for i := range nodeNames {
		nodeName := nodeNames[i]
		tr := &http.Transport{
			// This is necessary as the Exec "upgrade" of the underlying connection
			// seems to confuse the Golang Transport idle connection caching mechanism
			// and multiple pods wind up sharing a connection that "appears" idle.
			//DisableKeepAlives: true,

			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				conn, err := d.RuntimeSockProxy(ctx, nodeName)
				if err != nil {
					return nil, fmt.Errorf("failed to set up docker runtime proxies through pod %s: %w", nodeName, err)
				}
				return conn, nil
			},
		}

		c, err := dockerclient.NewClientWithOpts(
			dockerclient.WithAPIVersionNegotiation(),
			dockerclient.WithHTTPClient(&http.Client{Transport: tr}),
			dockerclient.WithHost("http://"+nodeName), // TODO nuke
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to set up docker client via proxy: %w", err)
		}

		pr, pw := io.Pipe()
		readers[i] = pr
		writers[i] = pw
		pws[i] = pw
		clients[i] = c
		names[i] = nodeName
		started[i] = make(chan struct{})
	}

	w := &waitingWriter{
		Writer: io.MultiWriter(writers...),
		pws:    pws,
		names:  names,
		f: func(i int) {
			c := clients[i]
			resp, err := c.ImageLoad(ctx, readers[i], false)
			if err != nil {
				_ = readers[i].CloseWithError(err)
				close(started[i])
				return
			}

			prog := mw.WithPrefix("", false)

			close(started[i])

			progress.FromReader(prog, fmt.Sprintf("loading image to docker runtime via pod %s", names[i]), resp.Body)
			resp.Body.Close()
		},
		started: started,
	}

	return w, func() {
		for _, pr := range readers {
			pr.Close()
		}
	}, nil
}

type waitingWriter struct {
	io.Writer
	pws   []*io.PipeWriter
	names []string
	f     func(i int)
	once  sync.Once
	// mu      sync.Mutex
	// err     error
	started []chan struct{}
}

func (w *waitingWriter) Write(dt []byte) (int, error) {
	w.once.Do(func() {
		for i := range w.pws {
			go w.f(i)
		}
	})

	return w.Writer.Write(dt)
}

func (w *waitingWriter) Close() error {
	var err error
	for _, pw := range w.pws {
		err2 := pw.Close()
		if err2 != nil {
			err = err2
		}
	}
	for i := range w.pws {
		<-w.started[i]
	}
	return err
}

func newContainerdLoader(ctx context.Context, d driver.Driver, kubeClientConfig clientcmd.ClientConfig, builderName string, mw *progress.MultiWriter) (io.WriteCloser, func(), error) {
	nodeNames := []string{}
	// TODO this isn't quite right - we need a better "list only pods from one instance" func
	// TODO revamp this flow to return a list of pods

	builders, err := d.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, builder := range builders {
		if builder.Name != builderName {
			continue
		}
		for _, node := range builder.Nodes {
			// For containerd, we never have to load on the node where it was built
			// TODO - we may want to filter the source node, but when we switch the output.Type to "oci" we need to load it everywhere anyway
			nodeNames = append(nodeNames, node.Name)
		}
	}

	readers := make([]*io.PipeReader, len(nodeNames))
	writers := make([]io.Writer, len(nodeNames))
	pws := make([]*io.PipeWriter, len(nodeNames))
	names := make([]string, len(nodeNames))
	clients := make([]*containerd.Client, len(nodeNames))
	started := make([]chan struct{}, len(nodeNames))
	for i := range nodeNames {
		nodeName := nodeNames[i]
		c, err := containerd.New("/run/containerd/containerd.sock",

			// TODO - plumb the containerd namespace through so this isn't hard-coded
			containerd.WithDefaultNamespace("k8s.io"),
			containerd.WithDialOpts([]grpc.DialOption{
				grpc.WithContextDialer(
					func(ctx context.Context, _ string) (net.Conn, error) {
						conn, err := d.RuntimeSockProxy(ctx, nodeName)
						if err != nil {
							return nil, fmt.Errorf("failed to set up containerd runtime proxies through pod %s: %w", nodeName, err)
						}
						return conn, nil
					},
				),
				grpc.WithTransportCredentials(insecure.NewCredentials()), //  Nested connection on an existing secure transport
			}),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to set up containerd client via proxy: %w", err)
		}

		pr, pw := io.Pipe()
		readers[i] = pr
		writers[i] = pw
		pws[i] = pw
		clients[i] = c
		names[i] = nodeName
		started[i] = make(chan struct{})
	}

	w := &waitingWriter{
		Writer: io.MultiWriter(writers...),
		pws:    pws,
		names:  names,
		f: func(i int) {
			//prog := mw.WithPrefix(names[i], false)

			c := clients[i]

			// TODO - is there some way to wire up fine-grain progress reporting stream here?
			// Adding calls to progress.Write(mw, "importing image", func() error { return nil }) seem to cause hangs...
			imgs, err := c.Import(ctx, readers[i],

				// TODO - might need to pass through the tagged name with the tag part stripped off
				//containerd.WithImageRefTranslator(archive.FilterRefPrefix(prefix),

				// TODO - might need this too..
				//containerd.WithIndexName(tag),

				// TODO - is this necessary or implicit?  (try without after it's working...)
				containerd.WithAllPlatforms(false),
			)
			if err != nil {
				_ = readers[i].CloseWithError(err)
				close(started[i])
				return
			}

			for _, img := range imgs {
				image := containerd.NewImage(c, img)

				// TODO: Show unpack status
				//progress.Write(prog, fmt.Sprintf("unpacking %s (%s)", img.Name, img.Target.Digest), func() error { return nil })
				err = image.Unpack(ctx, "") // Empty snapshotter is default
				if err != nil {
					_ = readers[i].CloseWithError(err)
					close(started[i])
					return
				}
			}
			close(started[i])
		},
		started: started,
	}

	return w, func() {
		for _, pr := range readers {
			pr.Close()
		}
	}, nil
}
