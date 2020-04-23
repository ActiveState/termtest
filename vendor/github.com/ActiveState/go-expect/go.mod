module github.com/ActiveState/go-expect

go 1.14

require (
	github.com/ActiveState/vt10x v1.1.0
	github.com/ActiveState/xpty v0.1.1
	github.com/stretchr/testify v1.5.1
)

replace github.com/ActiveState/xpty => ../xpty

replace github.com/ActiveState/vt10x => ../vt10x
