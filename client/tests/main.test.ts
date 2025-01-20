import { Agent, type Logger } from "../lib/client";

const fakeHandler = async (request: Request, waitUntil: (task: Promise<any>) => void): Promise<Response> => {

	const agent = new Agent('http://localhost:8000/push/stream/0d460d8a-f497-4027-9384-c45378a5a63d', {
		client_ip: request.headers.get('x-forwarded-for'),
		agent: request.headers.get('user-agent'),
		method: request.method.toLowerCase(),
		api: new URL(request.url).pathname,
		env: 'prod',
	});

	const result = await fakeProcedure(request, agent.logger);

	waitUntil(agent.flush());

	return result;
};

const fakeProcedure = async (_request: Request, logger: Logger): Promise<Response> => {

	logger.info('Processing lead form request');
	logger.error('Some important API is down, rerouting', { api_down: 'catpics' });
	logger.warn('Rerouting to an internal API', { timeout: 3600 });
	logger.info('Recaptcha OK', { score: 0.9 });

	logger.error('Failed to do some ops, hollon...');
	logger.info('Oh nvm that actually worked', { lead_id: 42, product: 'miata', price: 35_000 });

	return new Response('success');
};

const fakeRuntime = async (request: Request): Promise<Response> => {

	const tasks: Promise<void>[] = [];
	const pushTask = (next: Promise<void>) => tasks.push(next);

	const result = await fakeHandler(request, pushTask);

	if (tasks.length) {
		await Promise.all(tasks.map(item => item.catch(err => console.error(err))));
	}

	return result;
};

fakeRuntime(new Request('http://example.com/api/name/procedure', {
	method: 'GET',
	headers: {
		'x-forwarded-for': '10.10.10.10',
		'user-agent': 'x-maddsua-bruh'
	}
}));
