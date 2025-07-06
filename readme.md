## The elevator pitch

One way to tell this would be that it's really annoying having to jump from one dashboard to another just to check on all of your serverless apss. Yeah, pretty much not a single provider to this day provides a centralized operations center, sort of speak. You always end up clicking way too many buttons just to discover that everyting is broken after the latest commit by the intern.

Another way to put it would be: renting an okay-ish VPS costs under 10 buchs these days, and getting proper log retention on Vercel costs over 20. You see where I'm gonig with it, right?

I mean, I don't condone ripping off the multimillion dollar companies but just so you know, nothing stops you from doing so.

The only downside here is that you'll still need to set up grafana yourself, yeah.

**Features:**

- Endpoint basic auth
- Stream token auth
- Log batching
- TypeScript client (available on npm and the github registry)
- Label sanitization
- Log volume limits

### Writers

In order to be able to do anything meaningfull with your logs there must be a place to send them to. As of now only grafana loki and timescaledb are supported.

#### timescale

Set `TIMESCALE_URL` to a valid postgres url to enable this driver. It will write all of your logs into a table called something like `logpush_entries_v?`, from which you can then query the logs using any sql client.

#### loki

Set `LOKI_URL` to a url pointing to your loki host. The loki writer does a few label transformations in order to make proper tags available the way it's intended to.

IMPORTANT: you should have structured metadata enabled in your loki config as this service uses that by default.
It's simply there to prevent Loki from shitting the bed due to the high cardinality of the labels or whatever.
Just enable the feature dawg.

Structured metadata is enabled by adding `?labels=struct` or `s_meta=true` query parameters.

## Deploying

The easiest way to deploy logpush is by using docker:
```dockerfile
from ghcr.io/maddsua/logpush:latest
copy ./logpush.yml /etc/mws/logpush/logpush.yml
```

Config reference:
```yml
ingester:
  basic_auth:                # sets username/password pairs for all streams
    username: password
  max_entries: 100          # batch entry count limit. anything over this would be truncated
  max_message_size: 100000  # message size limit in bytes
  max_metadata_size: 64000  # total batch metadata block size limit in bytes
  max_label_size: 100       # label key size limit in characters
  max_field_size: 1000      # label value size limit in characters
streams:
  stream-key:                  # key is the unique stream_id or (service id in loki)
    tag: mytag              # optional value to overwrite app-key (some legacy systems use random tokens in stream keys as a security measure)
    labels:                 # custom stream labels that will be written over any conflicting log labels
      org: mws
      env: dev
    token: verystrongpassword # oh look, we have an additional token requirement here
```

**Using auth:**

- Basic auth: Pass it in the url params like so: `http://myuser:mypass@myhostpush/stream/myapp`
- Token auth: Pass the token in the `Authorization` header (type: `Bearer`) OR with a `?token=token` URL parameter


**Client URLs**

To form a client URL follow this format: `{protocol}://{host}:{port}/${stream_id}?token={token}`.
