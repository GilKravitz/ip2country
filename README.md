# ip2country

HTTP service that resolves an IP address to a country and city.

## API

Lookup endpoint:

```sh
curl 'http://localhost:8080/v1/find-country?ip=2.22.233.255'
```

Successful responses use this shape:

```json
{"country":"GB","city":"London"}
```

Error responses use this shape with the relevant HTTP status code:

```json
{"error":"invalid ip address"}
```

Common lookup statuses:

| Status | Meaning |
| --- | --- |
| `200` | IP address was found. |
| `400` | Missing or invalid `ip` query parameter. |
| `404` | IP address is valid but not found in the active datastore. |
| `405` | HTTP method is not allowed. Use `GET`. |
| `429` | Rate limit was exceeded. |
| `500` | Internal server error. |

Operational endpoints:

| Endpoint | Purpose |
| --- | --- |
| `GET /healthz` | Liveness probe. |
| `GET /metrics` | Prometheus metrics scrape endpoint. |

`/healthz` and `/metrics` are operational endpoints and do not consume lookup rate-limit budget.

## Configuration

Configuration is read from environment variables at startup.

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | Address the HTTP server binds to. |
| `IP2COUNTRY_DB` | `csv` | Active datastore backend. |
| `IP2COUNTRY_CSV_PATH` | none | CSV file path. Required when `IP2COUNTRY_DB=csv`. |
| `RATE_LIMIT_RPS` | `100` | Global allowed requests per second for lookup requests. Must be positive. |
| `LOG_LEVEL` | `info` | Structured log level: `debug`, `info`, `warn`, or `error`. |

## CSV Datastore

The CSV backend expects rows in this format:

```text
from,to,City,Country
```

Example:

```csv
2.22.233.0,2.22.233.255,London,GB
```

Rows are loaded once at startup. IP ranges are inclusive.

## Run Locally

```sh
IP2COUNTRY_CSV_PATH=testdata/sample.csv go run ./cmd/server
```

Then query the sample data:

```sh
curl 'http://localhost:8080/v1/find-country?ip=2.22.233.255'
curl 'http://localhost:8080/healthz'
curl 'http://localhost:8080/metrics'
```

## Run With Docker Compose

```sh
docker compose up --build
```

The image includes `testdata/sample.csv` at `/data/sample.csv`. To use another dataset, mount the file and set `IP2COUNTRY_CSV_PATH` in `compose.yaml`.

## Validate

```sh
go test ./...
go vet ./...
```
