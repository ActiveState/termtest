module github.com/ActiveState/termtest

go 1.18

require (
	github.com/creack/pty v1.1.11
	github.com/stretchr/testify v1.8.0
	go.uber.org/goleak v1.2.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.0.0-20220721230656-c6bc011c0c49 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/creack/pty v1.1.11 => github.com/ActiveState/pty v0.0.0-20230323202545-db6cfc0728c8
