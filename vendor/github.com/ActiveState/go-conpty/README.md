# go-conpty

<p align="center">
  <a href="https://github.com/ActiveState/go-conpty/actions?query=workflow%3Atest-example"><img alt="GitHub Actions status" src="https://github.com/ActiveState/go-conpty/workflows/test-example/badge.svg" /></a>
</p>


Support for the [Windows pseudo
console](https://devblogs.microsoft.com/commandline/windows-command-line-introducing-the-windows-pseudo-console-conpty/)
in Go.

Developed as part of the cross-platform terminal automation library
[go-expect](https://github.com/ActiveState/go-expect) for the [ActiveState
state tool](https://www.activestate.com/products/platform/state-tool/).

## Install

    go get github.com/ActiveState/go-conpty

## Example

See cmd/example/main.go

## Client configuration

On Windows, you may have to adjust the programme that you are running in the
pseudo-console, by configuring the standard output handler to process virtual
terminal codes. See https://docs.microsoft.com/en-us/windows/console/setconsolemode

This package comes with a convenience function `InitTerminal()` that you can
use in your client to set this option.

