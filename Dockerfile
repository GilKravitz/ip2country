# Build a static binary, then ship it on distroless (no shell, nonroot, ~2MB base).
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
# Bake the sample dataset so `docker compose up` works with zero extra setup.
# Override by mounting your own file and pointing IP2COUNTRY_CSV_PATH at it.
COPY testdata/sample.csv /data/sample.csv
# Default to the baked sample so `docker run ip2country:local` works as-is;
# compose / -e overrides these.
ENV IP2COUNTRY_CSV_PATH=/data/sample.csv
EXPOSE 8080
ENTRYPOINT ["/server"]
