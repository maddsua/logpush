## Logpush is a service that funnels logs from various SaaS apps to a single point of log retention

It has originated from the project of mine called [eventdb](https://gitlab.com/maddsua/eventdb).
It's huge problem was that it was too focused on a very specific set of features, didn't support any exteranl data storages and was pain in the ahh to maintain.

Later on I switched to using Grafana+Loki, which required to reimplement log stream splitting/routing/auth as a separate service.
There were quite a lot of versions, and I have ultimately decided to make the theird version of this public.
