package server

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (run *Run) readStdout(std io.Reader, exitStatus chan int) {
	scanner := bufio.NewScanner(std)

	for scanner.Scan() {
		text := scanner.Text()

		if len(text) > 2 && text[0:2] == "__" {
			run.Log.Trace(text)
			parts := strings.Split(text, "=")
			switch parts[0] {
			case "__EXIT":
				if len(parts) != 2 {
					run.Log.Errorf("invalid __EXIT: %s", text)
					continue
				}
				status, err := strconv.Atoi(parts[1])
				if err != nil {
					run.Log.Errorf("invalid __EXIT value: %s", text)
					continue
				}
				run.Log.Tracef("EXIT detected: %s (status %d)", text, status)
				exitStatus <- status
			default:
				run.Log.Errorf("unknown keyword: %s", text)
			}
			continue
		} else {
			run.Log.Info(text)
		}

		if err := scanner.Err(); err != nil {
			run.Log.Errorf("error reading stdout: %s", err)
		}
	}
}

func (run *Run) readStderr(std io.Reader) {
	scanner := bufio.NewScanner(std)

	for scanner.Scan() {
		text := scanner.Text()
		run.Log.Error(text)
	}

	if err := scanner.Err(); err != nil {
		run.Log.Errorf("error reading stderr: %s", err)
		return // !!!
	}
}

// scripts -> ssh
func (run *Run) stdinInject(out io.WriteCloser, exitStatus chan int) {

	defer out.Close()

	var err error

	for num, task := range run.Tasks {

		// trace current script!
		run.Log.Tracef("------ script: %s ------", task.ScriptName)

		var scanner *bufio.Scanner

		scanner = bufio.NewScanner(task.ScriptReader)

		// args := task.Probe.Arguments
		// params := make(map[string]interface{})
		// args = StringExpandVariables(args, params)
		args := ""

		// cat is needed to "focus" stdin only on the child bash
		// cat is "sudoed" so it can be killed by __kill_subshell bellow
		str := fmt.Sprintf("sudo -u %s cat | sudo -u %s __SCRIPT_ID=%d bash -s -- %s ; echo __EXIT=$?", task.As, task.As, num, args)
		run.Log.Tracef("child=%s", str)

		_, err = out.Write([]byte(str + "\n"))
		if err != nil {
			run.Log.Errorf("error writing (starting child bash): %s", err)
			return
		}

		// pkill -og0 cat: we kill the oldest "cat" of our process group (see above)
		// no newline so we dont change line numbers
		_, err = out.Write([]byte("function __kill_subshell() { pkill -og0 cat ; } ; export -f __kill_subshell ; trap __kill_subshell EXIT ; "))
		if err != nil {
			run.Log.Errorf("error writing (init child bash): %s", err)
			return
		}

		for scanner.Scan() {
			text := scanner.Text()
			run.Log.Tracef("stdin=%s", text)
			_, errw := out.Write([]byte(text + "\n"))
			if errw != nil {
				run.Log.Errorf("error writing: %s", errw)
				return
			}
		}

		run.Log.Tracef("killing subshell")
		_, err = out.Write([]byte("__kill_subshell\n"))
		if err != nil {
			run.Log.Errorf("error writing (while killing subshell): %s", err)
			return
		}

		if err = scanner.Err(); err != nil {
			run.Log.Errorf("error scanner: %s", err)
			return
		}

		status := <-exitStatus
		if status != 0 {
			run.Log.Errorf("detected non-zero exit status: %d", status)
			return
		}
	}
}
func (run *Run) preparePipes() error {
	exitStatus := make(chan int)
	session := run.SSHConn.Session

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("Unable to setup stdin for session: %v", err)
	}
	go run.stdinInject(stdin, exitStatus)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Unable to setup stdout for session: %v", err)
	}
	go run.readStdout(stdout, exitStatus)

	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("Unable to setup stderr for session: %v", err)
	}
	go run.readStderr(stderr)

	return nil
}
