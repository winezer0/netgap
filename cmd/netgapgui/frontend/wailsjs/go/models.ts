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
	    }
	}

}

export namespace guiapp {
	
	export class ProxyConfigInfo {
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
	        this.enabled = source["enabled"];
	        this.type = source["type"];
	        this.address = source["address"];
	        this.port = source["port"];
	        this.source = source["source"];
	    }
	}
	export class RouteActionResult {
	    success: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
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
	    logs: string[];
	    configPath: string;
	    isAdmin: boolean;
	    running: boolean;
	    statusText: string;
	    permissionText: string;
	
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
	        this.logs = source["logs"];
	        this.configPath = source["configPath"];
	        this.isAdmin = source["isAdmin"];
	        this.running = source["running"];
	        this.statusText = source["statusText"];
	        this.permissionText = source["permissionText"];
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

