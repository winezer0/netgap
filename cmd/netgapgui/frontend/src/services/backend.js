import {
  EventsOn,
  WindowHide,
} from '../../wailsjs/runtime/runtime'
import {
  AddRegion,
  ClearLogs,
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
  StopMonitoring,
} from '../../wailsjs/go/main/App'

export const backend = {
  getInitialState: () => GetInitialState(),
  saveSettings: (payload) => SaveSettings(payload),
  addRegion: (keyword) => AddRegion(keyword),
  removeRegion: (region) => RemoveRegion(region),
  startMonitoring: () => StartMonitoring(),
  stopMonitoring: () => StopMonitoring(),
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
}
