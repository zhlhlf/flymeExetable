export namespace main {
	
	export class WindowPosition {
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new WindowPosition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class AppConfig {
	    folder?: string;
	    folders: string[];
	    windowPosition?: WindowPosition;
	    poem: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.folders = source["folders"];
	        this.windowPosition = this.convertValues(source["windowPosition"], WindowPosition);
	        this.poem = source["poem"];
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
	export class LauncherItem {
	    name: string;
	    path: string;
	    letter: string;
	    extension: string;
	    isDir: boolean;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new LauncherItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.letter = source["letter"];
	        this.extension = source["extension"];
	        this.isDir = source["isDir"];
	        this.icon = source["icon"];
	    }
	}

}

