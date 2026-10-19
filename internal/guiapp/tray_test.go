package guiapp

import "testing"

func TestTrayControllerStartShouldBeIdempotent(t *testing.T) {
	t.Parallel()

	calls := 0
	controller := &TrayController{
		start: func() {
			calls++
		},
	}

	controller.Start()
	controller.Start()
	controller.Start()

	if calls != 1 {
		t.Fatalf("expected start called once, got=%d", calls)
	}
}

func TestTrayControllerStopShouldBeIdempotent(t *testing.T) {
	t.Parallel()

	calls := 0
	controller := &TrayController{
		end: func() {
			calls++
		},
	}

	controller.Stop()
	controller.Stop()

	if calls != 1 {
		t.Fatalf("expected stop called once, got=%d", calls)
	}
}

