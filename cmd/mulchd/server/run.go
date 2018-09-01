package server

// RunTask is a task (script) for a Run
type RunTask struct {
	Script string
	As     string
}

// Run is a list of Tasks on Host, including task results
type Run struct {
	SSHConn *SSHConnection
	Tasks   []*RunTask
	// CurrentTask int
	// StartTime    time.Time
	// Duration     time.Duration
	// DialDuration time.Duration
	Log *Log
}

// Go will execute the Run
func (run *Run) Go() error {
	const bootstrap = "bash -s --"

	if err := run.SSHConn.Connect(); err != nil {
		return err
	}
	defer run.SSHConn.Close()

	if err := run.preparePipes(); err != nil {
		return err
	}

	if err := run.SSHConn.Session.Run(bootstrap); err != nil {
		return err
	}

	return nil
}
