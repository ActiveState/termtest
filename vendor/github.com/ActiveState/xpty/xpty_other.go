// +build darwin dragonfly linux netbsd openbsd solaris

package xpty

import (
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type impl struct {
	ptm    *os.File
	pts    *os.File
	rwPipe *readWritePipe
}

func open(cols uint16, rows uint16) (*impl, error) {
	ptm, pts, err := pty.Open()
	if err != nil {
		return nil, err
	}
	pty.Setsize(ptm, &pty.Winsize{Cols: cols, Rows: rows})
	return &impl{ptm: ptm, pts: pts}, nil
}

func (p *impl) terminalOutPipe() io.Reader {
	return p.ptm
}

func (p *impl) terminalInPipe() io.Writer {
	return p.ptm
}

func (p *impl) close() error {
	p.pts.Close()
	p.ptm.Close()
	return nil
}

func (p *impl) tty() *os.File {
	return p.pts
}

func (p *impl) terminalOutFd() uintptr {
	return p.ptm.Fd()
}

func (p *impl) startProcessInTerminal(cmd *exec.Cmd) error {
	cmd.Stdin = p.pts
	cmd.Stdout = p.pts
	cmd.Stderr = p.pts
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Ctty = int(p.pts.Fd())
	return cmd.Start()
}
