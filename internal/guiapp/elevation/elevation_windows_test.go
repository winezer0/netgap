package elevation

import "testing"

func TestIsRestartChild(t *testing.T) {
	if !IsRestartChild([]string{"--netgap-restart-parent=123", "--netgap-restart-state=C:\\temp\\state.json"}) {
		t.Fatal("restart arguments were not recognized")
	}
	if IsRestartChild([]string{"--autostart"}) {
		t.Fatal("ordinary application arguments were treated as restart arguments")
	}
}
