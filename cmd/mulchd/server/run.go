package server

import (
	"context"
	"errors"
	"io"
	"time"
)

// RunTask is a task (script) for a Run
type RunTask struct {
	ScriptName   string
	ScriptReader io.Reader
	As           string
	Arguments    string
	EnvWords     map[string]string
}

// Run is a list of Tasks on Host, including task results
type Run struct {
	Caption string
	SSHConn *SSHConnection
	Tasks   []*RunTask
	// CurrentTask int
	// StartTime    time.Time
	// Duration     time.Duration
	// DialDuration time.Duration
	Log            *Log
	StdoutCallback func(string)
}

// Go will execute the Run
func (run *Run) Go(ctx context.Context) error {
	const bootstrap = "bash -s --"
	errorChan := make(chan error)

	if len(run.Tasks) == 0 {
		run.Log.Info("nothing to run")
		return nil
	}

	if err := run.SSHConn.Connect(); err != nil {
		return err
	}
	defer run.SSHConn.Close()

	if err := run.preparePipes(ctx, errorChan); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		run.Log.Tracef("Close request received, closing SSH session (%s)", ctx.Err())
		run.SSHConn.Close()
	}()

	go run.scheduleSSHKeepAlives()

	if err := run.SSHConn.Session.Run(bootstrap); err != nil {
		return err
	}

	// currently, I'm not sure that I will have and errorChan
	// in every case, so… let's timeout.
	select {
	case err := <-errorChan:
		// we exit on the first error of any stream
		return err
	case <-time.After(1 * time.Second):
		return errors.New("timeout after waiting stderr errorChan")
	}
}

// Send sparses keepalives to detect dead connections, a failed SendRequest
// will set channels to nil, closing the connection (and we use a timeout as well)
// We use low timeouts, since we're inside our own virtual network
func (run *Run) scheduleSSHKeepAlives() {
	sshConn := run.SSHConn.Client.Conn

	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for range t.C {
		run.Log.Tracef("[run %s] send SSH keepalive", run.Caption)
		err := SSHSendKeepAlive(sshConn, 2*time.Second)
		if err != nil {
			run.Log.Tracef("[run %s] ssh keepalive error: %s, closing", run.Caption, err)
			run.SSHConn.Close()
			return
		}
	}
}
