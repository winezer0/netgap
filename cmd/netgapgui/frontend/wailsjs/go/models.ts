export namespace autostart {
	
	export class Status {
	    supported: boolean;
	    registered: boolean;
	    needsRepair: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.registered = source["registered"];
	        this.needsRepair = source["needsRepair"];
	    }
	}

}

export namespace configstore {
	
	export class Settings {
	    autoStartMonitoring: boolean;
	    startMinimized: boolean;
	    language: string;
	    interval: number;
	    keywordInput: string;
	    regions: string[];
	    apiEnabled: string[];
	    customApiUrl: string;
	    closeAction: string;
	    removeRouteOnMatch: boolean;
	    notificationPopup: boolean;
	    autoCheckUpdates: boolean;
	    lastUpdateCheckAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoStartMonitoring = source["autoStartMonitoring"];
	        this.startMinimized = source["startMinimized"];
	        this.language = source["language"];
	        this.interval = source["interval"];
	        this.keywordInput = source["keywordInput"];
	        this.regions = source["regions"];
	        this.apiEnabled = source["apiEnabled"];
	        this.customApiUrl = source["customApiUrl"];
	        this.closeAction = source["closeAction"];
	        this.removeRouteOnMatch = source["removeRouteOnMatch"];
	        this.notificationPopup = source["notificationPopup"];
	        this.autoCheckUpdates = source["autoCheckUpdates"];
	        this.lastUpdateCheckAt = source["lastUpdateCheckAt"];
	    }
	}

}

export namespace elevation {
	
	export class RestartResult {
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RestartResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	    }
	}
	export class Status {
	    supported: boolean;
	    isAdmin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.isAdmin = source["isAdmin"];
	    }
	}

}

export namespace guiapp {
	
