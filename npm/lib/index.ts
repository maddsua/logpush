import { newConsole, newLogger, type LogEntry, type Logger, type LogpushConsole } from "./logger";
import { unwrapMetadata, type LogLevel, type Metadata, type MetadataInit } from "./transform";

/**
 * Basic auth is used to prevent unauthorized log writes
 */
type BasicAuth = {
	user: string;
	pass: string;
};

/**
 * Logpush agent is a class that holds instance/context level metadata, log queue and a connection to Logpush service.
 * 
 * All metadata fields added to the agent will be copied to every log entry that it pushes,
 * so put stuff like app environment name and other static options here.
 */
export class Agent {

	readonly url: string;
	readonly auth: BasicAuth | null = null;
	readonly meta: Metadata;
	private entries: LogEntry[];

	readonly logger: Logger;
	readonly console: LogpushConsole;
	
	constructor(url: URL | string, meta?: MetadataInit, service_id?: string) {

		this.meta = Object.assign({}, unwrapMetadata(meta) || {});

		const useURL = typeof url === 'string' ? new URL(url) : url;

		if (!useURL.pathname.toLowerCase().includes('/push/')) {
			useURL.pathname = '/push/stream/';
			if (service_id) {
				useURL.pathname += service_id;
			}
		}

		if (useURL.username) {
			this.auth = { user: useURL.username, pass: useURL.password };
			useURL.username = "";
			useURL.password = "";
		}

		this.url = useURL.href;
		this.entries = [];

		this.console = newConsole(this.pushEntry);
		this.logger = newLogger(this.pushEntry);
	}

	private pushEntry = (level: LogLevel, message: string, meta?: MetadataInit) => {
		
		const date = new Date();

		const entry = {
			date: date.getTime(),
			level,
			message,
			meta: unwrapMetadata(meta),
		};

		this.entries.push(entry);

		const logFn = console.debug || console.log;
		if (typeof logFn === 'function') {
			logFn(`${slogDate(date)} ${level.toUpperCase()} ${message}`, entry.meta || undefined);
		}
	};

	flush = async () => {

		if (!this.entries.length) {
			return;
		}

		const headers = new Headers({
			"content-type": "application/json",
		});

		if (this.auth) {
			headers.set("authorization", `Basic ${btoa(this.auth.user + ':' + this.auth.pass)}`);
		}

		const response = await fetch(this.url, {
			method: 'POST',
			headers: headers,
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
