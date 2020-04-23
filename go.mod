module github.com/ActiveState/termtest

go 1.14

require (
	github.com/ActiveState/go-expect v0.0.0-20200417191304-164057b5e878
	github.com/ActiveState/vt10x v1.1.0
	github.com/stretchr/testify v1.5.1
)

replace github.com/ActiveState/go-expect => ../go-expect
replace github.com/ActiveState/vt10x => ../vt10x
replace github.com/ActiveState/xpty => ../xpty
