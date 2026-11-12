package server

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
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
	AlertTypeGood     = "GOOD"
	AlertTypeBad      = "BAD"
	AlertTypeSecurity = "SECURITY" // sensitive actions and suspicious events
)

const alertScriptDirectory = "alerts"

// suspicious events (that an attacker can repeat) are notified at most
// once per interval (see SecurityThrottled)
const securityAlertThrottle = 15 * time.Minute

// AlertSender will be attached to the application
type AlertSender struct {
	scriptsPath string
	log         *Log

	lastSent map[string]time.Time // throttling (see sendBackground)
	mutex    sync.Mutex
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
		lastSent:    make(map[string]time.Time),
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
	varMap["SUBJECT"] = alertSanitize(alert.Subject)
	varMap["CONTENT"] = alertSanitize(alert.Content)
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

// Security notifies a sensitive action (in background)
func (sender *AlertSender) Security(subject string, content string) {
	sender.sendBackground(&Alert{
		Type:    AlertTypeSecurity,
		Subject: subject,
		Content: content,
	}, "", 0)
}

// SecurityThrottled notifies a suspicious event (in background), at most
// once per securityAlertThrottle for the same subject and throttleID (an
// API key name, for instance), since an attacker can repeat such events
func (sender *AlertSender) SecurityThrottled(subject string, throttleID string, content string) {
	sender.sendBackground(&Alert{
		Type:    AlertTypeSecurity,
		Subject: subject,
		Content: content,
	}, subject+"\n"+throttleID, securityAlertThrottle)
}

// sendBackground sends the alert without waiting for scripts to finish.
// If throttleID is not empty, alerts with the same ID are sent at most
// once per throttleInterval.
func (sender *AlertSender) sendBackground(alert *Alert, throttleID string, throttleInterval time.Duration) {
	if throttleID != "" {
		sender.mutex.Lock()
		now := time.Now()
		for id, last := range sender.lastSent {
			if now.Sub(last) >= throttleInterval {
				delete(sender.lastSent, id)
			}
		}
		_, throttled := sender.lastSent[throttleID]
		if !throttled {
			sender.lastSent[throttleID] = now
		}
		sender.mutex.Unlock()

		if throttled {
			return
		}
	}

	go func() {
		err := sender.Send(alert)
		if err != nil {
			sender.log.Error(err.Error())
		}
	}()
}

// alertSanitize replaces control characters (new lines, NUL, …) with spaces:
// alerts are single-line, and a NUL byte is not allowed in the environment.
// Everything else is passed as is (quotes, $, backslashes, …) and some
// content comes from users (key comments, SSH key comments…): alert
// scripts must escape these variables for their own output format.
func alertSanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

// RunKeepAlive will send a keepalive alert every X days
func (sender *AlertSender) RunKeepAlive(daysInterval int) {
	go func() {
		for {
			time.Sleep(24 * time.Hour * time.Duration(daysInterval))
			sender.Send(&Alert{
				Type:    AlertTypeGood,
				Subject: "Hi",
				Content: "I'm alive and able to send alerts. Seeya!",
			})
		}
	}()
}
