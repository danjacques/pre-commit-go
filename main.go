// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Globals

// TODO(maruel): Reimplement this in go instead of processing it in bash.
var preCommitHook = []byte(`#!/bin/sh
# Copyright 2015 Marc-Antoine Ruel. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# pre-commit git hook to runs presubmit.py on the tree with unstaged changes
# removed.
#
# WARNING: This file was generated by tool "pre-commit-go"


# Redirect output to stderr.
exec 1>&2


run_checks() {
  # Ensure everything is either tracked or ignored. This is because git stash
  # doesn't stash untracked files.
  untracked="$(git ls-files --others --exclude-standard)"
  if [ "$untracked" != "" ]; then
    echo "This check refuses to run if there is an untracked file. Either track"
    echo "it or put it in the .gitignore or your global exclusion list:"
    echo "$untracked"
    return 1
  fi

  # Run the presubmit check.
  pre-commit-go run
  result=$?
  if [ $result != 0 ]; then
    return $result
  fi
}


if git rev-parse --verify HEAD >/dev/null 2>&1
then
  against=HEAD
else
  # Initial commit: diff against an empty tree object
  against=4b825dc642cb6eb9a060e54bf8d69288fbee4904
fi


# Use a precise "stash, run checks, unstash" to ensure that the check is
# properly run on the data in the index.
# Inspired from
# http://stackoverflow.com/questions/20479794/how-do-i-properly-git-stash-pop-in-pre-commit-hooks-to-get-a-clean-working-tree
# First, stash index and work dir, keeping only the to-be-committed changes in
# the working directory.
old_stash=$(git rev-parse -q --verify refs/stash)
git stash save -q --keep-index
new_stash=$(git rev-parse -q --verify refs/stash)

# If there were no changes (e.g., '--amend' or '--allow-empty') then nothing was
# stashed, and we should skip everything, including the tests themselves.
# (Presumably the tests passed on the previous commit, so there is no need to
# re-run them.)
if [ "$old_stash" = "$new_stash" ]; then
  exit 0
fi

run_checks
result=$?

# Restore changes.
git reset --hard -q && git stash apply --index -q && git stash drop -q
exit $result
`)

var helpText = `pre-commit-go: runs pre-commit checks on Go projects.

Supported commands are:
  help        - this page
  install     - install the git commit hook as .git/hooks/pre-commit
  prereq      - install prerequisites, e.g.: errcheck, golint, goimports, govet,
                etc as applicable for the enabled checks.
  run         - run all enabled checks
  writeconfig - write (or rewrite) a pre-commit-go.json

When executed without command, it does the equivalent of 'prereq', 'install'
then 'run'.

Supported flags are:
  -verbose

Supported checks:
  Native ones that only depends on the stdlib:
    - go build
    - go test
    - gofmt -s
  Checks that have prerequisites (which will be automatically installed):
    - errcheck
    - goimports
    - golint
    - go tool vet
    - go test -cover

No check ever modify any file.
`

// Configuration.

type Config struct {
	MaxDuration float64 `json:"maxduration"` // In seconds.

	// Native checks.
	Build Build `json:"build"`
	Gofmt Gofmt `json:"gofmt"`
	Test  Test  `json:"test"`

	// Checks that require prerequisites.
	Errcheck     Errcheck     `json:"errcheck"`
	Goimports    Goimports    `json:"goimports"`
	Golint       Golint       `json:"golint"`
	Govet        Govet        `json:"govet"`
	TestCoverage TestCoverage `json:"testcoverage"`

	// User configurable presubmit checks.
	CustomChecks []CustomCheck `json:"customchecks"`
}

// getConfig() returns a Config with defaults set then loads the config from
// pre-commit-go.json.
// TODO(maruel): filename is subject to change.
func getConfig() *Config {
	config := &Config{MaxDuration: 120}

	// Set defaults for native tools.
	config.Build.Enabled = true                     //
	config.Gofmt.Enabled = true                     //
	config.Test.Enabled = true                      //
	config.Test.ExtraArgs = []string{"-v", "-race"} // Enable the race detector by default.

	// Set defaults for add-on tools.
	config.Errcheck.Enabled = true    // TODO(maruel): A future version will disable this by default.
	config.Errcheck.Ignores = "Close" // "Close|Write.*|Flush|Seek|Read.*"
	config.Goimports.Enabled = true   //
	config.Golint.Enabled = true      // TODO(maruel): A future version will disable this by default.
	config.Govet.Enabled = true       // TODO(maruel): A future version will disable this by default.
	config.Govet.Blacklist = []string{" composite literal uses unkeyed fields"}
	config.TestCoverage.Enabled = true //
	config.TestCoverage.Minimum = 20.  //

	// TODO(maruel): I'd prefer to use yaml (github.com/go-yaml/yaml) but that
	// would mean slowing down go get .../pre-commit-go. Other option is to godep
	// it but go-yaml is under active development.
	content, err := ioutil.ReadFile("pre-commit-go.json")
	if err == nil {
		_ = json.Unmarshal(content, config)
	}
	out, _ := json.MarshalIndent(config, "", "  ")
	if !bytes.Equal(out, content) {
		// TODO(maruel): Return an error.
	}
	return config
}

