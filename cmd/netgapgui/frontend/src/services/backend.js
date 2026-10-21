import {
  EventsOn,
  WindowHide,
} from '../../wailsjs/runtime/runtime'
import {
  AddRegion,
  ClearLogs,
  LogUIError,
  GetInitialState,
  GetDefaultRouteInfo,
  GetProxyConfigInfo,
  HideToTray,
  QuitApp,
  RemoveDefaultRoute,
  RemoveRegion,
  RestoreDefaultRoute,
  SaveSettings,
  StartMonitoring,
  SetWindowForeground,
  StopMonitoring,
} from '../../wailsjs/go/main/App'

export const backend = {
  logUIError: (message) => LogUIError(message),
  getInitialState: () => GetInitialState(),
  saveSettings: (payload) => SaveSettings(payload),
  addRegion: (keyword) => AddRegion(keyword),
  removeRegion: (region) => RemoveRegion(region),
  startMonitoring: () => StartMonitoring(),
  stopMonitoring: () => StopMonitoring(),
  setWindowForeground: (foreground, revision) => SetWindowForeground(foreground, revision),
  removeDefaultRoute: () => RemoveDefaultRoute(),
  restoreDefaultRoute: () => RestoreDefaultRoute(),
  getDefaultRouteInfo: () => GetDefaultRouteInfo(),
  clearLogs: () => ClearLogs(),
  getProxyConfigInfo: () => GetProxyConfigInfo(),
  hideToTray: async () => {
    if (HideToTray) {
      return HideToTray()
    }
    WindowHide()
  },
  quitApp: () => QuitApp(),
  onState: (handler) => EventsOn('netgapgui:state', handler),
  onHealth: (handler) => EventsOn('netgapgui:health', handler),
  onLogs: (handler) => EventsOn('netgapgui:logs', handler),
  onAlert: (handler) => EventsOn('netgapgui:alert', handler),
  onAlertState: (handler) => EventsOn('netgapgui:alert-state', handler),
}
