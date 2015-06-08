pre-commit-go
=============

`pre-commit-go` runs multiple checks on a Go project *on commit* via
`pre-commit` git hook and *on push* via `pre-push` git hook. It's designed to be
simple and *fast*. Everything is run concurrently. It also includes linting
support and Continuous Integration service (CI) support. No check ever modify
any file.


Usage
-----

### Setup

    go get github.com/maruel/pre-commit-go

Use built-in help to list all options and commands:

    pre-commit-go help

Print the enabled checks:

    pre-commit-go info

Run from within a git checkout inside `$GOPATH`. This installs the git hooks
within `.git/hooks` and runs the checks in mode `pre-push` by default:

    pre-commit-go


### Bypassing hook

It may become necessary to commit something known to be broken. To bypass the
pre-commit hook, use:

    git commit --no-verify

or shorthand `-n`


Modes
-----

`pre-commit-go` runs on 4 different modes:

  * `pre-commit`: it's the fast tests, e.g. running go test -short
  * `pre-push`: the slower checks but still bearable for interactive usage.
  * `continuous-integration`: runs every checks, including the race detector.
  * `lint`: are off-by-default checks.

Default checks are meant to be sensible but it can be configured by adding a
[pre-commit-go.yml](https://github.com/maruel/pre-commit-go/blob/master/pre-commit-go.yml)
in your git checkout root directory. If you don't want to pollute your git
repository with yml files, put it at `.git/pre-commit-go.yml`.


Checks
------

Checks documentation:
[![GoDoc](https://godoc.org/github.com/maruel/pre-commit-go/checks/definitions?status.svg)](https://godoc.org/github.com/maruel/pre-commit-go/checks/definitions)


### Native checks

  * [go build](https://golang.org/pkg/go/build/) all directories with .go files
    found
  * [go test -race](https://golang.org/pkg/testing/) by default with [race
    detector](https://blog.golang.org/race-detector)
  * [go test -cover](https://golang.org/pkg/testing/) with
    [coverage](https://blog.golang.org/cover)
  * [gofmt](https://golang.org/cmd/gofmt/), especially for the -s flag.
  * [goimports](https://golang.org/x/tools/cmd/goimports)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [golint](https://github.com/golang/lint)
  * [govet (go tool vet)](https://golang.org/x/tools/cmd/vet)


### Custom check

A custom check can be defined by adding a `custom` check in one of the modes.
Here's an example running `sample-pre-commit-go-custom-check` on the tree in
mode continuous-integration:

```yaml
modes:
  continous-integration:
    checks:
    - check_type: custom
      display_name: sample-pre-commit-go-custom-check
      description: runs the check sample-pre-commit-go-custom-check on this repository
      command:
      - sample-pre-commit-go-custom-check
      - check
      check_exit_code: true
      prerequisites:
      - help_command:
        - sample-pre-commit-go-custom-check
        - -help
        expected_exit_code: 2
        url: github.com/maruel/pre-commit-go/samples/sample-pre-commit-go-custom-check
```


Continous integration support
-----------------------------

### Overview

Here's a sample of CI systems that can be used. Obviously, use 1, not 3 but none
is perfect:

  * Travis: [![Build Status](https://travis-ci.org/maruel/pre-commit-go.svg?branch=master)](https://travis-ci.org/maruel/pre-commit-go)
    * Lets you to run tests against multiple versions of Go, even against tip!
    * The free version is the slowest of all 3.
    * Can't ssh in.
  * CircleCI: [![Build Status](https://circleci.com/gh/maruel/pre-commit-go.svg?style=shield&circle-token=:circle-token)](https://circleci.com/gh/maruel/pre-commit-go)
    * Lets you ssh into the bot for 30 minutes to debug a failure!
    * Uses build output caching which can get in the way.
    * Uses symlinks in ~/.go_project/src which can get in the way.
    * Can't specify Go version.
  * Drone: [![Build Status](https://drone.io/github.com/maruel/pre-commit-go/status.png)](https://drone.io/github.com/maruel/pre-commit-go/latest)
    * Uses a git template which gets in the way if you ever run git in a smoke
      test.
    * Can't specify Go version.
    * Can't ssh in.

Code coverage can be used via one of the systems above via Coveralls:
[![Coverage Status](https://coveralls.io/repos/maruel/pre-commit-go/badge.svg?branch=master)](https://coveralls.io/r/maruel/pre-commit-go?branch=master)


### travis-ci.org

Post push CI (continuous integration) works with Travis. This
runs the checks on pull requests automatically! This also works with
github organizations.

   1. Visit https://travis-ci.org and connect your github account (or whatever
      git host provider) to Travis. Enable your repository.
   2. Copy
      [sample/travis.yml](https://github.com/maruel/pre-commit-go/blob/master/sample/travis.yml)
      as `.travis.yml` in your repository and push it.


### drone.io

   1. Visit https://drone.io and connect your github account (or whatever git
      host provider) to Drone. Enable your repository.
   2. At page "Setup your Build Script", put:

    go get -d -t ./...
    go get github.com/maruel/pre-commit-go
    pre-commit-go


### circleci.com


   1. Visit https://circleci.com and enable your repository.
   2. Click 'Project Settings', 'Dependency Commands' and type:

    go get github.com/maruel/pre-commit-go

   3. Click 'Test Commands' and type:

    pre-commit-go


### coveralls.io

Integrate with travis-ci first, then visit https://coveralls.io and enable your
repository.

[goveralls](https://github.com/mattn/goveralls) doesn't detect drone.io job id
automatically yet. Please send a Pull Request to fix this if this is your
preferred setup.


### Fine tuning what is tested.

When running under CI, you'll want it to run more tests than run locally, in
particular things that take too much time for a user to test. You can configure
this with adding a pre-commit-go.yml file in your repository. You can also
enable running lint checks by default on your CI by enabling it explicitly:

    pre-commit-go installrun -M all
