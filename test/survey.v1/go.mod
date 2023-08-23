module github.com/ActiveState/termtest/test/survey.v1

go 1.20

require (
	github.com/ActiveState/termtest v0.0.0-00010101000000-000000000000
	github.com/creack/pty v1.1.18
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02
	gopkg.in/AlecAivazis/survey.v1 v1.8.8
)

require (
	github.com/ActiveState/pty v0.0.0-20230628221854-6fb90eb08a14 // indirect
	github.com/Netflix/go-expect v0.0.0-20220104043353-73e0943537d2 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-isatty v0.0.8 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	golang.org/x/sys v0.9.0 // indirect
)

replace github.com/ActiveState/termtest => ../../

//replace gopkg.in/AlecAivazis/survey.v1 => ./surveylib
