import { Agent } from "../lib/client";
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
agent.console.log('Just writing multiple values', true, 42, new Date(), /heeey/ig, new Map([['uhm', 'secret']]), new Error('Task failed successfullly'));

await agent.flush();
