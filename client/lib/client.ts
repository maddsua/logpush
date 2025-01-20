
type LogLevel = 'error' | 'warn' | 'info' | 'debug' | 'log';
type Metadata = Record<string, string>;
type MetadataInitValue = string | number | boolean | null | undefined;
type MetadataInit = Record<string, MetadataInitValue>;

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

export interface LogEntry {
	date: number;
	level: LogLevel;
	message: string;
	meta: Metadata | null;
};

type LoggerPushFn = (message: string, meta?: MetadataInit) => void;

export interface Logger {
	log: LoggerPushFn;
	info: LoggerPushFn;
	debug: LoggerPushFn;
	warn: LoggerPushFn;
	error: LoggerPushFn;
};

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

	private push = (level: LogLevel, message: string, meta?: MetadataInit) => {
		const date = new Date();
		console.log(`${slogDate(date)} ${level.toUpperCase()} ${message}`);
		this.entries.push({ date: date.getTime(), level, message, meta: unwrapMetadata(meta) });
	};

	readonly logger: Logger = {
		log: (message: string, meta?: MetadataInit) => this.push('log', message, meta),
		info: (message: string, meta?: MetadataInit) => this.push('info', message, meta),
		debug: (message: string, meta?: MetadataInit) => this.push('debug', message, meta),
		warn: (message: string, meta?: MetadataInit) => this.push('warn', message, meta),
		error: (message: string, meta?: MetadataInit) => this.push('error', message, meta),
	};

	flush = async () => {

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

const slogDate = (date: Date): string => {

	const year = date.getFullYear();
	const month = (date.getMonth() + 1).toString().padStart(2, '0');
	const day = date.getDate().toString().padStart(2, '0');
	const hour = date.getHours().toString().padStart(2, '0');
	const min = date.getMinutes().toString().padStart(2, '0');
	const sec = date.getSeconds().toString().padStart(2, '0');

	return `${year}/${month}/${day} ${hour}:${min}:${sec}`;
};
