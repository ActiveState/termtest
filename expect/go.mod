module github.com/ActiveState/termtest/expect

go 1.12

require (
	github.com/ActiveState/termtest/xpty v0.5.5
	github.com/ActiveState/vt10x v1.3.1
	github.com/Netflix/go-expect v0.0.0-20200312175327-da48e75238e2 // indirect
	github.com/creack/pty v1.1.11 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/stretchr/testify v1.5.1
)

replace github.com/ActiveState/termtest/xpty => ../xpty