	export class Alert {
	    type: string;
	    message: string;
	    timestamp: number;
	    observedAt: number;
	    eventId: number;
	    runId: number;
	    ip: string;
	    location: string;
	    path: string;
	    protection: string;
	    active: boolean;
	    historical: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Alert(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.message = source["message"];
	        this.timestamp = source["timestamp"];
	        this.observedAt = source["observedAt"];
	        this.eventId = source["eventId"];
	        this.runId = source["runId"];
	        this.ip = source["ip"];
	        this.location = source["location"];
	        this.path = source["path"];
	        this.protection = source["protection"];
	        this.active = source["active"];
	        this.historical = source["historical"];
	    }
	}
	export class ProxyConfigInfo {
	    observationOnly: string;
	    enabled: boolean;
	    type: string;
	    address: string;
	    port: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfigInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observationOnly = source["observationOnly"];
	        this.enabled = source["enabled"];
	        this.type = source["type"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.source = source["source"];
	    }
	}
	export class RouteActionResult {
	    success: boolean;
	    noop: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.noop = source["noop"];
	        this.message = source["message"];
	    }
	}
	export class ViewState {
	    autoStartMonitoring: boolean;
	    startMinimized: boolean;
	    language: string;
	    interval: number;
	    keywordInput: string;
	    regions: string[];
	    apiEnabled: string[];
	    customApiUrl: string;
	    closeAction: string;
	    removeRouteOnMatch: boolean;
	    notificationPopup: boolean;
	    autoCheckUpdates: boolean;
	    lastUpdateCheckAt: string;
	    logs: string[];
	    errorCount: number;
	    configPath: string;
	    isAdmin: boolean;
	    running: boolean;
	    statusText: string;
	    permissionText: string;
	    health: monitor.Health;
	    latestAlert?: Alert;
	    alertUnread: boolean;
	    alertRevision: number;
	
	    static createFrom(source: any = {}) {
	        return new ViewState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoStartMonitoring = source["autoStartMonitoring"];
	        this.startMinimized = source["startMinimized"];
	        this.language = source["language"];
	        this.interval = source["interval"];
	        this.keywordInput = source["keywordInput"];
	        this.regions = source["regions"];
	        this.apiEnabled = source["apiEnabled"];
	        this.customApiUrl = source["customApiUrl"];
	        this.closeAction = source["closeAction"];
	        this.removeRouteOnMatch = source["removeRouteOnMatch"];
	        this.notificationPopup = source["notificationPopup"];
	        this.autoCheckUpdates = source["autoCheckUpdates"];
	        this.lastUpdateCheckAt = source["lastUpdateCheckAt"];
	        this.logs = source["logs"];
	        this.errorCount = source["errorCount"];
	        this.configPath = source["configPath"];
	        this.isAdmin = source["isAdmin"];
	        this.running = source["running"];
	        this.statusText = source["statusText"];
	        this.permissionText = source["permissionText"];
	        this.health = this.convertValues(source["health"], monitor.Health);
	        this.latestAlert = this.convertValues(source["latestAlert"], Alert);
	        this.alertUnread = source["alertUnread"];
	        this.alertRevision = source["alertRevision"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace iproute {
	
	export class RouteEntry {
	    // Go type: time
	    capturedAt?: any;
	    destination: string;
	    nextHop: string;
	    interfaceIndex: number;
	    interfaceAlias?: string;
	    compartmentId?: number;
	    interfaceGuid: string;
	    addresses: string[];
	    network: string;
	    metric: number;
	    store: string;
	    protocol: string;
	    publish: string;
	    validTicks: string;
	    preferredTicks: string;
	    raw?: number[];
	    nativeKey?: number[];
	    bindings?: RouteEntry[];
	    createdByRecovery?: RouteEntry;
	    activeBefore?: RouteEntry;
	    removeAfterRestore?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RouteEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capturedAt = this.convertValues(source["capturedAt"], null);
	        this.destination = source["destination"];
	        this.nextHop = source["nextHop"];
	        this.interfaceIndex = source["interfaceIndex"];
	        this.interfaceAlias = source["interfaceAlias"];
	        this.compartmentId = source["compartmentId"];
	        this.interfaceGuid = source["interfaceGuid"];
	        this.addresses = source["addresses"];
	        this.network = source["network"];
	        this.metric = source["metric"];
	        this.store = source["store"];
	        this.protocol = source["protocol"];
	        this.publish = source["publish"];
	        this.validTicks = source["validTicks"];
	        this.preferredTicks = source["preferredTicks"];
	        this.raw = source["raw"];
	        this.nativeKey = source["nativeKey"];
	        this.bindings = this.convertValues(source["bindings"], RouteEntry);
	        this.createdByRecovery = this.convertValues(source["createdByRecovery"], RouteEntry);
	        this.activeBefore = this.convertValues(source["activeBefore"], RouteEntry);
	        this.removeAfterRestore = source["removeAfterRestore"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RouteBackupRoute {
	    saved: RouteEntry;
	    current?: RouteEntry;
	    state: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteBackupRoute(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saved = this.convertValues(source["saved"], RouteEntry);
	        this.current = this.convertValues(source["current"], RouteEntry);
	        this.state = source["state"];
	        this.reason = source["reason"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RouteBackupSummary {
	    id: string;
	    // Go type: time
	    savedAt: any;
	    routeCount: number;
	    ipv4Count: number;
	    ipv6Count: number;
	    interfaces: string[];
	
	    static createFrom(source: any = {}) {
	        return new RouteBackupSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.savedAt = this.convertValues(source["savedAt"], null);
	        this.routeCount = source["routeCount"];
	        this.ipv4Count = source["ipv4Count"];
	        this.ipv6Count = source["ipv6Count"];
	        this.interfaces = source["interfaces"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RouteBackupDetail {
	    summary: RouteBackupSummary;
	    routes: RouteBackupRoute[];
	
	    static createFrom(source: any = {}) {
	        return new RouteBackupDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], RouteBackupSummary);
	        this.routes = this.convertValues(source["routes"], RouteBackupRoute);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RouteBackupHistory {
	    records: RouteBackupSummary[];
	    invalidLines: number;
	
	    static createFrom(source: any = {}) {
	        return new RouteBackupHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], RouteBackupSummary);
	        this.invalidLines = source["invalidLines"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RouteBackupRestoreResult {
	    restored: number;
	    alreadyPresent: number;
	    replaced: number;
	    skipped: number;
	    failed: number;
	    failures: string[];
	    complete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RouteBackupRestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.restored = source["restored"];
	        this.alreadyPresent = source["alreadyPresent"];
	        this.replaced = source["replaced"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.failures = source["failures"];
	        this.complete = source["complete"];
	    }
	}
	
	
	
	export class RouteInfo {
	    gateway: string;
	    interface: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway = source["gateway"];
	        this.interface = source["interface"];
	    }
	}

}

export namespace monitor {
	
	export class Health {
	    runId: number;
	    revision: number;
	    running: boolean;
	    phase: string;
	    intervalSeconds: number;
	    startedAt: number;
	    lastAttemptAt: number;
	    lastCompletedAt: number;
	    lastSuccessAt: number;
	    consecutiveFailures: number;
	    queryFailure: string;
	    observationOnly: string;
	    disconnectState: string;
	    disconnectFailure: string;
	    nextRetryAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Health(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.revision = source["revision"];
	        this.running = source["running"];
	        this.phase = source["phase"];
	        this.intervalSeconds = source["intervalSeconds"];
	        this.startedAt = source["startedAt"];
	        this.lastAttemptAt = source["lastAttemptAt"];
	        this.lastCompletedAt = source["lastCompletedAt"];
	        this.lastSuccessAt = source["lastSuccessAt"];
	        this.consecutiveFailures = source["consecutiveFailures"];
	        this.queryFailure = source["queryFailure"];
	        this.observationOnly = source["observationOnly"];
	        this.disconnectState = source["disconnectState"];
	        this.disconnectFailure = source["disconnectFailure"];
	        this.nextRetryAt = source["nextRetryAt"];
	    }
	}

}

