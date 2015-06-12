// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build: !go15

// On pre 1.5, there will be a 2x slow down due to context switches but it will
// still likely help with our use case.
// Ref:
// https://docs.google.com/document/d/1At2Ls5_fhJQ59kDK2DFVhFu3g5mATSXqqV5QrxinasI/preview

package scm

import "runtime"

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}
