# Release Process

The release CI is defined by
[../.github/workflows/release.yaml](../.github/workflows/release.yaml) and is
triggered by pushing a git tag to the main repo.


When we're ready to create a new release we follow this process:

* Create a new git tag locally mapping to `upstream/main` HEAD (or the desired commit) - e.g. `git tag v0.1.2 HEAD`
* Push that tag to trigger CI to run the release flow - `git push upstream v0.1.2` - this will trigger the CI flow, and create a draft release
* Inspect the draft release at https://github.com/vmware-tanzu/buildkit-cli-for-kubectl/releases (only maintainers will be able to see draft releases)
* Add release notes to the release showing the relevant changes since the last release
```
git log --oneline --no-merges --no-decorate v0.1.1..v0.1.2
```
* Remove "noise" commits that won't be relevant to most users (e.g., vendoring bumps, doc changes, etc.)
* Rephrase commit messages where necessary to better summarize the fix for readability
* Have another maintainer review the release
* Publish the release


## When you're making release CI changes

To test a change to the release pipeline, a maintainer will have to help.  Push
the proposed change to a temporary branch on the main tree.  Once that proposed
change is made, then push a temporary pre-release tag to the main repo.  e.g.,
`v0.1.1-test1` to trigger the release pipeline.  Inspect the logs, and look at
the draft release that was produced (download binaries, try them out, etc.) Once
satisfied that the release change is ready, post a PR to the repo from that
branch, delete the temporary test tag, delete the draft release, merge the
change, and perform the normal release process described above.