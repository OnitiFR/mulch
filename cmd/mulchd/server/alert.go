package server

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Alert are used only for background big "events" (seed download
// failure, vm autorebuild failure, etc)
type Alert struct {
	Type    string
	Subject string
	Content string
}

// Alert.Type values
const (
	AlertTypeGood = "GOOD"
	AlertTypeBad  = "BAD"
)

const alertScriptDirectory = "alerts"

// AlertSender will be attached to the application
type AlertSender struct {
	scriptsPath string
	log         *Log
}

func listScripts(scriptsPath string) ([]string, error) {

	stat, err := os.Stat(scriptsPath)

	if err != nil {
		return nil, fmt.Errorf("invalid directory '%s': %s", scriptsPath, err)
	}

	if !stat.Mode().IsDir() {
		return nil, fmt.Errorf("is not a directory '%s'", scriptsPath)
	}

	list, err := filepath.Glob(scriptsPath + "/*.sh")
	if err != nil {
		return nil, fmt.Errorf("error listing '%s' directory: %s", scriptsPath, err)
	}

	return list, nil
}

// NewAlertSender creates a new AlertSender
func NewAlertSender(configPath string, log *Log) (*AlertSender, error) {
	scriptsPath := path.Clean(configPath + "/" + alertScriptDirectory)

	sender := &AlertSender{
		scriptsPath: scriptsPath,
		log:         log,
	}

	// test run
	list, err := listScripts(scriptsPath)
	if err != nil {
		return nil, fmt.Errorf("alert scripts init: %s", err.Error())
	}

	if len(list) == 0 {
		log.Warningf("no alert script found (no *.sh files in '%s') so you're currently blind to background failures", scriptsPath)
	}

	return sender, nil
}

// Send an alert using all alert scripts (etc/alerts/*.sh)
func (sender *AlertSender) Send(alert *Alert) error {
	alertScripts, err := listScripts(sender.scriptsPath)
	if err != nil {
		return err
	}

	varMap := make(map[string]string)
	varMap["TYPE"] = alert.Type
	varMap["SUBJECT"] = alert.Subject
	varMap["CONTENT"] = alert.Content
	varMap["DATETIME"] = time.Now().Format(time.RFC3339)

	scriptsWithError := make([]string, 0)

	for _, script := range alertScripts {
		cmd := exec.Command(script)

		env := os.Environ()
		for key, val := range varMap {
			env = append(env, fmt.Sprintf("%s=%s", key, val))
		}
		cmd.Env = env
		if cmdOut, err := cmd.CombinedOutput(); err != nil {
			sender.log.Errorf("error running alert script '%s': %s, output: %s", script, err, cmdOut)
			scriptsWithError = append(scriptsWithError, script)
		} else {
			sender.log.Infof("alert run was successfull '%s'", script)
		}
	}

	if len(scriptsWithError) > 0 {
		return fmt.Errorf("error with the following alert scripts: %s", strings.Join(scriptsWithError, ", "))
	}

	return nil
}

// RunKeepAlive will send a keepalive alert every 24h
func (sender *AlertSender) RunKeepAlive() {
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			sender.Send(&Alert{
				Type:    AlertTypeGood,
				Subject: "Hi",
				Content: "I'm alive and able to send alerts. Seeya!",
			})
		}
	}()
}
