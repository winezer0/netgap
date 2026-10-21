export namespace configstore {
	
	export class Settings {
	    language: string;
	    interval: number;
	    keywordInput: string;
	    regions: string[];
	    apiEnabled: string[];
	    customApiUrl: string;
	    closeAction: string;
	    removeRouteOnMatch: boolean;
	    notificationPopup: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.interval = source["interval"];
	        this.keywordInput = source["keywordInput"];
	        this.regions = source["regions"];
	        this.apiEnabled = source["apiEnabled"];
	        this.customApiUrl = source["customApiUrl"];
	        this.closeAction = source["closeAction"];
	        this.removeRouteOnMatch = source["removeRouteOnMatch"];
	        this.notificationPopup = source["notificationPopup"];
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
	    language: string;
	    interval: number;
	    keywordInput: string;
	    regions: string[];
	    apiEnabled: string[];
	    customApiUrl: string;
	    closeAction: string;
	    removeRouteOnMatch: boolean;
	    notificationPopup: boolean;
	    logs: string[];
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
	        this.language = source["language"];
	        this.interval = source["interval"];
	        this.keywordInput = source["keywordInput"];
	        this.regions = source["regions"];
	        this.apiEnabled = source["apiEnabled"];
	        this.customApiUrl = source["customApiUrl"];
	        this.closeAction = source["closeAction"];
	        this.removeRouteOnMatch = source["removeRouteOnMatch"];
	        this.notificationPopup = source["notificationPopup"];
	        this.logs = source["logs"];
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

