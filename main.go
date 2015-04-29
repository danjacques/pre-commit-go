// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// pre-commit-go: runs pre-commit checks on Go projects.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/maruel/pre-commit-go/checks"
	"gopkg.in/yaml.v2"
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

var helpText = template.Must(template.New("help").Parse(`pre-commit-go: runs pre-commit checks on Go projects, fast.

Supported commands are:
  help        - this page
  install     - runs 'prereq' then installs the git commit hook as
                .git/hooks/pre-commit
  prereq      - installs prerequisites, e.g.: errcheck, golint, goimports,
                govet, etc as applicable for the enabled checks
  installrun  - runs 'prereq', 'install' then 'run'
  run         - runs all enabled checks
  writeconfig - writes (or rewrite) a pre-commit-go.yml

When executed without command, it does the equivalent of 'installrun'.
Supported flags are:
{{.Usage}}
Supported checks and their runlevel:
  Native checks that only depends on the stdlib:{{range .NativeChecks}}
    - {{printf "%-*s" $.Max .GetName}} : {{.GetDescription}}{{end}}

  Checks that have prerequisites (which will be automatically installed):{{range .OtherChecks}}
    - {{printf "%-*s" $.Max .GetName}} : {{.GetDescription}}{{end}}

No check ever modify any file.
`))

// Commands.

func help(config *checks.Config, usage string) error {
	s := &struct {
		Usage        string
		Max          int
		NativeChecks []checks.Check
		OtherChecks  []checks.Check
	}{
		usage,
		0,
		[]checks.Check{},
		[]checks.Check{},
	}
	for name, c := range checks.KnownChecks {
		if v := len(name); v > s.Max {
			s.Max = v
		}
		if len(c.GetPrerequisites()) == 0 {
			s.NativeChecks = append(s.NativeChecks, c)
		} else {
			s.OtherChecks = append(s.OtherChecks, c)
		}
	}
	return helpText.Execute(os.Stdout, s)
}

// installPrereq installs all the packages needed to run the enabled checks.
func installPrereq(config *checks.Config, r checks.RunLevel) error {
	var wg sync.WaitGroup
	enabledChecks := config.EnabledChecks(r)
	c := make(chan string, len(enabledChecks))
	for _, check := range enabledChecks {
		for _, p := range check.GetPrerequisites() {
			wg.Add(1)
			go func(prereq checks.CheckPrerequisite) {
				defer wg.Done()
				if !prereq.IsPresent() {
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
func install(config *checks.Config, r checks.RunLevel) error {
	if err := installPrereq(config, r); err != nil {
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

func callRun(check checks.Check) (error, time.Duration) {
	if l, ok := check.(sync.Locker); ok {
		l.Lock()
		defer l.Unlock()
	}
	start := time.Now()
	err := check.Run()
	return err, time.Now().Sub(start)
}

// run runs all the enabled checks.
func run(config *checks.Config, r checks.RunLevel) error {
	start := time.Now()
	enabledChecks := config.EnabledChecks(r)
	var wg sync.WaitGroup
	errs := make(chan error, len(enabledChecks))
	for _, c := range enabledChecks {
		wg.Add(1)
		go func(check checks.Check) {
			defer wg.Done()
			log.Printf("%s...", check.GetName())
			err, duration := callRun(check)
			log.Printf("... %s in %1.2fs", check.GetName(), duration.Seconds())
			if err != nil {
				errs <- err
			}
			// A check that took too long is a check that failed.
			max := config.MaxDuration
			if duration > time.Duration(max)*time.Second {
				errs <- fmt.Errorf("check %s took %1.2fs", check.GetName(), duration.Seconds())
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

func writeConfig(config *checks.Config, configPath string) error {
	content, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("internal error when marshaling config: %s", err)
	}
	_ = os.Remove(configPath)
	out := []byte("# https://github.com/maruel/pre-commit-go configuration file to run checks\n# automatically on commit and pull requests.\n#\n# See https://godoc.org/github.com/maruel/pre-commit-go/checks for more\n# information.\n\n")
	out = append(out, content...)
	return ioutil.WriteFile(configPath, out, 0666)
}

func mainImpl() error {
	cmd := ""
	if len(os.Args) == 1 {
		cmd = "installrun"
	} else {
		cmd = os.Args[1]
		copy(os.Args[1:], os.Args[2:])
		os.Args = os.Args[:len(os.Args)-1]
	}
	verbose := flag.Bool("verbose", false, "enables verbose logging output")
	configPath := flag.String("config", "pre-commit-go.yml", "file name of the config to load")
	runLevelFlag := flag.Int("level", 1, "runlevel, between 0 and 3; the higher, the more tests are run")
	flag.Parse()

	log.SetFlags(log.Lmicroseconds)
	if !*verbose {
		log.SetOutput(ioutil.Discard)
	}

	if *runLevelFlag < 0 || *runLevelFlag > 3 {
		return fmt.Errorf("-level %d is invalid, must be between 0 and 3", *runLevelFlag)
	}
	runLevel := checks.RunLevel(*runLevelFlag)

	gitRoot, err := captureAbs("git", "rev-parse", "--show-cdup")
	if err != nil {
		return fmt.Errorf("failed to find git checkout root")
	}

	// Make the config path relative to the current directory, not the git
	// repository root.
	if *configPath, err = filepath.Abs(*configPath); err != nil {
		return err
	}
	if err := os.Chdir(gitRoot); err != nil {
		return fmt.Errorf("failed to chdir to git checkout root: %s", err)
	}
	config := checks.GetConfig(*configPath)

	switch cmd {
	case "help", "-help", "-h":
		b := &bytes.Buffer{}
		flag.CommandLine.SetOutput(b)
		flag.CommandLine.PrintDefaults()
		return help(config, b.String())
	case "install", "i":
		return install(config, runLevel)
	case "installrun":
		if err := install(config, runLevel); err != nil {
			return err
		}
		return run(config, runLevel)
	case "prereq", "p":
		return installPrereq(config, runLevel)
	case "run", "r":
		return run(config, runLevel)
	case "writeconfig", "w":
		return writeConfig(config, *configPath)
	default:
		return errors.New("unknown command, try 'help'")
	}
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-commit-go: %s\n", err)
		os.Exit(1)
	}
}
