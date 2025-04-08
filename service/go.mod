module github.com/maddsua/logpush/service

go 1.22.3

require github.com/joho/godotenv v1.5.1

require github.com/google/uuid v1.6.0

require github.com/lib/pq v1.10.9

require (
	github.com/golang-migrate/migrate/v4 v4.18.1
	github.com/guregu/null v4.0.0+incompatible
	github.com/sqlc-dev/sqlc v1.27.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)
