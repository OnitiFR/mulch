package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func (run *Run) readStdout(std io.Reader, exitStatus chan int) {
	scanner := bufio.NewScanner(std)

	for scanner.Scan() {
		text := scanner.Text()

		run.Log.Tracef("stdout=%s", text)

		if len(text) > 2 && text[0:2] == "__" {
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
		run.Log.Tracef("stderr=%s", text)
	}

	if err := scanner.Err(); err != nil {
		run.Log.Errorf("error reading stderr: %s", err)
		return // !!!
	}
}

// scripts -> ssh
func (run *Run) stdinInject(out io.WriteCloser, exitStatus chan int) {

	defer out.Close()

	// "pkill" dependency or Linux "ps"? (ie: not Cygwin)
	_, err := out.Write([]byte("export __MAIN_PID=$$\nfunction __kill_subshells() { pkill -TERM -P $__MAIN_PID cat; }\nexport -f __kill_subshells\n"))
	if err != nil {
		run.Log.Errorf("error writing (setup parent bash): %s", err)
		return
	}

	for num, task := range run.Tasks {

		// trace current script!
		run.Log.Tracef("------ script: %s ------", task.Script)

		var scanner *bufio.Scanner

		file, erro := os.Open(task.Script)
		if erro != nil {
			run.Log.Errorf("Failed to open script: %s", erro)
			continue
		}
		defer file.Close()

		scanner = bufio.NewScanner(file)

		// args := task.Probe.Arguments
		// params := make(map[string]interface{})
		// args = StringExpandVariables(args, params)
		args := ""

		// cat is needed to "focus" stdin only on the child bash
		str := fmt.Sprintf("cat | __SCRIPT_ID=%d bash -s -- %s ; echo __EXIT=$?\n", num, args)
		run.Log.Tracef("child=%s", str)

		_, err = out.Write([]byte(str))
		if err != nil {
			run.Log.Errorf("error writing (starting child bash): %s", err)
			return
		}

		// no newline so we dont change line numbers
		_, err = out.Write([]byte("trap __kill_subshells EXIT ; "))
		if err != nil {

			run.Log.Errorf("error writing (init child bash): %s", err)
			return
		}

		for scanner.Scan() {
			text := scanner.Text()
			run.Log.Tracef("stdin=%s\n", text)
			_, errw := out.Write([]byte(text + "\n"))
			if errw != nil {
				run.Log.Errorf("error writing: %s", errw)
				return
			}
		}

		run.Log.Tracef("killing subshell")
		_, err = out.Write([]byte("__kill_subshells\n"))
		if err != nil {
			run.Log.Errorf("error writing (while killing subshell): %s", err)
			return
		}

		if err := scanner.Err(); err != nil {
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
