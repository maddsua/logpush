## The elevator pitch

I bet you had a headache of having to manualll check logs from multiple SaaS services.

Imagine this: your 15 side projects with a total of 10 MAU are deployed in the popular these days way, where frontend
sits on Netlify/Vercel/Cloudflare pages, the databases are completely different things
living on their own and the rest is a bunch of auth services/lambdas that don't even want to live under the same roof.

Now, it's the end of the week and you want to check how your projects are doing. How many dashboards are you visiting?

From my experience I would say that it's at least 15x`$the_number_of_services` different screens inside of those dashboards.

And all of that just to check some logs?

### Grafana comes to help

Grafana is a nice tool for data visualization. They have the Loki thingy for logs specifically, however their API is not
the most convenient, and also, how do you separate different projects in a way they won't be able to interfere with each other?

And what if you want to keep an eye on that funni new app from your friend who has kindly asked for your help?
It really sounds like you want to have some sort of log stream separation and authentication.

### Logpush pushes logs

Yup this is where logpush comes into the play.

The basic features are:

- Batched log push REST API (go check the openapi spec)
- TypeScript client, push agent and logger
- Log stream authentication (aka apps can't push under some other app's names)
- Label processing
- Data output to Grafana Loki or Postgres/TimescaleDB

### Should you use it?

If you have a ton of different apps shitting logs all over different hosting platforms, probably yes.

And if you find this project useful give it a start dawg
