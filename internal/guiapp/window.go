package guiapp

// ShowWindow shows, restores and centers the main window.
func (s *Service) ShowWindow() {
	if s.window == nil {
		return
	}
	s.windowHidden.Store(false)
	s.window.Show()
	s.window.UnMinimise()
	s.window.Center()
	s.emitState()
}
func (s *Service) MinimiseWindow() {
	if s.window == nil {
		return
	}
	s.windowForeground.Store(false)
	s.window.Minimise()
}
func (s *Service) HideToTray() {
	if s.window == nil {
		return
	}
	s.windowHidden.Store(true)
	s.windowForeground.Store(false)
	s.window.Hide()
}
func (s *Service) QuitApp() {
	if s.app == nil {
		return
	}
	s.app.Quit()
}
