module github.com/ActiveState/termtest/test/pty_deadlock

go 1.20

require (
	github.com/ActiveState/termtest v0.0.0-00010101000000-000000000000
	github.com/creack/pty v0.0.0-00010101000000-000000000000
	gopkg.in/AlecAivazis/survey.v1 v1.8.8
)

require (
	github.com/ActiveState/pty v0.0.0-20230628221854-6fb90eb08a14 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-isatty v0.0.8 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v3 v3.23.8 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	golang.org/x/sys v0.11.0 // indirect
)

replace github.com/creack/pty => github.com/photostorm/pty v0.0.0-20230324012736-6794a5ba0ba0

replace github.com/ActiveState/termtest => ../../
