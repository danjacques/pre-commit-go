Continous Integration setup
===========================

## Overview

`pcg` sets itself in mode `run-hook continuous-integration` automatically when
run without arguments and the environment variable `CI=true` is set. It is set
on all popular hosted CI services.

Here's a sample of CI systems that can be used.  M-A did a review of hosted CI
services at
[maruel.net/post/ci-go-review/](https://maruel.net/post/ci-go-review/). See this
blog post for the pros and cons for each service. Obviously, use 1, not all but
none is perfect:

  - Travis: [![Build Status](https://travis-ci.org/maruel/pre-commit-go.svg?branch=master)](https://travis-ci.org/maruel/pre-commit-go)
  - CircleCI: [![Build Status](https://circleci.com/gh/maruel/pre-commit-go.svg?style=shield&circle-token=:circle-token)](https://circleci.com/gh/maruel/pre-commit-go)
  - Drone: [![Build Status](https://drone.io/github.com/maruel/pre-commit-go/status.png)](https://drone.io/github.com/maruel/pre-commit-go/latest)
  - Codeship: [![Build Status](https://codeship.com/projects/a932ed10-faa2-0132-33b9-1a34b2d0f857/status?branch=master)](https://codeship.com/projects/86965)

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
    go get github.com/maruel/pre-commit-go/cmd/pcg
    pcg


### circleci.com


   1. Visit https://circleci.com and enable your repository.
   2. Click 'Project Settings', 'Dependency Commands' and type:

    go get github.com/maruel/pre-commit-go/cmd/pcg

   3. Click 'Test Commands' and type:

    pcg


### coveralls.io

Integrate with travis-ci first, then visit https://coveralls.io and enable your
repository.

[goveralls](https://github.com/mattn/goveralls) doesn't detect drone.io job id
automatically yet. Please send a Pull Request to fix this if this is your
preferred setup.

To use coveralls.io, you must check-in a pre-commit-go.yml that has a `coverage`
check with `use_coveralls: true`.


### Fine tuning what is tested.

When running under CI, you'll want it to run more tests than run locally, in
particular things that take too much time for a user to test. You can configure
this with adding a pre-commit-go.yml file in your repository. You can also
enable running lint checks by default on your CI by enabling it explicitly:

    pcg installrun -m all -a