// allChecks returns all the enabled checks.
func (c *Config) allChecks() []Check {
	out := []Check{}
	all := []Check{&c.Build, &c.Gofmt, &c.Test, &c.Errcheck, &c.Goimports, &c.Golint, &c.Govet, &c.TestCoverage}
	for i := range c.CustomChecks {
		all = append(all, &c.CustomChecks[i])
	}
	for _, c := range all {
		if c.enabled() {
			out = append(out, c)
		}
	}
	return out
}

// Commands.

// installPrereq installs all the packages needed to run the enabled checks.
func installPrereq() error {
	config := getConfig()
	var wg sync.WaitGroup
	checks := config.allChecks()
	c := make(chan string, len(checks))
	for _, check := range checks {
		for _, p := range check.prerequisites() {
			wg.Add(1)
			go func(prereq CheckPrerequisite) {
				defer wg.Done()
				_, exitCode, _ := capture(prereq.HelpCommand...)
				if exitCode != prereq.ExpectedExitCode {
					c <- prereq.URL
				}
			}(p)
		}
	}
	wg.Wait()
	loop := true
	// Use a map to remove duplicates.
	m := map[string]bool{}
	for loop {
		select {
		case url := <-c:
			m[url] = true
		default:
			loop = false
		}
	}
	urls := make([]string, 0, len(m))
	for url := range m {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	if len(urls) != 0 {
		fmt.Printf("Installing:\n")
		for _, url := range urls {
			fmt.Printf("  %s\n", url)
		}

		// First try without -u, then with -u. The main reason is golint, which
		// changed its API around go1.3~1.4 time frame. -u slows things down
		// significantly so it's worth trying out without, and people will
		// generally do not like to have things upgraded behind them.
		out, _, err := capture(append([]string{"go", "get"}, urls...)...)
		if len(out) != 0 || err != nil {
			out, _, err = capture(append([]string{"go", "get", "-u"}, urls...)...)
		}
		if len(out) != 0 {
			return fmt.Errorf("prerequisites installation failed: %s", out)
		}
		if err != nil {
			return fmt.Errorf("prerequisites installation failed: %s", err)
		}
	}
	return nil
}

// install first calls installPrereq() then install the .git/hooks/pre-commit hook.
func install() error {
	if err := installPrereq(); err != nil {
		return err
	}
	gitDir, err := captureAbs("git", "rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("failed to find .git dir: %s", err)
	}
	// Always remove "pre-commit" first if it exists, in case it's a symlink.
	p := filepath.Join(gitDir, "hooks", "pre-commit")
	_ = os.Remove(p)
	err = ioutil.WriteFile(p, preCommitHook, 0766)
	log.Printf("installation done")
	return err
}

// run runs all the enabled checks.
func run() error {
	start := time.Now()
	config := getConfig()
	checks := config.allChecks()
	var wg sync.WaitGroup
	errs := make(chan error, len(checks))
	for _, c := range checks {
		wg.Add(1)
		go func(check Check) {
			defer wg.Done()
			log.Printf("%s...", check.name())
			start := time.Now()
			err := check.run()
			duration := time.Now().Sub(start)
			log.Printf("... %s in %1.2fs", check.name(), duration.Seconds())
			if err != nil {
				errs <- err
			}
			// A check that took too long is a check that failed.
			max := check.maxDuration()
			if max == 0 {
				max = config.MaxDuration
			}
			if duration > time.Duration(max)*time.Second {
				errs <- fmt.Errorf("check %s took %1.2fs", check.name(), duration.Seconds())
			}
		}(c)
	}
	wg.Wait()

	var err error
	for {
		select {
		case err = <-errs:
			fmt.Printf("%s\n", err)
		default:
			if err != nil {
				duration := time.Now().Sub(start)
				return fmt.Errorf("checks failed in %1.2fs", duration.Seconds())
			}
			return err
		}
	}
}

func writeConfig() error {
	config := getConfig()
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	_ = os.Remove("pre-commit-go.json")
	return ioutil.WriteFile("pre-commit-go.json", out, 0666)
}

func mainImpl() error {
	cmd := ""
	if len(os.Args) == 1 {
		cmd = "installRun"
	} else {
		cmd = os.Args[1]
		copy(os.Args[1:], os.Args[2:])
		os.Args = os.Args[:len(os.Args)-1]
	}
	verbose := flag.Bool("verbose", false, "verbose")
	flag.Parse()

	log.SetFlags(log.Lmicroseconds)
	if !*verbose {
		log.SetOutput(ioutil.Discard)
	}

	gitRoot, err := captureAbs("git", "rev-parse", "--show-cdup")
	if err != nil {
		return fmt.Errorf("failed to find git checkout root")
	}
	if err := os.Chdir(gitRoot); err != nil {
		return fmt.Errorf("failed to chdir to git checkout root: %s", err)
	}

	if cmd == "help" || cmd == "-help" || cmd == "-h" {
		fmt.Printf(helpText)
		return nil
	}
	if cmd == "install" || cmd == "i" {
		return install()
	}
	if cmd == "installRun" {
		if err := install(); err != nil {
			return err
		}
		return run()
	}
	if cmd == "prereq" || cmd == "p" {
		return installPrereq()
	}
	if cmd == "run" || cmd == "r" {
		return run()
	}
	if cmd == "writeconfig" || cmd == "w" {
		return writeConfig()
	}
	return errors.New("unknown command")
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-commit-go: %s\n", err)
		os.Exit(1)
	}
}
