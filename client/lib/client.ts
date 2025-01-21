
type LogLevel = 'error' | 'warn' | 'info' | 'debug' | 'log';
type Metadata = Record<string, string>;
type MetadataInitValue = string | number | boolean | null | undefined;
type MetadataInit = Record<string, MetadataInitValue>;

interface LogEntry {
	date: number;
	level: LogLevel;
	message: string;
	meta?: Metadata | null;
};

type LoggerPushFn = (message: string, meta?: MetadataInit) => void;

/**
 * Logger implements a similar to go slog interface.
 * 
 * Use it to push new log entries.
 */
export interface Logger {
	log: LoggerPushFn;
	info: LoggerPushFn;
	debug: LoggerPushFn;
	warn: LoggerPushFn;
	error: LoggerPushFn;
};

/**
 * Console is a compatibility interface for eventdb and loki-serverless clients.
 * 
 * It implements the most frequently used methods of the standard ES console,
 * however not all objects and classes can be properly serialized by it.
 * 
 * To ensure proper serialization consider preferring to pass only objects
 * that can be JSON-serialized. Some widely used classes as FormData, Date, Set, Map and RegExp
 * are fully supported, while classes like Request and Response will not be fully serialized to
 * avoid any side effects of calling their async methods.
 * 
 * For new projects you should use the Logger interface instead.
 */
export interface LogpushConsole {
	info: (...args: any[]) => void;
	log: (...args: any[]) => void;
	warn: (...args: any[]) => void;
	error: (...args: any[]) => void;
	debug: (...args: any[]) => void;
};

/**
 * Logpush agent is a class that holds instance/context level metadata, log queue and a connection to Logpush service.
 * 
 * All metadata fields added to the agent will be copied to every log entry that it pushes,
 * so put stuff like app environment name and other static options here.
 */
export class Agent {

	readonly url: string;
	readonly meta: Metadata;
	private entries: LogEntry[];
	
	constructor(url: URL | string, meta?: MetadataInit, service_id?: string) {

		this.meta = Object.assign({}, unwrapMetadata(meta) || {});

		const useURL = typeof url === 'string' ? new URL(url) : url;

		if (!useURL.pathname.toLowerCase().includes('/push/')) {
			useURL.pathname = '/push/stream/';
			if (service_id) {
				useURL.pathname += service_id;
			}
		}

		this.url = useURL.href;
		this.entries = [];
	}

	private m_loggerPush = (level: LogLevel, message: string, meta?: MetadataInit) => {
		
		const date = new Date();

		this.entries.push({
			date: date.getTime(),
			level,
			message,
			meta: unwrapMetadata(meta),
		});

		const logFn = console.debug || console.log;
		if (typeof logFn === 'function') {
			logFn(`${slogDate(date)} ${level.toUpperCase()} ${message}`);
		}
	};

	readonly logger: Logger = {
		log: (message: string, meta?: MetadataInit) => this.m_loggerPush('log', message, meta),
		info: (message: string, meta?: MetadataInit) => this.m_loggerPush('info', message, meta),
		debug: (message: string, meta?: MetadataInit) => this.m_loggerPush('debug', message, meta),
		warn: (message: string, meta?: MetadataInit) => this.m_loggerPush('warn', message, meta),
		error: (message: string, meta?: MetadataInit) => this.m_loggerPush('error', message, meta),
	};

	private m_consolePush = (level: keyof LogpushConsole, args: any[]) => {

		this.entries.push({
			date: new Date().getTime(),
			level,
			message: args.map(item => stringifyArg(item)).join(' '),
		});
		
		const logFn = console[level] || console.log;
		if (typeof logFn === 'function') {
			logFn(...args);
		}
	};

	readonly console: LogpushConsole = {
		info: (...args: any[]) => this.m_consolePush('info', args),
		log: (...args: any[]) => this.m_consolePush('log', args),
		warn: (...args: any[]) => this.m_consolePush('warn', args),
		error: (...args: any[]) => this.m_consolePush('error', args),
		debug: (...args: any[]) => this.m_consolePush('debug', args),
	};

	flush = async () => {

		if (!this.entries.length) {
			return;
		}

		const response = await fetch(this.url, {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ meta: this.meta, entries: this.entries })
		});

		if (response.ok) {
			this.entries = [];
			return;
		}

		throw new Error(`Failed to flush log entries: ${await response.text()}`);	
	};
};

const unwrapMetadata = (init?: MetadataInit): Metadata | null => {

	if (!init) {
		return null;
	}

	const transform = (val: MetadataInitValue): string | null => {
		switch (typeof val) {
			case 'string':
				return val.trim();
			case 'number':
				return `${val}`;
			case 'boolean':
				return `${val}`;
			default:
				return null;
		}
	};

	return Object.fromEntries(Object.entries(init)
		.map(([key, value]) => ([key, transform(value)]))
		.filter(([_, val]) => !!val)) as Metadata;
};

const slogDate = (date: Date): string => {

	const year = date.getFullYear();
	const month = (date.getMonth() + 1).toString().padStart(2, '0');
	const day = date.getDate().toString().padStart(2, '0');
	const hour = date.getHours().toString().padStart(2, '0');
	const min = date.getMinutes().toString().padStart(2, '0');
	const sec = date.getSeconds().toString().padStart(2, '0');

	return `${year}/${month}/${day} ${hour}:${min}:${sec}`;
};

const stringifyArg = (item: any, nested?: boolean): string => {
	switch (typeof item) {
		case 'string': return nested ? `'${item}'` : item;
		case 'number': return item.toString();
		case 'bigint': return item.toString();
		case 'boolean': return `${item}`;
		case 'object': return stringifyObjectArg(item);
		case 'function': return '[fn()]';
		case 'symbol': return item.toString();
		default: return '{}';
	}
};

const stringifyObjectArg = (value: object): string => {

	try {

		if (value instanceof Error) {
			return value.stack ? `${value.stack}\n` : `${value.name || 'Error'}: '${value.message}'`;
		}

		if (value instanceof Date) {
			return `'${value.toUTCString()}'`;
		}

		if (value instanceof RegExp) {
			return `'${value}'`;
		}
		
		if (value instanceof URL) {
			return `'${value.href}'`;
		}

		return JSON.stringify(value, stringifyObjectReplacer);

	} catch (_) {
		return '{}';
	}
};

const stringifyObjectReplacer = (_: string, value: any): any => {

	if (typeof value !== 'object') {
		return value;
	}

	if (value instanceof Error) {
		return { message: value.message, stack: value.stack, type: value.name };
	}

	if (value instanceof FormData || value instanceof Map || value instanceof Headers) {
		return Object.fromEntries(value);
	}

	if (value instanceof Date) {
		return value.toUTCString();
	}

	if (value instanceof RegExp) {
		return value.source;
	}

	if (value instanceof Set) {
		return Array.from(value.keys());
	}

	if (value instanceof Request) {
		return {
			url: value.url,
			method: value.method,
			headers: value.headers,
			referrer: value.referrer,
			credentials: value.credentials,
			mode: value.mode,
		};
	}

	if (value instanceof Response) {
		return { status: value.status, headers: value.headers, type: value.type };
	}

	return value;
};
