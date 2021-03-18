package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (run *Run) readStdout(ctx context.Context, std io.Reader, exitStatus chan int) error {
	scanner := bufio.NewScanner(std)

	for scanner.Scan() {
		text := scanner.Text()

		if len(text) > 2 && text[0:2] == "__" {
			run.Log.Trace(text)
			parts := strings.Split(text, "=")
			switch parts[0] {
			case "___EXIT":
				if len(parts) != 2 {
					run.Log.Errorf("invalid ___EXIT: %s", text)
					continue
				}
				status, err := strconv.Atoi(parts[1])
				if err != nil {
					run.Log.Errorf("invalid ___EXIT value: %s", text)
					continue
				}
				run.Log.Tracef("EXIT detected: %s (status %d)", text, status)
				select {
				case exitStatus <- status:
				case <-ctx.Done():
					return fmt.Errorf("premature exit: %s", ctx.Err())
				}
			default:
				run.Log.Errorf("unknown keyword: %s", text)
			}
			continue
		} else {
			run.Log.Info(text)
			if run.StdoutCallback != nil {
				run.StdoutCallback(text)
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stdout: %s", err)
		}
	}
	return nil
}

func (run *Run) readStderr(std io.Reader) error {
	scanner := bufio.NewScanner(std)

	for scanner.Scan() {
		text := scanner.Text()
		run.Log.Error(text)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stderr: %s", err)
	}
	return nil
}

// scripts -> ssh
func (run *Run) stdinInject(ctx context.Context, out io.WriteCloser, exitStatus chan int) error {

	defer out.Close()

	var err error

	for num, task := range run.Tasks {

		run.Log.Infof("------ [%s] script: %s ------", run.Caption, task.ScriptName)

		scanner := bufio.NewScanner(task.ScriptReader)

		args := task.Arguments
		// params := make(map[string]interface{})
		// args = StringExpandVariables(args, params)

		// cat is needed to "focus" stdin only on the child bash
		// cat is "sudoed" so it can be killed by __kill_subshell bellow
		str := fmt.Sprintf("sudo -iu %s cat | sudo -iu %s __SCRIPT_ID=%d bash -s -- %s ; echo ___EXIT=$?", task.As, task.As, num, args)
		run.Log.Tracef("child=%s", str)

		_, err = out.Write([]byte(str + "\n"))
		if err != nil {
			return fmt.Errorf("error writing (starting child bash): %s", err)
		}

		// pkill -og0 cat: we kill the oldest "cat" of our process group (see above)
		// no newline so we dont change line numbers
		_, err = out.Write([]byte("function __kill_subshell() { pkill -og0 cat ; } ; export -f __kill_subshell ; trap __kill_subshell EXIT ; cd ; "))
		if err != nil {
			return fmt.Errorf("error writing (init child bash): %s", err)
		}

		for scanner.Scan() {
			text := scanner.Text()
			run.Log.Tracef("stdin=%s", text)
			_, errw := out.Write([]byte(text + "\n"))
			if errw != nil {
				return fmt.Errorf("error writing: %s", errw)
			}
		}

		run.Log.Tracef("killing subshell")
		_, err = out.Write([]byte("__kill_subshell\n"))
		if err != nil {
			return fmt.Errorf("error writing (while killing subshell): %s", err)
		}

		if err = scanner.Err(); err != nil {
			return fmt.Errorf("error scanner: %s", err)
		}

		select {
		case status := <-exitStatus:
			if status != 0 {
				return fmt.Errorf("detected non-zero exit status: %d", status)
			}
		case <-ctx.Done():
			return fmt.Errorf("context exit: %s", ctx.Err())
		}
	}
	return nil
}

func (run *Run) preparePipes(ctx context.Context, errorChan chan error) error {
	exitStatus := make(chan int)
	session := run.SSHConn.Session

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("unable to setup stdin for session: %v", err)
	}
	go func() {
		errI := run.stdinInject(ctx, stdin, exitStatus)
		select {
		case errorChan <- errI:
		case <-ctx.Done():
		}
	}()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("unable to setup stdout for session: %v", err)
	}
	go func() {
		errI := run.readStdout(ctx, stdout, exitStatus)
		select {
		case errorChan <- errI:
		case <-ctx.Done():
		}
	}()

	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("unable to setup stderr for session: %v", err)
	}
	go func() {
		errI := run.readStderr(stderr)
		select {
		case errorChan <- errI:
		case <-ctx.Done():
		}
	}()

	return nil
}
