import { stringifyArgList, type LogLevel, type Metadata, type MetadataInit } from "./transform";

export interface LogEntry {
	date: number;
	level: LogLevel;
	message: string;
	meta?: Metadata | null;
};

export type WriterWriteFn = (level: LogLevel, message: string, meta?: MetadataInit) => void;

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

export const newLogger = (writer: WriterWriteFn): Logger => {

	const supportedMethods = new Set<LogLevel>(['debug', 'error', 'warn', 'log', 'info', 'trace']);

	return new Proxy({}, {
		get(target, key, receiver) {

			if (!supportedMethods.has(key as LogLevel)) {
				return undefined;
			}

			return function(message: string, meta?: MetadataInit) {
				writer(key as LogLevel, message, meta);
			}
		}
	}) as Logger;
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

export const newConsole = (writer: WriterWriteFn): Logger => {

	const supportedMethods = new Set<LogLevel>(['debug', 'error', 'warn', 'log', 'info']);

	return new Proxy({}, {
		get(target, key, receiver) {

			if (!supportedMethods.has(key as LogLevel)) {
				return undefined;
			}

			return function(...args: any[]) {
				writer(key as LogLevel, stringifyArgList(args));
			}
		}
	}) as Logger;
};
