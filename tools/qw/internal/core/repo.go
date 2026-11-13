package core

import (
	"os/exec"
	"strings"
)

// imagesRoot is the images/ checkout; main sets it.
var ImagesRoot = "."

// current is the app's VERSIONS array from its build.conf (bash, because build.conf is bash).
func Current(app string) []string {
	out, _ := exec.Command("bash", "-c", "source "+ImagesRoot+"/apps/"+app+"/build.conf; echo ${VERSIONS[@]}").Output()
	return strings.Fields(string(out))
}
