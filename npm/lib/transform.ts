
export type LogLevel = 'error' | 'warn' | 'info' | 'debug' | 'log' | 'trace';
export type Metadata = Record<string, string>;
export type MetadataInitValue = string | number | boolean | null | undefined;
export type MetadataInit = Record<string, MetadataInitValue>;

export const stringifyArgList = (args: any[]) => args.map(item => stringifyArg(item)).join(' ');

export const stringifyArg = (item: any, nested?: boolean): string => {
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

export const stringifyObjectArg = (value: object): string => {

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

export const stringifyObjectReplacer = (_: string, value: any): any => {

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

export const unwrapMetadata = (init?: MetadataInit): Metadata | null => {

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
