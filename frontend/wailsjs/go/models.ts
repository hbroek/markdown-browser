export namespace auth {
	
	export class Credential {
	    origin: string;
	    kind: string;
	    cookie?: string;
	    token?: string;
	    username?: string;
	    password?: string;
	
	    static createFrom(source: any = {}) {
	        return new Credential(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origin = source["origin"];
	        this.kind = source["kind"];
	        this.cookie = source["cookie"];
	        this.token = source["token"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class Info {
	    origin: string;
	    kind: string;
	    username?: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origin = source["origin"];
	        this.kind = source["kind"];
	        this.username = source["username"];
	    }
	}

}

export namespace history {
	
	export class Entry {
	    url: string;
	    title: string;
	    // Go type: time
	    time: any;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.title = source["title"];
	        this.time = this.convertValues(source["time"], null);
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

export namespace page {
	
	export class Link {
	    text: string;
	    href: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.href = source["href"];
	    }
	}
	export class Page {
	    url: string;
	    kind: string;
	    status: number;
	    contentType: string;
	    title: string;
	    html: string;
	    source: string;
	    links: Link[];
	    error: string;
	    origin: string;
	    authScheme: string;
	    canBack: boolean;
	    canForward: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Page(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.contentType = source["contentType"];
	        this.title = source["title"];
	        this.html = source["html"];
	        this.source = source["source"];
	        this.links = this.convertValues(source["links"], Link);
	        this.error = source["error"];
	        this.origin = source["origin"];
	        this.authScheme = source["authScheme"];
	        this.canBack = source["canBack"];
	        this.canForward = source["canForward"];
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

