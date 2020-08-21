module github.com/ActiveState/termtest/xpty

go 1.14

require (
	github.com/ActiveState/termtest/conpty v0.5.0
	github.com/ActiveState/vt10x v1.3.1
	github.com/Netflix/go-expect v0.0.0-20200312175327-da48e75238e2 // indirect
	github.com/creack/pty v1.1.11
	github.com/kr/pty v1.1.8 // indirect
	github.com/stretchr/testify v1.5.1
	golang.org/x/crypto v0.0.0-20200427165652-729f1e841bcc
	golang.org/x/sys v0.0.0-20200821140526-fda516888d29 // indirect
)

replace github.com/ActiveState/termtest/conpty => ../conpty
