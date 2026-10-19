package guiapp

import "github.com/wailsapp/wails/v2/pkg/runtime"

// ShowWindow 显示并恢复主窗口。
func (s *Service) ShowWindow() {
	if s.ctx == nil {
		return
	}
	runtime.WindowShow(s.ctx)
	runtime.WindowUnminimise(s.ctx)
	runtime.WindowCenter(s.ctx)
	s.emitState()
}

// MinimiseWindow 将窗口最小化到任务栏。
func (s *Service) MinimiseWindow() {
	if s.ctx == nil {
		return
	}
	runtime.WindowMinimise(s.ctx)
}

// HideToTray 将窗口隐藏到托盘区域。
func (s *Service) HideToTray() {
	if s.ctx == nil {
		return
	}
	runtime.WindowHide(s.ctx)
}

// QuitApp 退出当前 GUI 程序。
func (s *Service) QuitApp() {
	if s.ctx == nil {
		return
	}
	runtime.Quit(s.ctx)
}
