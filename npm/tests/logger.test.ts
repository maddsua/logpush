import { Agent } from "../lib/index";
import { Logger } from "../lib/logger";
import { testAgentUrl } from "./test.config";

import { faker } from '@faker-js/faker';

const fakeHandler = async (request: Request, waitUntil: (task: Promise<any>) => void): Promise<Response> => {

	const started = new Date();

	const agent = new Agent(testAgentUrl, {
		remote_addr: request.headers.get('x-forwarded-for'),
		agent: request.headers.get('user-agent'),
		method: request.method.toLowerCase(),
		api: new URL(request.url).pathname,
		env: 'prod',
	});

	const result = await fakeProcedure(request, agent.logger);

	agent.logger.debug("Request done", { t: new Date().getTime() - started.getTime() });

	waitUntil(agent.flush());

	return result;
};

const fakeProcedure = async (_request: Request, logger: Logger): Promise<Response> => {

	logger.info('Processing lead form request');

	logger.warn('Pretending that theres an issue with something, but we still can proceed', { 'database': faker.database.engine() })
	logger.info('Recaptcha OK', { score: 0.5 + Math.random() * 0.5 });
	logger.debug('Authorization', { token: faker.internet.jwt()});

	logger.log('Accepted form', {
		lead_id: faker.string.uuid(),
		product_code: 'miata mx5-nd',
		person_name: faker.person.fullName(),
		email: faker.internet.email(),
		phone: faker.phone.number(),
		lead_price: 35_000,
		currency: 'EUR'
	});

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
		'x-forwarded-for': faker.internet.ip(),
		'user-agent': faker.internet.userAgent()
	}
}));
