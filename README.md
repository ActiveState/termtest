# termtest

<p align="center">
  <a href="https://github.com/ActiveState/termtest/actions?query=workflow%3Aunit-tests"><img alt="GitHub Actions status" src="https://github.com/ActiveState/termtest/workflows/unit-tests/badge.svg" /></a>
</p>

An automatable terminal session with send/expect controls.

This package leverages the [go-expect](https://github.com/ActiveState/go-expect) package to test terminal applications on Linux, MacOS and Windows.

It has been developed for CI testing of the [ActiveState state
tool](https://www.activestate.com/products/platform/state-tool/)

## Example usage

```go

import (
    "testing"

    "github.com/ActiveState/termtest"
    "github.com/stretchr/testify/suite"
)

func TestBash(t *testing.T) {
    opts := termtest.Options{
        CmdName: "/bin/bash",
    }
    cp, err := termtest.NewTest(t, opts)
    require.NoError(t, err, "create console process")
    defer cp.Close()

    cp.SendLine("echo hello world")
    cp.Expect("hello world")
    cp.SendLine("exit")
    cp.ExpectExitCode(0)
}

```
