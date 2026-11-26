import { Events } from '@wailsio/runtime'
import * as App from '../bindings/netgap/cmd/netgapgui/app.js'

const on = (name, handler) => Events.On(name, (event) => {
  const data = event.data
  if (Array.isArray(data)) return handler(...data)
  return handler(data)
})

export const backend = {
  logUIError: (message) => App.LogUIError(message),
  getInitialState: () => App.GetInitialState(),
  checkForUpdates: (version) => App.CheckForUpdates(version),
  downloadUpdate: () => App.DownloadUpdate(),
  getPendingUpdate: (version) => App.GetPendingUpdate(version),
  installUpdate: () => App.InstallUpdate(),
  showWindow: () => App.ShowWindow(),
  getElevationStatus: () => App.GetElevationStatus(),
  restartAsAdmin: () => App.RestartAsAdmin(),
  getStartupStatus: () => App.GetStartupStatus(),
  setStartupEnabled: (enabled) => App.SetStartupEnabled(enabled),
  saveSettings: (payload) => App.SaveSettings(payload),
  addRegion: (keyword) => App.AddRegion(keyword),
  removeRegion: (region) => App.RemoveRegion(region),
  startMonitoring: () => App.StartMonitoring(),
  stopMonitoring: () => App.StopMonitoring(),
  setWindowForeground: (foreground, revision) => App.SetWindowForeground(foreground, revision),
  removeDefaultRoute: () => App.RemoveDefaultRoute(),
  restoreDefaultRoute: () => App.RestoreDefaultRoute(),
  listRouteBackups: () => App.ListRouteBackups(),
  getRouteBackupDetail: (id) => App.GetRouteBackupDetail(id),
  restoreRouteBackup: (id, replaceConflicts) => App.RestoreRouteBackup(id, replaceConflicts),
  getDefaultRouteInfo: () => App.GetDefaultRouteInfo(),
  clearLogs: () => App.ClearLogs(),
  getProxyConfigInfo: () => App.GetProxyConfigInfo(),
  hideToTray: () => App.HideToTray(),
  quitApp: () => App.QuitApp(),
  onState: (handler) => on('netgapgui:state', handler),
  onHealth: (handler) => on('netgapgui:health', handler),
  onLogs: (handler) => on('netgapgui:logs', handler),
  onAlert: (handler) => on('netgapgui:alert', handler),
  onAlertState: (handler) => on('netgapgui:alert-state', handler),
  frontendReady: () => App.FrontendReady(),
}

