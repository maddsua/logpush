import { Agent } from "../lib/index";
import { testAgentUrl } from "./test.config";

const agent = new Agent(testAgentUrl, {
	api: 'console-test',
	agent: 'node',
	env: 'dev',
});

const formData = new FormData();
formData.set('name', 'John Doe');
formData.set('phone', '+100000000000');

agent.console.debug('Dumping form data:', formData);
agent.console.debug('Dumping trpc:', { name: 'Jane Doe', ig_username: 42 });

agent.console.debug(
	'Just writing multiple values',
	true,
	42,
	new Date(),
	/heeey/ig,
	new Map([['uhm', 'secret']]),
	new Set(['aaa', { key: 'bbb' }]),
);

agent.console.debug(new Error('Task failed successfullly'));

agent.console.debug([
	1,
	'two',
	{ value: 'three'},
	{ value: 4, alt_value: 5 },
	new Map([['key', 'value']]),
]);

agent.console.debug({
	type: 'lead data',
	title: 'miata shop',
	name: 'maddsua',
	phone: '+380960000000',
	bid_price: 35_000,
	nested: {
		type: 'lead data',
		title: 'miata shop',
		name: 'maddsua',
		phone: '+380960000000',
		bid_price: 35_000,
	}
});

agent.console.debug(new URL('https://localhost:8080/path?query=goth'));

agent.console.debug(new Headers({ 'content-type': 'application/json' }));

agent.console.debug(new Response('ok status', { status: 200, headers: new Headers({ 'content-type': 'application/json' }) }));
agent.console.debug(new Request('https://localhost:8080/path?query=goth', { method: 'GET', headers: new Headers({ 'content-type': 'application/json' }) }));

await agent.flush();
