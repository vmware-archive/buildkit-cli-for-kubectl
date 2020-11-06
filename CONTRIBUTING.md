# Contributing

Welcome to the BuildKit CLI for kubectl project!  We welcome contributions from everyone.  [Filing Issues](https://github.cm/vmware-tanzu/buildkit-cli-for-kubectl/issues/new) is great, submitting [Pull Requests](https://github.cm/vmware-tanzu/buildkit-cli-for-kubectl/pulls) are even better!  By participating in this project, you agree to abide by the [code of conduct](CODE-OF-CONDUCT.md).

## Setting up your Environment

[Install Go](https://golang.org/dl/) if you haven't already, and set up your GOPATH (e.g. something like `export GOPATH=${HOME}/go` )

Fork, then clone the repo:

    git clone git@github.com:your-username/buildkit-cli-for-kubectl.git $(GOPATH)/src/github.com/vmware-tanzu/buildkit-cli-for-kubectl

We use Go Modules to carry most dependencies, but you'll need a few additional tools installed on your system:

* GNU Make (e.g., `brew install make` for MacOS users)
* https://golangci-lint.run/usage/install/
* kubectl of course!

## Building Locally

In the main repo, you can quickly **build and install** the CLI plugin with

```
make build
sudo make install
```

To run the **unit tests**, run
```
make test
```

Assuming you have a valid kube configuration pointed at a cluster, you can run the **integration tests** with
```
make integration
```

To check your code for **lint/style consistency**, run
```
make lint
```

## Reporting Issues

A great way to contribute to the project is to send a detailed report when you run into problems.

Before you submit a new issue, please take a minute to search in the existing issues to see if someone else has already filed an issue to track the problem you ran into.  If so, you can "subscribe" to the issue and click on the smiley-face to leave a +1 (thumbs up) reaction to the opening comment so we have a rough gauge on how many people are hitting the same problem.  Please don't leave short "me too" comments in the issues as that clutters up the conversation.

If you weren't able to find an existing issue that matches, file a [new issue](https://github.cm/vmware-tanzu/buildkit-cli-for-kubectl/issues/new), and make sure to fill out the template with as much detail as you can.  This will help reduce the back-and-forth required for others to understand the specifics of your issue.

## Fixing Bugs

Bug fixes are always welcome!  We do our best to process them quickly.  If you're working on a fix for an existing issue, please add a comment on the issue that you're working on it so everyone watching the issue is aware.  When you post your [pull requests](https://github.cm/vmware-tanzu/buildkit-cli-for-kubectl/pulls) make sure to mention it `closes #<issue-number>` in the PR description or a comment on the PR so they get connected.

We strive to always improve our test coverage.  When submitting a bug fix, you should aim for ~100% unit-test coverage of the code you're touching.  When possible, you should also create an integration test case that captures the failure scenario you're fixing.

## Implementing new Features

If you've got an idea for a new feature or significant refactoring of the code, please submit a Feature Enhancement issue **before** you spend a lot of time working on your code.  This process enables the community to review your proposed changes to give early feedback.

New features should have good test coverage including both unit and integration coverage.

## Finding Something to Work on

If you want to help, but aren't sure what to work on, look for issues with a [help wanted](https://github.com/vmware-tanzu/buildkit-cli-for-kubectl/labels/help%20wanted) label that look interesting to you.  Put a comment on the issue if you want to pick it up.

## Testing

We use both unit and integration tests which are both implemented with `go test`.

Unit tests are written to exercise functions within a single go module.  Those tests should mock/stub any other dependencies, and must not require running against a live kubernetes environment.

Integration tests are designed to run "from the top" by exercising the CLI UX with specific command line flags, and inputs, running on a "real" kubernetes cluster, with a builder.  Integration tests live within [./integration/](./integration/)

When you submit a pull request, our CI system will exercise both of these types of tests.

## Connecting with other Contributors

We're just getting started.  For the moment, the main communication channel is through **XXX (insert link to google group here once created)**.  Over time we'll add more channels and update this doc accordingly.
