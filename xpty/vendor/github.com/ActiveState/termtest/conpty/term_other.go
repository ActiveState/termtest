// Copyright 2020 ActiveState Software. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file

// +build !windows

package conpty

func InitTerminal(_ bool) (func(), error) {
	return func() {}, nil
}
