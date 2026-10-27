export namespace ai {
	
	export class Message {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class Preset {
	    id: string;
	    name: string;
	    baseUrl: string;
	    model: string;
	    keyRequired: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.keyRequired = source["keyRequired"];
	        this.note = source["note"];
	    }
	}
	export class Suggestion {
	    title: string;
	    description: string;
	    keywords: string[];
	
	    static createFrom(source: any = {}) {
	        return new Suggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.keywords = source["keywords"];
	    }
	}

}

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

export namespace bookmarks {
	
	export class Bookmark {
	    id: string;
	    folderId: string;
	    title: string;
	    url: string;
	    description: string;
	    keywords: string[];
	    // Go type: time
	    created: any;
	    // Go type: time
	    updated: any;
	
	    static createFrom(source: any = {}) {
	        return new Bookmark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folderId = source["folderId"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.description = source["description"];
	        this.keywords = source["keywords"];
	        this.created = this.convertValues(source["created"], null);
	        this.updated = this.convertValues(source["updated"], null);
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
	export class Folder {
	    id: string;
	    parentId: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parentId = source["parentId"];
	        this.name = source["name"];
	    }
	}
	export class ImportResult {
	    folders: number;
	    bookmarks: number;
	    duplicates: number;
	    skipped: number;
	    canceled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folders = source["folders"];
	        this.bookmarks = source["bookmarks"];
	        this.duplicates = source["duplicates"];
	        this.skipped = source["skipped"];
	        this.canceled = source["canceled"];
	    }
	}

}

export namespace fetch {
	
	export class Header {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Header(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class Entry {
	    id: number;
	    // Go type: time
	    time: any;
	    method: string;
	    url: string;
	    status: number;
	    statusText: string;
	    proto: string;
	    contentType: string;
	    durationMs: number;
	    bytes: number;
	    reqHeaders: Header[];
	    respHeaders: Header[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = this.convertValues(source["time"], null);
	        this.method = source["method"];
	        this.url = source["url"];
	        this.status = source["status"];
	        this.statusText = source["statusText"];
	        this.proto = source["proto"];
	        this.contentType = source["contentType"];
	        this.durationMs = source["durationMs"];
	        this.bytes = source["bytes"];
	        this.reqHeaders = this.convertValues(source["reqHeaders"], Header);
	        this.respHeaders = this.convertValues(source["respHeaders"], Header);
	        this.error = source["error"];
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

export namespace main {
	
	export class AIAnswer {
	    text: string;
	    html: string;
	    source: string;
	    millis: number;
	
	    static createFrom(source: any = {}) {
	        return new AIAnswer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.html = source["html"];
	        this.source = source["source"];
	        this.millis = source["millis"];
	    }
	}
	export class AIInfo {
	    presets: ai.Preset[];
	    preset: string;
	    baseUrl: string;
	    model: string;
	    configured: boolean;
	    hasKey: boolean;
	    consented: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.presets = this.convertValues(source["presets"], ai.Preset);
	        this.preset = source["preset"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.configured = source["configured"];
	        this.hasKey = source["hasKey"];
	        this.consented = source["consented"];
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
	export class AIInput {
	    preset: string;
	    baseUrl: string;
	    model: string;
	    consent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset = source["preset"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.consent = source["consent"];
	    }
	}
	export class AISummary {
	    html: string;
	    source: string;
	    millis: number;
	
	    static createFrom(source: any = {}) {
	        return new AISummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.html = source["html"];
	        this.source = source["source"];
	        this.millis = source["millis"];
	    }
	}
	export class BookmarkItem {
	    bookmark: bookmarks.Bookmark;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new BookmarkItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bookmark = this.convertValues(source["bookmark"], bookmarks.Bookmark);
	        this.path = source["path"];
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
	export class BookmarkSettings {
	    backend: string;
	    root: string;
	    connected: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new BookmarkSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.root = source["root"];
	        this.connected = source["connected"];
	        this.error = source["error"];
	    }
	}
	export class ChatTurn {
	    role: string;
	    text: string;
	    html: string;
	    source: string;
	    millis: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatTurn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.text = source["text"];
	        this.html = source["html"];
	        this.source = source["source"];
	        this.millis = source["millis"];
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

