module github.com/ActiveState/termtest/expect

go 1.12

require (
	github.com/ActiveState/termtest/xpty v0.5.3
	github.com/ActiveState/vt10x v1.2.0
	github.com/Netflix/go-expect v0.0.0-20200312175327-da48e75238e2 // indirect
	github.com/creack/pty v1.1.11 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/stretchr/testify v1.5.1
	golang.org/x/sys v0.0.0-20200728102440-3e129f6d46b1 // indirect
)

replace github.com/ActiveState/termtest/xpty => ../xpty
