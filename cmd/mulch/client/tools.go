package client

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
)

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}

}

func stringIsVariable(s string, varName string) (bool, string) {
	if !strings.HasPrefix(s, varName+"=") {
		return false, ""
	}
	return true, s[len(varName)+1:]
}
